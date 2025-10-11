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

type MasterService struct {
	mu    sync.RWMutex
	nodes map[string]*NodeInfo
}

func runMaster(port int, volumes string) {
	master := &MasterService{nodes: make(map[string]*NodeInfo)}

	if volumes != "" {
		for _, v := range strings.Split(volumes, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				if !strings.HasPrefix(v, "http") {
					v = "http://" + v
				}
				master.nodes[v] = &NodeInfo{URL: v, LastSeen: time.Now()}
				log.Printf("Static volume: %s", v)
			}
		}
	}

	http.HandleFunc("/register", master.handleRegister)
	http.HandleFunc("/heartbeat", master.handleHeartbeat)
	http.HandleFunc("/nodes", master.handleNodes)
	http.HandleFunc("/health", master.handleHealth)

	go master.pruneDeadNodes()

	log.Printf("Master server starting on :%d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func (m *MasterService) handleRegister(w http.ResponseWriter, r *http.Request) {
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
	m.nodes[req.URL] = &NodeInfo{
		URL:      req.URL,
		LastSeen: time.Now(),
		Load:     req.Load,
	}
	m.mu.Unlock()

	log.Printf("Worker registered: %s", req.URL)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (m *MasterService) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
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
		node.Load = req.Load
	}
	m.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (m *MasterService) handleNodes(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	var nodes []string
	for url, node := range m.nodes {
		if time.Since(node.LastSeen) < NodeTimeout {
			nodes = append(nodes, url)
		}
	}
	m.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(NodesResponse{Nodes: nodes})
}

func (m *MasterService) handleHealth(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	activeNodes := 0
	for _, node := range m.nodes {
		if time.Since(node.LastSeen) < NodeTimeout {
			activeNodes++
		}
	}
	m.mu.RUnlock()

	health := map[string]interface{}{
		"status": "healthy",
		"nodes":  activeNodes,
		"role":   "master",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (m *MasterService) pruneDeadNodes() {
	ticker := time.NewTicker(HeartbeatInterval * 2)
	for range ticker.C {
		m.mu.Lock()
		for url, node := range m.nodes {
			if time.Since(node.LastSeen) > NodeTimeout {
				delete(m.nodes, url)
				log.Printf("Worker removed (timeout): %s", url)
			}
		}
		m.mu.Unlock()
	}
}
