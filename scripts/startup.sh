#!/bin/bash
set -e

PORTS="3001,3002,3003"
DATA_DIR="./data"
AUTH_KEY=""
BINARY="./minivault"

while [[ $# -gt 0 ]]; do
  case $1 in
    --ports) PORTS="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --auth) AUTH_KEY="$2"; shift 2 ;;
    --binary) BINARY="$2"; shift 2 ;;
    *) echo "unknown option: $1"; exit 1 ;;
  esac
done

IFS=',' read -ra PORT_ARRAY <<< "$PORTS"

PIDS=()

for i in "${!PORT_ARRAY[@]}"; do
  PORT="${PORT_ARRAY[$i]}"
  NODE_DATA="$DATA_DIR/node$i"
  mkdir -p "$NODE_DATA"

  CLUSTER_URLS=""
  for j in "${!PORT_ARRAY[@]}"; do
    if [ $i -ne $j ]; then
      if [ -n "$CLUSTER_URLS" ]; then
        CLUSTER_URLS="$CLUSTER_URLS,localhost:${PORT_ARRAY[$j]}"
      else
        CLUSTER_URLS="localhost:${PORT_ARRAY[$j]}"
      fi
    fi
  done

  AUTH_ARG=""
  if [ -n "$AUTH_KEY" ]; then
    AUTH_ARG="-auth $AUTH_KEY"
  fi

  echo "starting node $i on port $PORT"
  CLUSTER_NODES="$CLUSTER_URLS" $BINARY -port $PORT -public-url "localhost:$PORT" -data "$NODE_DATA" $AUTH_ARG &
  PIDS+=($!)
done

echo "all nodes started: ${PIDS[@]}"
echo "press ctrl+c to stop"

trap "kill ${PIDS[@]} 2>/dev/null" EXIT
wait
