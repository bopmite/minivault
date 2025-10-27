# minivault

distributed key-value store with geo-replication, <1700 lines of Go

inspired by geohot's minikeyvalue

## overview

stores key-value pairs across nodes, eventually consistent (30-50ms), quorum writes (2/3), survives node failures

**throughput (single node):**
- storage: 2.2M writes/sec, 20M reads/sec
- network: 334k writes/sec, 393k reads/sec (binary tcp)
- http: 100k ops/sec per worker

**protocols:** binary tcp (performance) + http/json (convenience)

## quick start

**single node:**
```bash
./minivault -port 3000 -data ./data
```

**3-node cluster:**
```bash
CLUSTER_NODES="localhost:3001,localhost:3002" \
  ./minivault -port 3001 -data ./data1 -public-url "localhost:3001" &

CLUSTER_NODES="localhost:3000,localhost:3002" \
  ./minivault -port 3002 -data ./data2 -public-url "localhost:3002" &

CLUSTER_NODES="localhost:3000,localhost:3001" \
  ./minivault -port 3003 -data ./data3 -public-url "localhost:3003" &
```

**docker:**
```bash
docker-compose up -d  # launches 3-node cluster
```

## binary protocol

native tcp protocol for performance (tcp:3000 default)

### operations

| opcode | name   | format                                                | response              |
|--------|--------|-------------------------------------------------------|-----------------------|
| 0x01   | GET    | `[01][keylen:u16][key]`                               | `[status][len:u32][data]` |
| 0x02   | SET    | `[02][keylen:u16][key][vallen:u32][compressed][val]` | `[status][len:u32]`   |
| 0x03   | DELETE | `[03][keylen:u16][key]`                               | `[status][len:u32]`   |
| 0x05   | HEALTH | `[05][keylen:u16][key]`                               | `[status][len:u32][json]` |
| 0x06   | AUTH   | `[06][keylen:u16][authkey]`                           | `[status][len:u32]`   |

**response codes:**
- `0x00` = success
- `0xFF` = error

**encoding:**
- integers: little-endian
- keylen, vallen: u16, u32
- compressed: 0=no, 1=zstd

### example (typescript)

```typescript
import { MiniVaultBinary } from './examples/typescript/binary';

const vault = new MiniVaultBinary('localhost:3000', 'optional-auth-key');

await vault.set('user:123', Buffer.from(JSON.stringify({ name: 'alice' })));
const data = await vault.get('user:123');
await vault.delete('user:123');

const health = await vault.health();
console.log(`cache: ${health.cache_items} items, ${health.memory_mb}MB`);
```

### example (go)

```go
client := minivault.NewBinaryClient("localhost:3000", "optional-auth-key")

err := client.Set("user:123", []byte(`{"name":"alice"}`))
data, err := client.Get("user:123")
err := client.Delete("user:123")

health, err := client.Health()
```

see [examples](examples) for binary and http clients in typescript, go, rust and python

## http protocol

optional json-wrapped http interface, enable with `-http 8080`

**endpoints:**
- `PUT /:key` - store value (json body: `{"value": any}`)
- `GET /:key` - retrieve value (json response: `{"success": bool, "data": any}`)
- `DELETE /:key` - remove key
- `GET /health` - cluster status

### example (curl)

```bash
# set
curl -X PUT http://localhost:8080/mykey \
  -H "Content-Type: application/json" \
  -d '{"value": "hello world"}'

# get
curl http://localhost:8080/mykey
# {"success":true,"data":"hello world"}

# delete
curl -X DELETE http://localhost:8080/mykey

# health
curl http://localhost:8080/health
```

### example (typescript)

```typescript
import { MiniVault } from './examples/typescript/http';

const vault = new MiniVault('http://localhost:8080', 'optional-auth-key');

await vault.set('user:123', { name: 'alice', age: 30 });
const user = await vault.get('user:123');  // auto json deserialize
await vault.delete('user:123');

const health = await vault.health();
```

## authentication

```bash
./minivault -auth "secretkey" -authmode writes
```

**modes:**
- `none` - no auth (default)
- `writes` - auth required for SET/DELETE (reads public)
- `all` - auth required for all ops except health

**binary protocol:** send OpAuth (0x06) before operations
**http protocol:** add header `Authorization: Bearer secretkey`

all clients handle auth automatically when key provided

## command-line flags

```
-port 3000           binary protocol tcp port
-http 0              http json port (0=disabled)
-public-url          this node's cluster address (required for multi-node)
-data /data          persistent storage directory
-auth ""             authentication key
-authmode none       auth mode: none|writes|all
-ratelimit 0         ops/sec throttle (0=unlimited)
-cache 512           in-memory cache size (MB)
-workers 50          worker pool size for replication
```

**environment:**
- `CLUSTER_NODES` - comma-separated list of other nodes (e.g., "node1:3000,node2:3000")

## architecture

### storage layers

**L1: in-memory cache**
- 256 shards with rwmutex locks
- LRU eviction based on hit counters
- bloom filter for fast negative lookups
- atomic size tracking

**L2: write-ahead log**
- batched writes (non-blocking channel)
- CRC32 checksums per entry
- atomic compaction (write temp, rename)
- replays on startup for durability

**L3: disk storage**
- hash-based directory structure (2-level)
- xxhash64 for key hashing
- files named by hash (hex encoded)

### replication

**consistent hashing:**
- uses CRC32(key + node) for placement
- 3 replicas per key
- survives 1 node failure

**quorum writes:**
- requires 2/3 nodes to acknowledge
- parallel replication to other nodes
- 50-worker pool for async operations

**node discovery:**
- configured via CLUSTER_NODES env var
- no leader election, all nodes equal
- eventual consistency (30-50ms typical)

### performance optimizations

- connection pooling (10 conns per remote node)
- zstd compression (adaptive, >1KB values)
- tcp nodelay + keepalive
- 512KB read/write buffers
- 50k max concurrent connections
- non-blocking WAL writes

## performance benchmarks

**test environment:**
- cpu: i7-12700K (12 cores, 20 threads)
- ram: 64GB DDR5-4800
- os: Linux 6.6.87 (WSL2)
- go: 1.25.2

### storage layer (raw)

direct storage operations without network overhead

```
BenchmarkStorageSet-20          2217830    537.1 ns/op     0 B/op    0 allocs/op
BenchmarkStorageGet-20         20032183     59.8 ns/op     0 B/op    0 allocs/op
BenchmarkCacheSet-20            1749616    684.6 ns/op   512 B/op    1 allocs/op
BenchmarkCacheGet-20           15963370     75.2 ns/op     0 B/op    0 allocs/op
```

**throughput:**
- set: 1.75M - 2.21M ops/sec
- get (cache hit): 15.9M - 20M ops/sec
- get latency: 60-75ns

### network (binary tcp)

end-to-end including serialization and tcp overhead

**sequential client (1 connection):**
- writes: 334k ops/sec
- reads: 393k ops/sec

**concurrent (10 clients):**
- mixed workload: 55k ops/sec total

### http protocol

json-wrapped http layer

**single worker:**
- ~100k req/sec (direct storage access)
- includes json serialization overhead

## examples

client implementations in 4 languages, located in `/examples`

| language   | binary tcp      | http json         |
|------------|-----------------|-------------------|
| go         | `go/binary.go`  | `go/http.go`      |
| typescript | `typescript/binary.ts` | `typescript/http.ts` |
| python     | `python/minivault_binary.py` | `python/minivault_http.py` |
| rust       | `rust/binary.rs` | `rust/http.rs`   |

all clients support:
- connection pooling
- automatic authentication
- json helpers (marshal/unmarshal)
- health checks
- timeouts

see `examples/README.md` for detailed usage and more examples

## build

**from source:**
```bash
# regular build
make build

# optimized build (stripped, smaller binary)
make build-optimized

# run tests
make test

# run benchmarks
make bench
```

**docker:**
```bash
# build image
make docker-build

# run 3-node cluster
make docker-run
```

**prebuilt binaries:**
- `minivault` - regular build (9MB)
- `minivault-optimized` - stripped symbols (6.2MB)

both binaries included in repo for convenience

## production deployment

**single node (development):**
```bash
./minivault -port 3000 -data ./data
```

**3-node cluster (production):**
```bash
# node 1
CLUSTER_NODES="node2.example.com:3000,node3.example.com:3000" \
  ./minivault \
    -port 3000 \
    -http 8080 \
    -public-url "node1.example.com:3000" \
    -data /var/lib/minivault \
    -auth "production-secret-key" \
    -authmode writes \
    -ratelimit 100000 \
    -cache 2048

# node 2, node 3 - same pattern with different CLUSTER_NODES
```

**docker compose:**
```bash
docker-compose up -d
# exposes:
#   - vault1: localhost:3001
#   - vault2: localhost:3002
#   - vault3: localhost:3003
```

## configuration

### cache sizing

default 512MB, adjust based on working set:
```bash
-cache 2048  # 2GB cache
```

cache eviction triggers when size exceeds limit, removes coldest 10% of keys

### rate limiting

protect against overload:
```bash
-ratelimit 100000  # 100k ops/sec max
```

uses token bucket algorithm with burst allowance (10% of limit)

### worker pool

controls concurrent replication operations:
```bash
-workers 100  # more workers = higher replication throughput
```

default 50 is good for most deployments

## limits

| limit                | value   | note                          |
|----------------------|---------|-------------------------------|
| max value size       | 100MB   | enforced on set operations    |
| max cache size       | 512MB   | configurable via `-cache`     |
| max connections      | 50,000  | per node, tcp semaphore       |
| write timeout        | 30s     | for replication operations    |
| eventual consistency | 30-50ms | typical convergence time      |

## troubleshooting

**slow writes:**
- check network latency between nodes
- increase `-workers` for parallel replication
- verify quorum (2/3 nodes) is reachable

**high memory usage:**
- reduce `-cache` size
- check for large values (>10MB)
- monitor with `/health` endpoint

**connection failures:**
- verify `CLUSTER_NODES` addresses are correct
- check firewall rules for tcp port
- ensure `-public-url` is reachable from other nodes

**data loss after crash:**
- WAL should replay on restart
- check logs for replay errors
- verify disk is not full
## license

MIT - see LICENSE file

inspired by geohot's minikeyvalue, rewritten for actual production use
