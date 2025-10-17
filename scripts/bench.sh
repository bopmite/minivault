#!/bin/bash
set -e

go test -bench=. -benchmem ../tests/...
