package main

import "sync"

var (
	pool256  = sync.Pool{New: func() any { return make([]byte, 256) }}
	pool1k   = sync.Pool{New: func() any { return make([]byte, 1024) }}
	pool4k   = sync.Pool{New: func() any { return make([]byte, 4096) }}
	pool16k  = sync.Pool{New: func() any { return make([]byte, 16384) }}
	pool64k  = sync.Pool{New: func() any { return make([]byte, 65536) }}
	pool256k = sync.Pool{New: func() any { return make([]byte, 262144) }}
	pool1m   = sync.Pool{New: func() any { return make([]byte, 1048576) }}
)

func getbuf(size int) []byte {
	switch {
	case size <= 256:
		return pool256.Get().([]byte)[:size]
	case size <= 1024:
		return pool1k.Get().([]byte)[:size]
	case size <= 4096:
		return pool4k.Get().([]byte)[:size]
	case size <= 16384:
		return pool16k.Get().([]byte)[:size]
	case size <= 65536:
		return pool64k.Get().([]byte)[:size]
	case size <= 262144:
		return pool256k.Get().([]byte)[:size]
	case size <= 1048576:
		return pool1m.Get().([]byte)[:size]
	default:
		return make([]byte, size)
	}
}

func putbuf(buf []byte) {
	if buf == nil {
		return
	}
	c := cap(buf)
	switch c {
	case 256:
		pool256.Put(buf[:256])
	case 1024:
		pool1k.Put(buf[:1024])
	case 4096:
		pool4k.Put(buf[:4096])
	case 16384:
		pool16k.Put(buf[:16384])
	case 65536:
		pool64k.Put(buf[:65536])
	case 262144:
		pool256k.Put(buf[:262144])
	case 1048576:
		pool1m.Put(buf[:1048576])
	}
}
