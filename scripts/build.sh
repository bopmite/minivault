#!/bin/bash
set -e

cd "$(dirname "$0")/.."

go mod tidy
go mod download
go build -o minivault ./src
