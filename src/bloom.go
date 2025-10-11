package main

import "sync/atomic"

type bloom struct {
	bits []uint64
	k    uint32
}

func newBloom(n int) *bloom {
	size := n * 10 / 64
	if size < 1024 {
		size = 1024
	}
	return &bloom{bits: make([]uint64, size), k: 3}
}

func (b *bloom) add(h uint64) {
	h1, h2 := uint32(h), uint32(h>>32)
	for i := uint32(0); i < b.k; i++ {
		pos := (h1 + i*h2) % uint32(len(b.bits)*64)
		idx, bit := pos/64, pos%64
		atomic.AddUint64(&b.bits[idx], 1<<bit)
	}
}

func (b *bloom) has(h uint64) bool {
	h1, h2 := uint32(h), uint32(h>>32)
	for i := uint32(0); i < b.k; i++ {
		pos := (h1 + i*h2) % uint32(len(b.bits)*64)
		idx, bit := pos/64, pos%64
		if atomic.LoadUint64(&b.bits[idx])&(1<<bit) == 0 {
			return false
		}
	}
	return true
}
