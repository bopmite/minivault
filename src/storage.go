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
	entries := make(map[uint64][]byte)
	if err := s.wal.replay(func(h uint64, data []byte) error {
		entries[h] = data
		return nil
	}); err != nil {
		return err
	}

	for h, data := range entries {
		if len(data) == 0 {
			path := s.getPath(h)
			os.Remove(path)
		} else {
			path := s.getPath(h)
			if err := os.WriteFile(path, data, 0644); err != nil {
				return err
			}
			s.cache.set(h, data)
		}
	}
	s.size.Store(s.cache.size.Load())
	return nil
}

func (s *Storage) load() error {
	return filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) == ".log" || filepath.Ext(path) == ".tmp" {
			return nil
		}

		if s.size.Load() >= s.maxSize {
			return nil
		}

		h := parseHex(filepath.Base(path))
		if !s.cache.has(h) {
			if data, err := os.ReadFile(path); err == nil {
				s.cache.set(h, data)
			}
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
	s.wal.append(h, value)
	s.cache.set(h, value)
	s.size.Store(s.cache.size.Load())

	path := s.getPath(h)
	if err := os.WriteFile(path, value, 0644); err != nil {
		return err
	}

	if s.size.Load() > s.maxSize {
		freed := s.cache.evict(s.maxSize)
		s.size.Add(-freed)
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

	s.wal.append(h, nil)
	freed := s.cache.del(h)
	s.size.Add(-freed)
	os.Remove(s.getPath(h))
	return nil
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
