package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type apiResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
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

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(apiResponse{Success: false, Error: msg})
}

func jsonSuccess(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiResponse{Success: true, Data: data})
}

func (v *Vault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !v.limiter.Allow() {
		jsonError(w, "rate limit", http.StatusTooManyRequests)
		return
	}

	if v.cluster.authKey != "" && !v.cluster.verify(r) {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestSize)
	path := strings.TrimPrefix(r.URL.Path, "/")

	switch path {
	case "_sync":
		v.handleSync(w, r)
		return
	case "health":
		v.handleHealth(w, r)
		return
	case "":
		jsonError(w, "key required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		v.handleGet(w, r, path)
	case http.MethodPost:
		v.handlePost(w, r, path)
	case http.MethodPut:
		v.handlePut(w, r, path)
	case http.MethodDelete:
		v.handleDelete(w, r, path)
	default:
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (v *Vault) handleGet(w http.ResponseWriter, r *http.Request, key string) {
	data, err := v.storage.Get(key)
	if err == nil {
		jsonSuccess(w, data)
		return
	}

	if r.Header.Get("X-Cluster-Fetch") == "" {
		data, err = v.cluster.read(key)
		if err == nil {
			jsonSuccess(w, data)
			return
		}
	}

	jsonError(w, "not found", http.StatusNotFound)
}

func (v *Vault) handlePost(w http.ResponseWriter, r *http.Request, key string) {
	if v.storage.Exists(key) {
		jsonError(w, "exists", http.StatusConflict)
		return
	}

	data, err := readBody(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer putbuf(data)

	if err := v.cluster.write(key, data); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(apiResponse{Success: true})
}

func (v *Vault) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	data, err := readBody(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer putbuf(data)

	if err := v.cluster.write(key, data); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiResponse{Success: true})
}

func (v *Vault) handleDelete(w http.ResponseWriter, _ *http.Request, key string) {
	v.storage.Delete(key)

	nodes := v.cluster.hash(key, ReplicaCount)
	for _, node := range nodes {
		if node != v.cluster.self {
			<-v.cluster.workers
			go func(n string) {
				defer func() { v.cluster.workers <- struct{}{} }()
				v.sendDelete(n, key)
			}(node)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiResponse{Success: true})
}

type syncMsg struct {
	Key  string `json:"key"`
	Data []byte `json:"data"`
	Hash uint64 `json:"hash"`
}

func (v *Vault) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		jsonError(w, "bad request", http.StatusBadRequest)
		return
	}

	var msg syncMsg
	if err := json.Unmarshal(body, &msg); err != nil {
		jsonError(w, "bad json", http.StatusBadRequest)
		return
	}

	if hash64(msg.Data) != msg.Hash {
		jsonError(w, "hash mismatch", http.StatusBadRequest)
		return
	}

	if err := v.storage.Set(msg.Key, msg.Data); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiResponse{Success: true})
}

type healthResp struct {
	Status string `json:"status"`
	Nodes  int    `json:"nodes"`
	Keys   int64  `json:"keys"`
	Size   int64  `json:"size"`
	Memory uint64 `json:"memory_mb"`
	GoRs   int    `json:"goroutines"`
}

func (v *Vault) handleHealth(w http.ResponseWriter, _ *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	resp := healthResp{
		Status: "ok",
		Nodes:  v.cluster.count(),
		Keys:   v.storage.Count(),
		Size:   v.storage.size.Load(),
		Memory: m.Alloc / 1024 / 1024,
		GoRs:   runtime.NumGoroutine(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (v *Vault) sendDelete(node, key string) {
	req, _ := http.NewRequest(http.MethodDelete, node+"/"+key, nil)
	v.cluster.sign(req)

	resp, err := v.cluster.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

var bodyPool = sync.Pool{New: func() any { return make([]byte, 0, 16384) }}

func readBody(r *http.Request) ([]byte, error) {
	buf := bodyPool.Get().([]byte)[:0]

	data, err := io.ReadAll(r.Body)
	if err != nil {
		bodyPool.Put(buf)
		return nil, err
	}

	if len(data) == 0 {
		bodyPool.Put(buf)
		return nil, fmt.Errorf("empty")
	}

	if ct := r.Header.Get("Content-Type"); strings.Contains(ct, "application/json") {
		var req struct{ Value json.RawMessage }
		if err := json.Unmarshal(data, &req); err == nil && len(req.Value) > 0 {
			return []byte(req.Value), nil
		}
	}

	return data, nil
}
