package main

import (
	"github.com/klauspost/compress/zstd"
	"sync"
)

var (
	encoderPool = sync.Pool{
		New: func() any {
			enc, _ := zstd.NewWriter(nil,
				zstd.WithEncoderLevel(zstd.SpeedFastest),
				zstd.WithWindowSize(128*1024),
			)
			return enc
		},
	}

	decoderPool = sync.Pool{
		New: func() any {
			dec, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(0))
			return dec
		},
	}
)

func compress(data []byte) []byte {
	if len(data) < 1024 {
		return data
	}

	enc := encoderPool.Get().(*zstd.Encoder)
	defer encoderPool.Put(enc)

	enc.Reset(nil)
	compressed := enc.EncodeAll(data, getbuf(len(data))[:0])

	if len(compressed) >= len(data) {
		putbuf(compressed)
		return data
	}

	return compressed
}

func decompress(data []byte, compressed bool) ([]byte, error) {
	if !compressed {
		return data, nil
	}

	dec := decoderPool.Get().(*zstd.Decoder)
	defer decoderPool.Put(dec)

	return dec.DecodeAll(data, nil)
}
