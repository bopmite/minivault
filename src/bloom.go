package main

import "sync/atomic"

const cacheLineSize = 64

type paddedUint64 struct {
	val uint64
	_   [cacheLineSize - 8]byte
}

type bloom struct {
	bits []paddedUint64
	k    uint32
}

func newBloom(n int) *bloom {
	size := max(n*10/64, 1024)
	return &bloom{bits: make([]paddedUint64, size), k: 3}
}

func (b *bloom) add(h uint64) {
	h1, h2 := uint32(h), uint32(h>>32)
	for i := uint32(0); i < b.k; i++ {
		pos := (h1 + i*h2) % uint32(len(b.bits)*64)
		idx, bit := pos/64, pos%64
		for {
			old := atomic.LoadUint64(&b.bits[idx].val)
			new := old | (1 << bit)
			if old == new || atomic.CompareAndSwapUint64(&b.bits[idx].val, old, new) {
				break
			}
		}
	}
}

func (b *bloom) has(h uint64) bool {
	h1, h2 := uint32(h), uint32(h>>32)
	for i := uint32(0); i < b.k; i++ {
		pos := (h1 + i*h2) % uint32(len(b.bits)*64)
		idx, bit := pos/64, pos%64
		if atomic.LoadUint64(&b.bits[idx].val)&(1<<bit) == 0 {
			return false
		}
	}
	return true
}
