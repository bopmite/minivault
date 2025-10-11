package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type ValueRequest struct {
	Value json.RawMessage `json:"value"`
}

type HealthResponse struct {
	Status     string `json:"status"`
	Nodes      int    `json:"nodes"`
	Keys       int64  `json:"keys"`
	Size       int64  `json:"size"`
	Memory     uint64 `json:"memory_mb"`
	Goroutines int    `json:"goroutines"`
}

type RateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	rate     float64
	capacity float64
	last     time.Time
}

func NewRateLimiter(rate float64) *RateLimiter {
	return &RateLimiter{
		tokens:   rate,
		rate:     rate,
		capacity: rate,
		last:     time.Now(),
	}
}

func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.last).Seconds()
	rl.last = now

	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.capacity {
		rl.tokens = rl.capacity
	}

	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}

	return false
}

func (v *Vault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !v.limiter.Allow() {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	if v.cluster.authKey != "" && !v.cluster.verifyRequest(r) {
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestSize)
	path := strings.TrimPrefix(r.URL.Path, "/")

	if path == "_sync" {
		v.handleSync(w, r)
		return
	}

	if path == "health" {
		v.handleHealth(w, r)
		return
	}

	if path == "" {
		writeError(w, http.StatusBadRequest, "key required")
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
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (v *Vault) handleGet(w http.ResponseWriter, r *http.Request, key string) {
	value, err := v.storage.Get(key)
	if err == nil {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write(value)
		return
	}

	value, err = v.cluster.QuorumRead(key)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(value)
}

func (v *Vault) handlePost(w http.ResponseWriter, r *http.Request, key string) {
	if v.storage.Exists(key) {
		writeError(w, http.StatusConflict, "key exists")
		return
	}

	value, err := extractValue(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := v.cluster.QuorumWrite(key, value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, http.StatusCreated, nil)
}

func (v *Vault) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	value, err := extractValue(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := v.cluster.QuorumWrite(key, value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, http.StatusOK, nil)
}

func (v *Vault) handleDelete(w http.ResponseWriter, r *http.Request, key string) {
	v.storage.Delete(key)

	nodes := v.cluster.rendezvousHash(key, ReplicationFactor)
	for _, node := range nodes {
		if node != v.cluster.self {
			go v.sendDelete(node, key)
		}
	}

	writeSuccess(w, http.StatusOK, nil)
}

func (v *Vault) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	var msg SyncMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if computeHash(msg.Value) != msg.Hash {
		writeError(w, http.StatusBadRequest, "hash mismatch")
		return
	}

	if err := v.storage.Set(msg.Key, msg.Value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, http.StatusOK, nil)
}

func (v *Vault) handleHealth(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	nodes := 0
	v.cluster.nodes.Range(func(_, _ interface{}) bool {
		nodes++
		return true
	})

	health := HealthResponse{
		Status:     "healthy",
		Nodes:      nodes,
		Keys:       v.storage.Count(),
		Size:       v.storage.size.Load(),
		Memory:     m.Alloc / 1024 / 1024,
		Goroutines: runtime.NumGoroutine(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (v *Vault) sendDelete(node, key string) {
	req, _ := http.NewRequest(http.MethodDelete, node+"/"+key, nil)
	v.cluster.signRequest(req)

	resp, err := v.cluster.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func extractValue(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("empty body")
	}

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req ValueRequest
		if err := json.Unmarshal(body, &req); err == nil && len(req.Value) > 0 {
			return []byte(req.Value), nil
		}
	}

	return body, nil
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{Success: false, Error: msg})
}

func writeSuccess(w http.ResponseWriter, status int, data json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{Success: true, Data: data})
}
