#!/bin/bash
set -e

go mod tidy
go mod download
go build -o minivault src/*.go
