package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/OneOfOne/xxhash"
)

const (
	HotMax  = 64 * 1024
	WarmMax = 4 * 1024 * 1024
	ColdMax = 1024 * 1024 * 1024
	HotMem  = 512 * 1024 * 1024
	ChunkSz = 1024 * 1024
)

type Storage struct {
	hot     sync.Map
	warm    *WarmCache
	cold    *ColdStore
	hotSize atomic.Int64
	dataDir string
}

type WarmCache struct {
	mu    sync.RWMutex
	files map[string]*MmapFile
	dir   string
}

type MmapFile struct {
	data []byte
	size int
}

type ColdStore struct {
	dir string
}

type ChunkMeta struct {
	Key    string      `json:"k"`
	Size   int64       `json:"s"`
	Chunks []ChunkInfo `json:"c"`
}

type ChunkInfo struct {
	Hash string `json:"h"`
	Size int    `json:"s"`
}

var storage *Storage

func InitStorage(dir string) {
	os.MkdirAll(filepath.Join(dir, "hot"), 0755)
	os.MkdirAll(filepath.Join(dir, "warm"), 0755)
	os.MkdirAll(filepath.Join(dir, "cold"), 0755)

	storage = &Storage{
		dataDir: dir,
		warm:    &WarmCache{files: make(map[string]*MmapFile), dir: filepath.Join(dir, "warm")},
		cold:    &ColdStore{dir: filepath.Join(dir, "cold")},
	}
}

func Set(key string, val []byte) error {
	size := len(val)

	if size < HotMax {
		storage.hot.Store(key, val)
		storage.hotSize.Add(int64(size))

		if storage.hotSize.Load() > HotMem {
			evictHot()
		}

		return nil
	}

	if size < WarmMax {
		return storage.warm.Set(key, val)
	}

	return storage.cold.Set(key, val)
}

func Get(key string) []byte {
	if val, ok := storage.hot.Load(key); ok {
		return val.([]byte)
	}

	if val := storage.warm.Get(key); val != nil {
		if len(val) < HotMax {
			storage.hot.Store(key, val)
			storage.hotSize.Add(int64(len(val)))
		}

		return val
	}

	return storage.cold.Get(key)
}

func Delete(key string) bool {
	storage.hot.Delete(key)
	storage.warm.Delete(key)
	storage.cold.Delete(key)
	return true
}

func evictHot() {
	var toEvict []struct {
		key string
		val []byte
	}

	storage.hot.Range(func(key, val interface{}) bool {
		toEvict = append(toEvict, struct {
			key string
			val []byte
		}{key.(string), val.([]byte)})
		return len(toEvict) < 100
	})

	for _, item := range toEvict {
		storage.hot.Delete(item.key)
		storage.hotSize.Add(-int64(len(item.val)))
		storage.warm.Set(item.key, item.val)
	}
}

func (w *WarmCache) Set(key string, val []byte) error {
	h := xxhash.ChecksumString64(key)
	path := filepath.Join(w.dir, fmt.Sprintf("%016x", h))

	if err := os.WriteFile(path, val, 0644); err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.files[key] != nil {
		syscall.Munmap(w.files[key].data)
		delete(w.files, key)
	}

	return nil
}

func (w *WarmCache) Get(key string) []byte {
	h := xxhash.ChecksumString64(key)
	path := filepath.Join(w.dir, fmt.Sprintf("%016x", h))

	w.mu.RLock()
	if mf, exists := w.files[key]; exists {
		w.mu.RUnlock()
		return mf.data[:mf.size]
	}
	w.mu.RUnlock()

	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	size := int(info.Size())
	data, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil
	}

	w.mu.Lock()
	w.files[key] = &MmapFile{data: data, size: size}
	w.mu.Unlock()

	return data[:size]
}

func (w *WarmCache) Delete(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if mf, exists := w.files[key]; exists {
		syscall.Munmap(mf.data)
		delete(w.files, key)
	}

	h := xxhash.ChecksumString64(key)
	os.Remove(filepath.Join(w.dir, fmt.Sprintf("%016x", h)))
}

func (c *ColdStore) Set(key string, val []byte) error {
	if len(val) < WarmMax {
		h := xxhash.ChecksumString64(key)
		return os.WriteFile(filepath.Join(c.dir, fmt.Sprintf("%016x", h)), val, 0644)
	}

	chunks := splitChunks(val)
	meta := ChunkMeta{
		Key:    key,
		Size:   int64(len(val)),
		Chunks: make([]ChunkInfo, len(chunks)),
	}

	for i, chunk := range chunks {
		hash := sha256.Sum256(chunk)
		hashStr := hex.EncodeToString(hash[:])

		if err := os.WriteFile(filepath.Join(c.dir, hashStr), chunk, 0644); err != nil {
			return err
		}

		meta.Chunks[i] = ChunkInfo{Hash: hashStr, Size: len(chunk)}
	}

	metaJSON, _ := json.Marshal(meta)
	h := xxhash.ChecksumString64(key)

	return os.WriteFile(filepath.Join(c.dir, fmt.Sprintf("%016x.meta", h)), metaJSON, 0644)
}

func (c *ColdStore) Get(key string) []byte {
	h := xxhash.ChecksumString64(key)
	metaPath := filepath.Join(c.dir, fmt.Sprintf("%016x.meta", h))
	metaData, err := os.ReadFile(metaPath)

	if err != nil {
		data, err := os.ReadFile(filepath.Join(c.dir, fmt.Sprintf("%016x", h)))
		if err != nil {
			return nil
		}

		return data
	}

	var meta ChunkMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil
	}

	result := make([]byte, 0, meta.Size)

	for _, chunk := range meta.Chunks {
		data, err := os.ReadFile(filepath.Join(c.dir, chunk.Hash))
		if err != nil {
			return nil
		}

		result = append(result, data...)
	}

	return result
}

func (c *ColdStore) Delete(key string) {
	h := xxhash.ChecksumString64(key)
	metaPath := filepath.Join(c.dir, fmt.Sprintf("%016x.meta", h))
	metaData, err := os.ReadFile(metaPath)

	if err == nil {
		var meta ChunkMeta
		if json.Unmarshal(metaData, &meta) == nil {
			for _, chunk := range meta.Chunks {
				os.Remove(filepath.Join(c.dir, chunk.Hash))
			}
		}

		os.Remove(metaPath)
	}

	os.Remove(filepath.Join(c.dir, fmt.Sprintf("%016x", h)))
}

func splitChunks(data []byte) [][]byte {
	var chunks [][]byte

	for i := 0; i < len(data); i += ChunkSz {
		end := min(i + ChunkSz, len(data))

		chunks = append(chunks, data[i:end])
	}

	return chunks
}
