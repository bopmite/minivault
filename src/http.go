package main

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

type HTTPServer struct {
	vault      *Vault
	startTime  time.Time
	authKey    string
	authMode   AuthMode
	limiter    *rate.Limiter
}

func NewHTTPServer(vault *Vault, authKey string, authMode AuthMode, rateLimit int, startTime time.Time) *HTTPServer {
	var limiter *rate.Limiter
	if rateLimit > 0 {
		limiter = rate.NewLimiter(rate.Limit(rateLimit), rateLimit/10)
	}
	return &HTTPServer{
		vault:     vault,
		startTime: startTime,
		authKey:   authKey,
		authMode:  authMode,
		limiter:   limiter,
	}
}

func (s *HTTPServer) checkAuth(r *http.Request, needsAuth bool) bool {
	if !needsAuth {
		return true
	}
	if s.authKey == "" {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+s.authKey
}

func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.limiter != nil && !s.limiter.Allow() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "rate limit"})
		return
	}

	if r.URL.Path == "/health" {
		s.handleHealth(w, r)
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/")
	if key == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "key required"})
		return
	}

	needsAuth := false
	if s.authMode == AuthAll {
		needsAuth = true
	} else if s.authMode == AuthWrites && (r.Method == http.MethodPut || r.Method == http.MethodPost || r.Method == http.MethodDelete) {
		needsAuth = true
	}

	if !s.checkAuth(r, needsAuth) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "unauthorized"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		data, err := s.vault.storage.Get(key)
		if err != nil {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "not found"})
			return
		}

		var value interface{}
		if err := json.Unmarshal(data, &value); err != nil {
			value = string(data)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": value})

	case http.MethodPut, http.MethodPost:
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "invalid json"})
			return
		}

		value, ok := req["value"]
		if !ok {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "missing value field"})
			return
		}

		data, err := json.Marshal(value)
		if err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "failed to marshal value"})
			return
		}

		if err := s.vault.cluster.write(key, data); err != nil {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "write error"})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	case http.MethodDelete:
		if err := s.vault.cluster.delete(key); err != nil {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "delete error"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	default:
		w.WriteHeader(405)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "method not allowed"})
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
