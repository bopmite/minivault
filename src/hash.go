package main

import "github.com/cespare/xxhash/v2"

func hash64str(s string) uint64 { return xxhash.Sum64String(s) }
func hash64(b []byte) uint64     { return xxhash.Sum64(b) }
