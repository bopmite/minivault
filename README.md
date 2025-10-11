# minivault

**sub 1000 line distributed key-value store with geo-replication support**

Personal recreation of George Hotz's [minikeyvalue](https://github.com/geohot/minikeyvalue) project

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

**Benchmarks (single node):**
- Small GET: <5μs (hot cache)
- Medium GET: <100μs (mmap)
- Large GET: <10ms (chunked)
- Throughput: 10k+ writes/sec