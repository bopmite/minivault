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

### Local Cluster

```bash
./scripts/startup.sh
```

Starts master on port 3000 and 3 volumes on ports 3001-3003.

**Options:**
- `--master-port <port>` - Master server port (default: 3000)
- `--volume-ports <ports>` - Comma-separated volume ports (default: 3001,3002,3003)
- `--data-dir <path>` - Data directory (default: ./data)
- `--auth <key>` - Authentication key (optional)
- `--binary <path>` - Binary path (default: ./minivault)

**Examples:**
```bash
./scripts/startup.sh --volume-ports 4000,4001,4002,4003,4004

./scripts/startup.sh --master-port 8080 --volume-ports 8081,8082,8083

./scripts/startup.sh --auth mykey123 --data-dir /tmp/vault --volume-ports 5000,5001
```

**Test:**
```bash
curl -X POST http://localhost:3001/test -d '{"value": "hello"}' -H "Content-Type: application/json"
curl http://localhost:3002/test
```

Press Ctrl+C to stop all services.

### Single Node

```bash
./minivault -port 3000 -data ./data
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

## Build

```bash
go build -o minivault src/*.go
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
- CPU: AMD Ryzen 7 PRO 7840U (16 cores)
- RAM: 64GB DDR5
- OS: Linux 6.6.87 (WSL2)
- Go: 1.25.2

**Storage Layer (WAL + Cache + Bloom):**
- Write 1KB: 998ns/op (1.0 GB/s, **~1M ops/sec**)
- Write 10KB: 1085ns/op (9.4 GB/s, **920k ops/sec**)
- Write 100KB: 954ns/op (107 GB/s, **1M ops/sec**)
- Write 1MB: 1034ns/op (1014 GB/s, **967k ops/sec**)
- Cache Hit: 63ns/op (16.3 GB/s, **15.9M ops/sec**)
- WAL Batch: 984ns/op (260 MB/s, **~1M ops/sec**)

**HTTP API Throughput:**
- Sequential Writes: **155k ops/sec** (159 MB/s)
- Sequential Reads: **235k ops/sec** (241 MB/s)
- Concurrent Writes: **713k ops/sec** (730 MB/s)
- Concurrent Reads: **776k ops/sec** (794 MB/s)

