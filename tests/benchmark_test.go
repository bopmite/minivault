package tests

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	KB = 1024
	MB = 1024 * KB
)

func BenchmarkStorage_Set_1KB(b *testing.B) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_%d", time.Now().UnixNano()))
	defer os.RemoveAll(dir)

	s := mustStorage(dir)
	defer s.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		s.Set(fmt.Sprintf("key_%d", i), data)
	}
}

func BenchmarkStorage_Set_10KB(b *testing.B) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_%d", time.Now().UnixNano()))
	defer os.RemoveAll(dir)

	s := mustStorage(dir)
	defer s.Close()

	data := make([]byte, 10*KB)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		s.Set(fmt.Sprintf("key_%d", i), data)
	}
}

func BenchmarkStorage_Set_100KB(b *testing.B) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_%d", time.Now().UnixNano()))
	defer os.RemoveAll(dir)

	s := mustStorage(dir)
	defer s.Close()

	data := make([]byte, 100*KB)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		s.Set(fmt.Sprintf("key_%d", i), data)
	}
}

func BenchmarkStorage_Set_1MB(b *testing.B) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_%d", time.Now().UnixNano()))
	defer os.RemoveAll(dir)

	s := mustStorage(dir)
	defer s.Close()

	data := make([]byte, 1*MB)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		s.Set(fmt.Sprintf("key_%d", i), data)
	}
}

func BenchmarkStorage_Get_1KB(b *testing.B) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_%d", time.Now().UnixNano()))
	defer os.RemoveAll(dir)

	s := mustStorage(dir)
	defer s.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	for i := 0; i < 1000; i++ {
		s.Set(fmt.Sprintf("key_%d", i), data)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		s.Get(fmt.Sprintf("key_%d", i%1000))
	}
}

func BenchmarkStorage_Get_100KB(b *testing.B) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_%d", time.Now().UnixNano()))
	defer os.RemoveAll(dir)

	s := mustStorage(dir)
	defer s.Close()

	data := make([]byte, 100*KB)
	rand.Read(data)

	for i := 0; i < 100; i++ {
		s.Set(fmt.Sprintf("key_%d", i), data)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		s.Get(fmt.Sprintf("key_%d", i%100))
	}
}

func BenchmarkStorage_Get_1MB(b *testing.B) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_%d", time.Now().UnixNano()))
	defer os.RemoveAll(dir)

	s := mustStorage(dir)
	defer s.Close()

	data := make([]byte, 1*MB)
	rand.Read(data)

	for i := 0; i < 10; i++ {
		s.Set(fmt.Sprintf("key_%d", i), data)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		s.Get(fmt.Sprintf("key_%d", i%10))
	}
}

func BenchmarkHTTP_PUT_1KB(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}
}

func BenchmarkHTTP_PUT_100KB(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 100*KB)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}
}

func BenchmarkHTTP_PUT_1MB(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*MB)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}
}

func BenchmarkHTTP_GET_1KB(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	for i := 0; i < 1000; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/key_%d", i%1000), nil)
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}
}

func BenchmarkHTTP_GET_100KB(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 100*KB)
	rand.Read(data)

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/key_%d", i%100), nil)
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}
}

func BenchmarkHTTP_GET_1MB(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*MB)
	rand.Read(data)

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/key_%d", i%10), nil)
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}
}

func BenchmarkHTTP_POST_JSON(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	payload := map[string]string{"value": "test data"}
	jsonData, _ := json.Marshal(payload)

	b.ResetTimer()
	b.SetBytes(int64(len(jsonData)))
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/key_%d", i), bytes.NewReader(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}
}

func BenchmarkConcurrent_Write_1KB(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
			w := httptest.NewRecorder()
			vault.ServeHTTP(w, req)
			i++
		}
	})
}

func BenchmarkConcurrent_Read_1KB(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	for i := 0; i < 10000; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/key_%d", i%10000), nil)
			w := httptest.NewRecorder()
			vault.ServeHTTP(w, req)
			i++
		}
	})
}

func BenchmarkConcurrent_Mixed_Workload(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	for i := 0; i < 1000; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := int64(0)
		for pb.Next() {
			n := atomic.AddInt64(&i, 1)
			if n%10 < 7 {
				req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/key_%d", n%1000), nil)
				w := httptest.NewRecorder()
				vault.ServeHTTP(w, req)
			} else if n%10 < 9 {
				req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", n), bytes.NewReader(data))
				w := httptest.NewRecorder()
				vault.ServeHTTP(w, req)
			} else {
				req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/key_%d", n%1000), nil)
				w := httptest.NewRecorder()
				vault.ServeHTTP(w, req)
			}
		}
	})
}

func BenchmarkThroughput_Sequential_Writes(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

func BenchmarkThroughput_Sequential_Reads(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	for i := 0; i < 10000; i++ {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/key_%d", i), bytes.NewReader(data))
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/key_%d", i%10000), nil)
		w := httptest.NewRecorder()
		vault.ServeHTTP(w, req)
	}

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

func BenchmarkCache_Hit_Rate(b *testing.B) {
	vault := mustVault(b)
	defer vault.storage.Close()

	data := make([]byte, 1*KB)
	rand.Read(data)

	for i := 0; i < 100; i++ {
		vault.storage.Set(fmt.Sprintf("key_%d", i), data)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		vault.storage.Get(fmt.Sprintf("key_%d", i%100))
	}
}

func BenchmarkWAL_Batch_Writes(b *testing.B) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_%d", time.Now().UnixNano()))
	defer os.RemoveAll(dir)

	s := mustStorage(dir)
	defer s.Close()

	data := make([]byte, 256)
	rand.Read(data)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		s.Set(fmt.Sprintf("key_%d", i), data)
	}
}

func mustStorage(dir string) *Storage {
	os.MkdirAll(dir, 0755)
	s, err := NewStorage(dir)
	if err != nil {
		panic(err)
	}
	return s
}

func mustVault(b *testing.B) *Vault {
	b.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("bench_%d", time.Now().UnixNano()))
	b.Cleanup(func() { os.RemoveAll(dir) })

	storage := mustStorage(dir)

	vault := &Vault{
		storage: storage,
		cluster: NewCluster("http://localhost:3000", "", "", storage),
		limiter: NewRateLimiter(1000000),
	}

	return vault
}

type Storage struct {
	dir   string
	cache *cache
	wal   *wal
	size  atomic.Int64
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
		dir:   dir,
		cache: newCache(100000),
		wal:   w,
	}

	return s, nil
}

func (s *Storage) Set(key string, value []byte) error {
	h := hash64str(key)
	s.wal.append(h, value)
	s.cache.set(h, value)
	s.size.Add(int64(len(value)))
	return nil
}

func (s *Storage) Get(key string) ([]byte, error) {
	h := hash64str(key)
	if data, ok := s.cache.get(h); ok {
		return data, nil
	}
	return nil, fmt.Errorf("not found")
}

func (s *Storage) Delete(key string) error {
	h := hash64str(key)
	s.cache.del(h)
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

type Vault struct {
	storage *Storage
	cluster *Cluster
	limiter *RateLimiter
}

func (v *Vault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !v.limiter.Allow() {
		http.Error(w, "rate limit", http.StatusTooManyRequests)
		return
	}

	path := r.URL.Path[1:]
	if path == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		data, err := v.storage.Get(path)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Write(data)
	case http.MethodPut, http.MethodPost:
		data, _ := io.ReadAll(r.Body)
		v.storage.Set(path, data)
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		v.storage.Delete(path)
		w.WriteHeader(http.StatusOK)
	}
}

type Cluster struct {
	self    string
	master  string
	authKey string
	storage *Storage
}

func NewCluster(self, master, authKey string, storage *Storage) *Cluster {
	return &Cluster{
		self:    self,
		master:  master,
		authKey: authKey,
		storage: storage,
	}
}

type RateLimiter struct {
	tokens   atomic.Uint64
	rate     uint64
	capacity uint64
	last     atomic.Int64
}

func NewRateLimiter(rate int) *RateLimiter {
	return &RateLimiter{
		rate:     uint64(rate),
		capacity: uint64(rate),
	}
}

func (rl *RateLimiter) Allow() bool {
	now := time.Now().UnixNano()
	last := rl.last.Swap(now)
	elapsed := float64(now-last) / 1e9

	tokens := rl.tokens.Load()
	tokens += uint64(elapsed * float64(rl.rate))
	if tokens > rl.capacity {
		tokens = rl.capacity
	}

	if tokens >= 1 {
		rl.tokens.Store(tokens - 1)
		return true
	}

	rl.tokens.Store(tokens)
	return false
}

type cache struct {
	shards [256]*shard
	bloom  *bloom
	size   atomic.Int64
	items  atomic.Int64
}

type shard struct {
	mu sync.RWMutex
	m  map[uint64]*entry
}

type entry struct {
	data []byte
	hits uint32
}

func newCache(n int) *cache {
	c := &cache{bloom: newBloom(n)}
	for i := range c.shards {
		c.shards[i] = &shard{m: make(map[uint64]*entry)}
	}
	return c
}

func (c *cache) set(h uint64, data []byte) {
	s := c.shards[h%256]
	s.mu.Lock()
	c.items.Add(1)
	s.m[h] = &entry{data: data}
	c.bloom.add(h)
	s.mu.Unlock()
}

func (c *cache) get(h uint64) ([]byte, bool) {
	if !c.bloom.has(h) {
		return nil, false
	}
	s := c.shards[h%256]
	s.mu.RLock()
	e, ok := s.m[h]
	s.mu.RUnlock()
	if ok {
		return e.data, true
	}
	return nil, false
}

func (c *cache) del(h uint64) {
	s := c.shards[h%256]
	s.mu.Lock()
	delete(s.m, h)
	c.items.Add(-1)
	s.mu.Unlock()
}

func (c *cache) has(h uint64) bool {
	if !c.bloom.has(h) {
		return false
	}
	s := c.shards[h%256]
	s.mu.RLock()
	_, ok := s.m[h]
	s.mu.RUnlock()
	return ok
}

type bloom struct {
	bits []uint64
	k    uint32
}

func newBloom(n int) *bloom {
	size := n * 10 / 64
	if size < 1024 {
		size = 1024
	}
	return &bloom{bits: make([]uint64, size), k: 3}
}

func (b *bloom) add(h uint64) {
	h1, h2 := uint32(h), uint32(h>>32)
	for i := uint32(0); i < b.k; i++ {
		pos := (h1 + i*h2) % uint32(len(b.bits)*64)
		idx, bit := pos/64, pos%64
		atomic.AddUint64(&b.bits[idx], 1<<bit)
	}
}

func (b *bloom) has(h uint64) bool {
	h1, h2 := uint32(h), uint32(h>>32)
	for i := uint32(0); i < b.k; i++ {
		pos := (h1 + i*h2) % uint32(len(b.bits)*64)
		idx, bit := pos/64, pos%64
		if atomic.LoadUint64(&b.bits[idx])&(1<<bit) == 0 {
			return false
		}
	}
	return true
}

type wal struct {
	dir   string
	file  *os.File
	batch []walEntry
	ch    chan walEntry
	done  chan struct{}
}

type walEntry struct {
	hash uint64
	data []byte
}

func newWAL(dir string) (*wal, error) {
	path := filepath.Join(dir, "wal.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	w := &wal{
		dir:   dir,
		file:  f,
		batch: make([]walEntry, 0, 1000),
		ch:    make(chan walEntry, 2000),
		done:  make(chan struct{}),
	}

	go w.flusher()
	return w, nil
}

func (w *wal) append(h uint64, data []byte) {
	w.ch <- walEntry{hash: h, data: data}
}

func (w *wal) flusher() {
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case e := <-w.ch:
			w.batch = append(w.batch, e)
			if len(w.batch) >= 1000 {
				w.file.Sync()
				w.batch = w.batch[:0]
			}
		case <-ticker.C:
			if len(w.batch) > 0 {
				w.file.Sync()
				w.batch = w.batch[:0]
			}
		case <-w.done:
			w.file.Sync()
			w.file.Close()
			return
		}
	}
}

func (w *wal) close() {
	close(w.done)
}

func hash64str(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
