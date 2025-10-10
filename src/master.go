package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type MasterServer struct {
	mu    sync.RWMutex
	nodes map[string]*WorkerNode
}

type WorkerNode struct {
	URL      string
	LastSeen time.Time
}

type RegisterRequest struct {
	URL string `json:"url"`
}

type NodesResponse struct {
	Nodes []string `json:"nodes"`
}

func (m *MasterServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.nodes[req.URL] = &WorkerNode{
		URL:      req.URL,
		LastSeen: time.Now(),
	}
	m.mu.Unlock()

	log.Printf("Worker registered: %s", req.URL)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (m *MasterServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	if node, exists := m.nodes[req.URL]; exists {
		node.LastSeen = time.Now()
	}
	m.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (m *MasterServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var nodes []string
	for url, node := range m.nodes {
		if time.Since(node.LastSeen) < 30*time.Second {
			nodes = append(nodes, url)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(NodesResponse{Nodes: nodes})
}

func (m *MasterServer) pruneDeadNodes() {
	ticker := time.NewTicker(10 * time.Second)

	for range ticker.C {
		m.mu.Lock()

		for url, node := range m.nodes {
			if time.Since(node.LastSeen) > 30*time.Second {
				delete(m.nodes, url)
				log.Printf("Worker removed (timeout): %s", url)
			}
		}

		m.mu.Unlock()
	}
}

func runMaster(port int, volumes string) {
	master := &MasterServer{nodes: make(map[string]*WorkerNode)}

	if volumes != "" {
		for _, vol := range splitVolumes(volumes) {
			master.nodes[vol] = &WorkerNode{
				URL:      vol,
				LastSeen: time.Now(),
			}
			log.Printf("Static volume configured: %s", vol)
		}
	}

	http.HandleFunc("/register", master.handleRegister)
	http.HandleFunc("/heartbeat", master.handleHeartbeat)
	http.HandleFunc("/nodes", master.handleNodes)

	go master.pruneDeadNodes()

	log.Printf("Master server starting on :%d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func splitVolumes(volumes string) []string {
	var result []string

	for _, v := range split(volumes, ',') {
		if v != "" {
			if !hasPrefix(v, "http://") && !hasPrefix(v, "https://") {
				v = "http://" + v
			}

			result = append(result, v)
		}
	}

	return result
}

func split(s string, sep rune) []string {
	var result []string
	start := 0

	for i, c := range s {
		if c == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}

	result = append(result, s[start:])

	return result
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
