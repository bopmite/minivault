#!/bin/bash

set -e

MASTER_PORT=3000
VOLUME_PORTS="3001,3002,3003"
DATA_DIR="./data"
AUTH_KEY=""
BINARY="./minivault"

while [[ $# -gt 0 ]]; do
  case $1 in
    --master-port) MASTER_PORT="$2"; shift 2 ;;
    --volume-ports) VOLUME_PORTS="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --auth) AUTH_KEY="$2"; shift 2 ;;
    --binary) BINARY="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

IFS=',' read -ra PORTS <<< "$VOLUME_PORTS"

VOLUME_URLS=""
for PORT in "${PORTS[@]}"; do
  if [ -n "$VOLUME_URLS" ]; then
    VOLUME_URLS="$VOLUME_URLS,http://localhost:$PORT"
  else
    VOLUME_URLS="http://localhost:$PORT"
  fi
done

MASTER_URL="http://localhost:$MASTER_PORT"

echo "Starting master server on port $MASTER_PORT..."
$BINARY -mode master -port $MASTER_PORT -volumes "$VOLUME_URLS" &
MASTER_PID=$!

sleep 1

for i in "${!PORTS[@]}"; do
  PORT="${PORTS[$i]}"
  VOLUME_DATA="$DATA_DIR/volume$i"
  mkdir -p "$VOLUME_DATA"

  echo "Starting volume $i on port $PORT..."
  AUTH_ARG=""
  if [ -n "$AUTH_KEY" ]; then
    AUTH_ARG="-auth $AUTH_KEY"
  fi

  $BINARY -mode worker -port $PORT -master "$MASTER_URL" -data "$VOLUME_DATA" $AUTH_ARG &
done

echo "All services started. Master PID: $MASTER_PID"
echo "Press Ctrl+C to stop all services"

trap "kill 0" EXIT
wait
