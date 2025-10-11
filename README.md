# minivault

**sub 1500 line distributed key-value store with geo-replication support**

Recreation of George Hotz's [minikeyvalue](https://github.com/geohot/minikeyvalue) project

## API

- `GET /key` - Retrieve value (404 if not found)
- `POST /key` - Create value (409 if exists)
- `PUT /key` - Upsert value
- `DELETE /key` - Remove value

All requests accept/return JSON: `{"value": <data>}`

## Architecture

```
                    MASTER SERVER
                  (Public IP/DNS)
                  Coordinates cluster
                         |
        ┌────────────────┼────────────────┐
        |                |                |
   WORKER EU        WORKER US        WORKER ASIA
   (Public IP)      (Public IP)      (Public IP)
```


## Quick Start

### Local Cluster (easiest)

```bash
# Start master with 3 volumes
VOLUMES=localhost:3001,localhost:3002,localhost:3003 PORT=3000 ./mkv &

# Start 3 volumes
PORT=3001 MASTER=http://localhost:3000 ./volume /tmp/volume1 &
PORT=3002 MASTER=http://localhost:3000 ./volume /tmp/volume2 &
PORT=3003 MASTER=http://localhost:3000 ./volume /tmp/volume3 &

# Wait for cluster sync (5s)
sleep 6

# Test it
curl -X POST http://localhost:3001/test -d '{"value": "hello"}' -H "Content-Type: application/json"
curl http://localhost:3002/test # replicated across volume 1 -> 2
```

### Single Node

```bash
./volume /tmp/data
```

### Geo-Distributed Cluster

**Start Master:**
```bash
./minivault -mode master -port 8080
```

**Start Workers:**
```bash
# EU
./minivault -port 3000 -public-url "https://eu.domain.com" -master "http://master:8080"

# US
./minivault -port 3000 -public-url "https://us.domain.com" -master "http://master:8080"

# Asia
./minivault -port 3000 -public-url "https://asia.domain.com" -master "http://master:8080"
```

## Usage

```bash
# Store value
curl -X POST http://localhost:3000/mykey \
  -H "Content-Type: application/json" \
  -d '{"value": "hello world"}'

# Retrieve value
curl http://localhost:3000/mykey

# Update value
curl -X PUT http://localhost:3000/mykey \
  -H "Content-Type: application/json" \
  -d '{"value": "updated"}'

# Delete value
curl -X DELETE http://localhost:3000/mykey

# Store file
curl -X PUT http://localhost:3000/file.jpg \
  -H "Content-Type: application/json" \
  -d "{\"value\": \"$(base64 file.jpg)\"}"
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

## Flags

**Worker Mode (default):**
- `-port` - Port to listen on (default: 3000)
- `-public-url` - Public URL for this worker
- `-master` - Master server URL (required for geo-distributed)
- `-data` - Data directory (default: ./data)

**Master Mode:**
- `-mode master` - Run as master server
- `-port` - Port to listen on (default: 3000)

## Performance

**Test Environment:**
- CPU: Intel Core i7-12700K (10 cores, 20 threads)
- RAM: 16GB
- OS: Linux 6.6.87 (WSL2)
- Go: 1.25.2

**Storage Layer (WAL + Cache + Bloom):**
- Write 1KB: 465ns/op (2.2 GB/s)
- Write 100KB: 412ns/op (24.8 GB/s)
- Write 1MB: 518ns/op (2 TB/s)
- Read 1KB (cache hit): 74ns/op (13.8 GB/s)
- Read 100KB (cache hit): 53ns/op (1.9 TB/s)
- Read 1MB (cache hit): 58ns/op (17.9 TB/s)

**HTTP API (Single-Threaded):**
- PUT 1KB: 3.5μs/req (295 MB/s)
- PUT 100KB: 128μs/req (797 MB/s)
- PUT 1MB: 1.1ms/req (936 MB/s)
- GET 1KB: 2.7μs/req (372 MB/s)
- GET 100KB: 19μs/req (5.3 GB/s)
- GET 1MB: 179μs/req (5.8 GB/s)

**Throughput:**
- Sequential Writes: 347,147 ops/sec
- Sequential Reads: 446,748 ops/sec
- Concurrent Writes: 974 MB/s (1.05μs/op)
- Concurrent Reads: 970 MB/s (1.05μs/op)
- Cache Hits: 19.2 GB/s (53ns/op)