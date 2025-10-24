package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

type Storage struct {
	dir     string
	cache   *cache
	wal     *wal
	size    atomic.Int64
	maxSize int64
}

func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	w, err := newWAL(dir)
	if err != nil {
		return nil, err
	}

	s := &Storage{
		dir:     dir,
		cache:   newCache(100000),
		wal:     w,
		maxSize: MaxCacheSize,
	}

	if err := s.replayWAL(); err != nil {
		return nil, err
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	if err := w.truncate(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Storage) replayWAL() error {
	return s.wal.replay(func(h uint64, data []byte) error {
		if len(data) == 0 {
			return nil
		}
		path := s.getPath(h)
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
		s.cache.set(h, data)
		s.size.Add(int64(len(data)))
		return nil
	})
}

func (s *Storage) load() error {
	return filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) == ".log" || filepath.Ext(path) == ".tmp" {
			return nil
		}

		h := parseHex(filepath.Base(path))
		if data, err := os.ReadFile(path); err == nil {
			s.cache.set(h, data)
			s.size.Add(info.Size())
		}
		return nil
	})
}

func (s *Storage) getPath(h uint64) string {
	hex := fmtHex(h)
	subdir := filepath.Join(s.dir, hex[:2])
	os.MkdirAll(subdir, 0755)
	return filepath.Join(subdir, hex)
}

func (s *Storage) Set(key string, value []byte) error {
	if len(value) > MaxValueSize {
		return fmt.Errorf("too large")
	}

	h := hash64str(key)
	oldSize := int64(0)
	if old, ok := s.cache.get(h); ok {
		oldSize = int64(len(old))
	} else {
		path := s.getPath(h)
		if info, err := os.Stat(path); err == nil {
			oldSize = info.Size()
		}
	}

	s.wal.append(h, value)
	s.cache.set(h, value)
	delta := int64(len(value)) - oldSize
	s.size.Add(delta)

	path := s.getPath(h)
	if err := os.WriteFile(path, value, 0644); err != nil {
		s.size.Add(-delta)
		return err
	}

	if s.size.Load() > s.maxSize {
		s.cache.evict(s.maxSize)
	}

	return nil
}

func (s *Storage) Get(key string) ([]byte, error) {
	h := hash64str(key)

	if data, ok := s.cache.get(h); ok {
		return data, nil
	}

	path := s.getPath(h)
	data, err := os.ReadFile(path)
	if err == nil {
		s.cache.set(h, data)
		return data, nil
	}

	return nil, fmt.Errorf("not found")
}

func (s *Storage) Delete(key string) error {
	h := hash64str(key)

	size := int64(0)
	if data, ok := s.cache.get(h); ok {
		size = int64(len(data))
	} else {
		path := s.getPath(h)
		if info, err := os.Stat(path); err == nil {
			size = info.Size()
		}
	}

	if size > 0 {
		s.size.Add(-size)
	}

	s.wal.append(h, nil)
	s.cache.del(h)
	os.Remove(s.getPath(h))
	return nil
}

func (s *Storage) Exists(key string) bool {
	h := hash64str(key)
	return s.cache.has(h)
}

func (s *Storage) Count() int64 {
	return s.cache.items.Load()
}

func (s *Storage) Close() {
	s.wal.close()
}

func fmtHex(h uint64) string {
	const hex = "0123456789abcdef"
	var buf [16]byte
	for i := 15; i >= 0; i-- {
		buf[i] = hex[h&0xf]
		h >>= 4
	}
	return string(buf[:])
}

func parseHex(s string) uint64 {
	var h uint64
	for i := 0; i < len(s) && i < 16; i++ {
		h <<= 4
		c := s[i]
		if c >= '0' && c <= '9' {
			h |= uint64(c - '0')
		} else if c >= 'a' && c <= 'f' {
			h |= uint64(c - 'a' + 10)
		}
	}
	return h
}
