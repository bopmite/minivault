package main

import "sync"

var (
	pool1k   = sync.Pool{New: func() any { return make([]byte, 1024) }}
	pool16k  = sync.Pool{New: func() any { return make([]byte, 16384) }}
	pool256k = sync.Pool{New: func() any { return make([]byte, 262144) }}
	pool1m   = sync.Pool{New: func() any { return make([]byte, 1048576) }}
)

func putbuf(buf []byte) {
	c := cap(buf)
	switch c {
	case 1024:
		pool1k.Put(buf[:1024])
	case 16384:
		pool16k.Put(buf[:16384])
	case 262144:
		pool256k.Put(buf[:262144])
	case 1048576:
		pool1m.Put(buf[:1048576])
	}
}
