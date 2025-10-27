#!/bin/bash
set -e

cd "$(dirname "$0")/.."
go test -bench=. -benchmem -benchtime=2s ./tests/...
