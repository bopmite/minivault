#!/bin/bash
go build -ldflags="-s -w" -gcflags="-l=4" -o minivault ./src
