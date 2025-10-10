package tests

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
)

func BenchmarkSmallValueStorage(b *testing.B) {
	data := []byte("small value for benchmarking")
	cache := &sync.Map{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key%d", i)
		cache.Store(key, data)
	}
}

func BenchmarkSmallValueRetrieval(b *testing.B) {
	cache := &sync.Map{}
	data := []byte("benchmark data")

	for i := 0; i < 1000; i++ {
		cache.Store(fmt.Sprintf("key%d", i), data)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key%d", i%1000)
		cache.Load(key)
	}
}

func BenchmarkMediumValueStorage(b *testing.B) {
	data := make([]byte, 100*1024)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		buf := make([]byte, len(data))
		copy(buf, data)
	}
}

func BenchmarkLargeValueChunking(b *testing.B) {
	data := make([]byte, 5*1024*1024)
	rand.Read(data)
	chunkSize := 1024 * 1024

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		chunks := 0
		for offset := 0; offset < len(data); offset += chunkSize {
			end := offset + chunkSize
			if end > len(data) {
				end = len(data)
			}
			_ = data[offset:end]
			chunks++
		}
	}
}

func BenchmarkConcurrentReads(b *testing.B) {
	cache := &sync.Map{}
	data := []byte("concurrent test data")

	for i := 0; i < 10000; i++ {
		cache.Store(fmt.Sprintf("key%d", i), data)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i%10000)
			cache.Load(key)
			i++
		}
	})
}

func BenchmarkConcurrentWrites(b *testing.B) {
	cache := &sync.Map{}
	data := []byte("write benchmark data")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i)
			cache.Store(key, data)
			i++
		}
	})
}

func BenchmarkRendezvousHash(b *testing.B) {
	nodes := []string{"node1", "node2", "node3", "node4", "node5"}
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := keys[i%len(keys)]
		_ = selectNodes(key, nodes, 3)
	}
}

func selectNodes(_ string, nodes []string, count int) []string {
	if count > len(nodes) {
		count = len(nodes)
	}
	return nodes[:count]
}

func BenchmarkJSONMarshaling(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := bytes.Buffer{}
		buf.WriteString("test")
	}
}

func BenchmarkMemoryAllocation(b *testing.B) {
	sizes := []int{1024, 64 * 1024, 1024 * 1024, 4 * 1024 * 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%dKB", size/1024), func(b *testing.B) {
			b.SetBytes(int64(size))
			for i := 0; i < b.N; i++ {
				data := make([]byte, size)
				_ = data
			}
		})
	}
}

func BenchmarkQuorumLogic(b *testing.B) {
	nodeCount := 3
	quorum := (nodeCount / 2) + 1

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		successes := 0
		for j := 0; j < nodeCount; j++ {
			successes++
			if successes >= quorum {
				break
			}
		}
	}
}

func BenchmarkDataCopy(b *testing.B) {
	src := make([]byte, 1024*1024)
	rand.Read(src)

	b.ResetTimer()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		dst := make([]byte, len(src))
		copy(dst, src)
	}
}
