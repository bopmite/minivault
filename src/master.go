package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Master struct {
	mu    sync.RWMutex
	nodes map[string]*node
}

func runMaster(port int, volumes string) {
	m := &Master{nodes: make(map[string]*node)}

	if volumes != "" {
		for v := range strings.SplitSeq(volumes, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				if !strings.HasPrefix(v, "http") {
					v = "http://" + v
				}
				m.nodes[v] = &node{url: v, seen: time.Now()}
				log.Printf("volume: %s", v)
			}
		}
	}

	http.HandleFunc("/register", m.handleRegister)
	http.HandleFunc("/heartbeat", m.handleHeartbeat)
	http.HandleFunc("/nodes", m.handleNodes)
	http.HandleFunc("/health", m.handleHealth)

	go m.pruner()

	log.Printf("master on :%d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func (m *Master) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req regReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.nodes[req.URL] = &node{
		url:  req.URL,
		seen: time.Now(),
		load: req.Load,
	}
	m.mu.Unlock()

	log.Printf("registered: %s", req.URL)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (m *Master) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req regReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	if n, ok := m.nodes[req.URL]; ok {
		n.seen = time.Now()
		n.load = req.Load
	}
	m.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (m *Master) handleNodes(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	var nodes []string
	for url, n := range m.nodes {
		if time.Since(n.seen) < NodeTimeout {
			nodes = append(nodes, url)
		}
	}
	m.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodesResp{Nodes: nodes})
}

func (m *Master) handleHealth(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	active := 0
	for _, n := range m.nodes {
		if time.Since(n.seen) < NodeTimeout {
			active++
		}
	}
	m.mu.RUnlock()

	h := map[string]any{
		"status": "ok",
		"nodes":  active,
		"role":   "master",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h)
}

func (m *Master) pruner() {
	ticker := time.NewTicker(Heartbeat * 2)
	for range ticker.C {
		m.mu.Lock()
		for url, n := range m.nodes {
			if time.Since(n.seen) > NodeTimeout {
				delete(m.nodes, url)
				log.Printf("removed: %s", url)
			}
		}
		m.mu.Unlock()
	}
}
