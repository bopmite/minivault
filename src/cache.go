package main

import (
	"container/heap"
	"sync"
	"sync/atomic"
)

const shards = 256

type entry struct {
	data []byte
	hits uint32
}

type shard struct {
	mu sync.RWMutex
	m  map[uint64]*entry
}

type cache struct {
	shards [shards]*shard
	bloom  *bloom
	size   atomic.Int64
	items  atomic.Int64
}

func newCache(n int) *cache {
	c := &cache{bloom: newBloom(n)}
	for i := range c.shards {
		c.shards[i] = &shard{m: make(map[uint64]*entry, n/shards)}
	}
	return c
}

func (c *cache) set(h uint64, data []byte) {
	s := c.shards[h%shards]
	s.mu.Lock()
	if old, ok := s.m[h]; ok {
		c.size.Add(int64(len(data) - len(old.data)))
		old.data = data
		s.m[h] = old
	} else {
		s.m[h] = &entry{data: data}
		c.size.Add(int64(len(data)))
		c.items.Add(1)
		c.bloom.add(h)
	}
	s.mu.Unlock()
}

func (c *cache) get(h uint64) ([]byte, bool) {
	if !c.bloom.has(h) {
		return nil, false
	}
	s := c.shards[h%shards]
	s.mu.RLock()
	e, ok := s.m[h]
	if !ok {
		s.mu.RUnlock()
		return nil, false
	}
	data := make([]byte, len(e.data))
	copy(data, e.data)
	atomic.AddUint32(&e.hits, 1)
	s.mu.RUnlock()

	return data, true
}

func (c *cache) del(h uint64) int64 {
	s := c.shards[h%shards]
	s.mu.Lock()
	size := int64(0)
	if e, ok := s.m[h]; ok {
		size = int64(len(e.data))
		c.size.Add(-size)
		c.items.Add(-1)
		delete(s.m, h)
	}
	s.mu.Unlock()
	return size
}

func (c *cache) has(h uint64) bool {
	if !c.bloom.has(h) {
		return false
	}
	s := c.shards[h%shards]
	s.mu.RLock()
	_, ok := s.m[h]
	s.mu.RUnlock()
	return ok
}

type evictItem struct {
	h    uint64
	hits uint32
	size int
}

type minHeap []evictItem

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].hits < h[j].hits }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(evictItem)) }
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (c *cache) evict(max int64) int64 {
	if c.size.Load() < max {
		return 0
	}

	n := int(c.items.Load() / 4)
	if n == 0 {
		n = 1
	}

	h := make(minHeap, 0, n*2)
	for i := range c.shards {
		shard := c.shards[i]
		shard.mu.RLock()
		for hash, e := range shard.m {
			if len(h) < n*2 {
				heap.Push(&h, evictItem{
					h:    hash,
					hits: atomic.LoadUint32(&e.hits),
					size: len(e.data),
				})
			} else if atomic.LoadUint32(&e.hits) < h[0].hits {
				h[0] = evictItem{h: hash, hits: atomic.LoadUint32(&e.hits), size: len(e.data)}
				heap.Fix(&h, 0)
			}
		}
		shard.mu.RUnlock()
	}

	freed := int64(0)
	for i := 0; i < n && h.Len() > 0; i++ {
		item := heap.Pop(&h).(evictItem)
		freed += c.del(item.h)
	}
	return freed
}
