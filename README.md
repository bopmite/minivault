# minivault

**sub 1500 line distributed key-value store with geo-replication support**

Recreation of George Hotz's [minikeyvalue](https://github.com/geohot/minikeyvalue) project

## API

Binary protocol over TCP for maximum performance.

**Operations:**
- `0x01` - Get value
- `0x02` - Set value
- `0x03` - Delete value
- `0x04` - Sync (internal replication)

**Request format:**
```
[1 byte op][2 bytes key length][N bytes key][4 bytes value length][1 byte compressed][M bytes value]
```

**Response format:**
```
[1 byte status (0x00=success, 0xFF=error)][4 bytes value length][M bytes value]
```

## Architecture

```
   NODE 1 (EU)          NODE 2 (US)          NODE 3 (ASIA)
   (Public IP)  <-----> (Public IP)  <-----> (Public IP)
        ^                    ^                    ^
        |                    |                    |
        └────────────────────┴────────────────────┘
              Peer-to-peer mesh (static discovery)
```


## Quick Start

### Single Node

```bash
./minivault -port 3000 -data ./data
```

### Local 3-Node Cluster

```bash
# Node 1
CLUSTER_NODES="localhost:3001,localhost:3002,localhost:3003" \
  ./minivault -port 3001 -data ./data1 -public-url "localhost:3001" &

# Node 2
CLUSTER_NODES="localhost:3001,localhost:3002,localhost:3003" \
  ./minivault -port 3002 -data ./data2 -public-url "localhost:3002" &

# Node 3
CLUSTER_NODES="localhost:3001,localhost:3002,localhost:3003" \
  ./minivault -port 3003 -data ./data3 -public-url "localhost:3003" &
```

### Geo-Distributed Cluster

```bash
# EU Node
CLUSTER_NODES="eu.example.com:3000,us.example.com:3000,asia.example.com:3000" \
  ./minivault -port 3000 -data ./data -public-url "eu.example.com:3000"

# US Node
CLUSTER_NODES="eu.example.com:3000,us.example.com:3000,asia.example.com:3000" \
  ./minivault -port 3000 -data ./data -public-url "us.example.com:3000"

# Asia Node
CLUSTER_NODES="eu.example.com:3000,us.example.com:3000,asia.example.com:3000" \
  ./minivault -port 3000 -data ./data -public-url "asia.example.com:3000"
```

## Usage

Use the BinaryClient for Go applications:

```go
client := NewBinaryClient()

// Set value
err := client.Set("localhost:3000", "mykey", []byte("hello world"))

// Get value
data, err := client.Get("localhost:3000", "mykey")

// Delete value
err := client.Delete("localhost:3000", "mykey")
```

For other languages, implement the binary protocol or use netcat for testing:

```bash
# Raw binary protocol (hex for demonstration)
# Set: [02][key_len][key][val_len][compressed][value]
# Get: [01][key_len][key]
```

## Docker

```bash
# Build
docker build -t minivault:latest .

# Run single node
docker run -p 3000:3000 minivault:latest

# Run 3-node cluster
docker-compose up -d
```

## Build

```bash
go build -o minivault src/*.go
```

## Flags

- `-port` - Port to listen on (default: 3000)
- `-public-url` - Public URL/address for this node (default: localhost:port)
- `-data` - Data directory (default: /data)
- `-auth` - Authentication key (optional)
- `-edge` - Enable edge caching mode
- `-async` - Enable async replication

**Environment Variables:**
- `CLUSTER_NODES` - Comma-separated list of all cluster nodes (e.g., "node1:3000,node2:3000,node3:3000")

## Performance

**Test Environment:**
- CPU: 12th Gen Intel i7-12700K
- RAM: 64GB DDR5
- OS: Linux 6.6.87 (WSL2)
- Go: 1.25.2

**Storage Layer (Optimized with xxHash + Heap Eviction + Cache Line Padding):**
- Set 1KB: 570ns/op (1.8 GB/s, **1.75M ops/sec**)
- Set 10KB: 543ns/op (18.8 GB/s, **1.84M ops/sec**)
- Set 100KB: 495ns/op (207 GB/s, **2.02M ops/sec**)
- Set 1MB: 452ns/op (2318 GB/s, **2.21M ops/sec**)
- Get 1KB: 63ns/op (16 GB/s, **15.9M ops/sec**)
- Get 100KB: 50ns/op (2061 GB/s, **20M ops/sec**)
- Get 1MB: 53ns/op (19617 GB/s, **18.8M ops/sec**)
- Cache Hit: **50ns/op** (20 GB/s, **20M ops/sec**)
- WAL Batch: 463ns/op (553 MB/s, **2.16M ops/sec**)

**Binary Protocol Throughput:**
- Sequential Writes: **334k ops/sec** (343 MB/s)
- Sequential Reads: **393k ops/sec** (403 MB/s)
- Concurrent (10 clients): **55k ops/sec** (18µs avg latency)

