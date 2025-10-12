# minivault

**sub 1500 line distributed key-value store with geo-replication support**

Recreation of George Hotz's [minikeyvalue](https://github.com/geohot/minikeyvalue) project

## API

- `GET /key` - Retrieve value (404 if not found)
- `POST /key` - Create value (409 if exists)
- `PUT /key` - Upsert value
- `DELETE /key` - Remove value

**Request format:** `{"value": <data>}`

**Response format:**
```json
{"success": true, "data": <value>}
{"success": false, "error": "message"}
```

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

*(benchmarked with every commit)*

**Test Environment:**
- CPU: Intel Core i7-12700K (12 cores, 20 threads)
- RAM: 16GB DDR5
- OS: Linux 6.6.87 (WSL2)
- Go: 1.25.2

**Storage Layer (WAL + Cache + Bloom):**
- Write 1KB: 390ns/op (2.6 GB/s)
- Write 100KB: 388ns/op (264 GB/s)
- Write 1MB: 383ns/op (2.7 TB/s)
- Read 1KB (cache hit): 60ns/op (17 GB/s)
- Read 100KB (cache hit): 48ns/op (2.1 TB/s)
- Read 1MB (cache hit): 54ns/op (19.3 TB/s)

**HTTP API:**
- PUT 1KB: 2.8μs/req (367 MB/s)
- PUT 100KB: 101μs/req (1.0 GB/s)
- PUT 1MB: 858μs/req (1.2 GB/s)
- GET 1KB: 2.3μs/req (442 MB/s)
- GET 100KB: 12.8μs/req (7.9 GB/s)
- GET 1MB: 131μs/req (7.9 GB/s)

**Throughput:**
- Sequential Writes: 350k ops/sec
- Sequential Reads: 457k ops/sec
- Concurrent Writes (1KB): 1.0 GB/s (991ns/op)
- Concurrent Reads (1KB): 1.0 GB/s (1011ns/op)
- Cache Hit Rate: 20.2 GB/s (50ns/op)
- WAL Batch Writes: 617 MB/s (415ns/op)