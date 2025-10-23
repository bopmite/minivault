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
		old.hits = 0
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
	s.mu.RUnlock()
	if ok {
		atomic.AddUint32(&e.hits, 1)
		return e.data, true
	}
	return nil, false
}

func (c *cache) del(h uint64) {
	s := c.shards[h%shards]
	s.mu.Lock()
	if e, ok := s.m[h]; ok {
		c.size.Add(-int64(len(e.data)))
		c.items.Add(-1)
		delete(s.m, h)
	}
	s.mu.Unlock()
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

func (c *cache) evict(max int64) {
	if c.size.Load() < max {
		return
	}

	n := int(c.items.Load() / 4)
	if n == 0 {
		return
	}

	h := make(minHeap, 0, c.items.Load())
	for i := range c.shards {
		shard := c.shards[i]
		shard.mu.RLock()
		for hash, e := range shard.m {
			h = append(h, evictItem{
				h:    hash,
				hits: atomic.LoadUint32(&e.hits),
				size: len(e.data),
			})
		}
		shard.mu.RUnlock()
	}

	heap.Init(&h)
	for i := 0; i < n && h.Len() > 0; i++ {
		item := heap.Pop(&h).(evictItem)
		c.del(item.h)
	}
}
