package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

type Storage struct {
	dir   string
	cache sync.Map
	mu    sync.RWMutex
	size  atomic.Int64
}

func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	s := &Storage{dir: dir}
	go s.loadCache()

	return s, nil
}

func (s *Storage) loadCache() {
	filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, ".tmp") {
			return nil
		}

		key := filepath.Base(path)
		if strings.HasSuffix(key, ".gz") {
			key = key[:len(key)-3]
		}

		s.cache.Store(key, true)
		s.size.Add(info.Size())
		return nil
	})
}

func (s *Storage) Set(key string, value []byte) error {
	if len(value) > MaxValueSize {
		return fmt.Errorf("value too large")
	}

	hash := hashKey(key)
	path := filepath.Join(s.dir, hash)
	tmpPath := path + ".tmp"

	data := value
	if len(value) > CompressionThreshold {
		buf := new(bytes.Buffer)
		gw := gzip.NewWriter(buf)
		if _, err := gw.Write(value); err == nil {
			gw.Close()
			if buf.Len() < len(value) {
				data = buf.Bytes()
				path += ".gz"
				tmpPath += ".gz"
			}
		}
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	f.Close()

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	s.cache.Store(key, true)
	s.size.Add(int64(len(data)))

	return nil
}

func (s *Storage) Get(key string) ([]byte, error) {
	hash := hashKey(key)

	path := filepath.Join(s.dir, hash+".gz")
	data, err := os.ReadFile(path)
	if err == nil {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		return io.ReadAll(gr)
	}

	path = filepath.Join(s.dir, hash)
	return os.ReadFile(path)
}

func (s *Storage) Delete(key string) error {
	hash := hashKey(key)
	os.Remove(filepath.Join(s.dir, hash))
	os.Remove(filepath.Join(s.dir, hash+".gz"))
	s.cache.Delete(key)
	return nil
}

func (s *Storage) Exists(key string) bool {
	_, ok := s.cache.Load(key)
	return ok
}

func (s *Storage) Count() int64 {
	count := int64(0)
	s.cache.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
