package main

import (
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type HTTPServer struct {
	vault     *Vault
	startTime time.Time
}

func NewHTTPServer(vault *Vault, startTime time.Time) *HTTPServer {
	return &HTTPServer{vault: vault, startTime: startTime}
}

func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		s.handleHealth(w, r)
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/")
	if key == "" {
		http.Error(w, "key required", 400)
		return
	}

	switch r.Method {
	case http.MethodGet:
		data, err := s.vault.storage.Get(key)
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)

	case http.MethodPut, http.MethodPost:
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", 400)
			return
		}
		if err := s.vault.cluster.write(key, data); err != nil {
			http.Error(w, "write error", 500)
			return
		}
		w.WriteHeader(204)

	case http.MethodDelete:
		s.vault.storage.Delete(key)
		w.WriteHeader(204)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	health := map[string]interface{}{
		"status":          "healthy",
		"uptime_seconds":  int64(time.Since(s.startTime).Seconds()),
		"cache_items":     s.vault.storage.cache.items.Load(),
		"cache_size_mb":   s.vault.storage.cache.size.Load() / (1024 * 1024),
		"storage_size_mb": s.vault.storage.size.Load() / (1024 * 1024),
		"goroutines":      runtime.NumGoroutine(),
		"memory_mb":       m.Alloc / (1024 * 1024),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}
