package main

import (
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

func (c *cache) evict(max int64) {
	if c.size.Load() < max {
		return
	}

	type item struct {
		h    uint64
		hits uint32
		size int
	}

	var all []item
	for i := range c.shards {
		s := c.shards[i]
		s.mu.RLock()
		for h, e := range s.m {
			all = append(all, item{h, atomic.LoadUint32(&e.hits), len(e.data)})
		}
		s.mu.RUnlock()
	}

	// evict 25% of items with lowest hits
	n := len(all) / 4
	if n == 0 {
		return
	}

	for i := 0; i < n; i++ {
		min := i
		for j := i + 1; j < len(all); j++ {
			if all[j].hits < all[min].hits {
				min = j
			}
		}
		all[i], all[min] = all[min], all[i]
		c.del(all[i].h)
	}
}
