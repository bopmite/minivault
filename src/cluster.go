package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"runtime"
	"sort"
	"sync"
	"time"
)

type Cluster struct {
	self    string
	master  string
	nodes   sync.Map
	client  *http.Client
	workers chan struct{} // Goroutine pool
	authKey string
	storage *Storage
}

type NodeInfo struct {
	URL      string
	LastSeen time.Time
	Load     float64
}

type SyncMessage struct {
	Key   string `json:"key"`
	Value []byte `json:"value"`
	Hash  string `json:"hash"`
}

type RegisterRequest struct {
	URL  string  `json:"url"`
	Load float64 `json:"load"`
}

type NodesResponse struct {
	Nodes []string `json:"nodes"`
}

func NewCluster(self, master, authKey string, storage *Storage) *Cluster {
	c := &Cluster{
		self:    self,
		master:  master,
		authKey: authKey,
		storage: storage,
		workers: make(chan struct{}, WorkerPoolSize),
		client: &http.Client{
			Timeout: WriteTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        200,
				MaxIdleConnsPerHost: 50,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}

	for i := 0; i < WorkerPoolSize; i++ {
		c.workers <- struct{}{}
	}

	c.nodes.Store(self, &NodeInfo{URL: self, LastSeen: time.Now()})
	return c
}

func (c *Cluster) registerWithMaster() {
	if c.master == "" {
		return
	}

	load := float64(runtime.NumGoroutine())
	data, _ := json.Marshal(RegisterRequest{URL: c.self, Load: load})

	req, _ := http.NewRequest(http.MethodPost, c.master+"/register", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	c.signRequest(req)

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("Failed to register: %v", err)
		return
	}
	resp.Body.Close()
}

func (c *Cluster) heartbeatLoop() {
	ticker := time.NewTicker(HeartbeatInterval)
	for range ticker.C {
		c.sendHeartbeat()
	}
}

func (c *Cluster) sendHeartbeat() {
	if c.master == "" {
		return
	}

	load := float64(runtime.NumGoroutine())
	data, _ := json.Marshal(RegisterRequest{URL: c.self, Load: load})

	req, _ := http.NewRequest(http.MethodPost, c.master+"/heartbeat", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	c.signRequest(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (c *Cluster) syncNodesLoop() {
	ticker := time.NewTicker(HeartbeatInterval)
	for range ticker.C {
		req, _ := http.NewRequest(http.MethodGet, c.master+"/nodes", nil)
		c.signRequest(req)

		resp, err := c.client.Do(req)
		if err != nil {
			continue
		}

		var nodesResp NodesResponse
		json.NewDecoder(resp.Body).Decode(&nodesResp)
		resp.Body.Close()

		for _, url := range nodesResp.Nodes {
			if _, exists := c.nodes.Load(url); !exists {
				c.nodes.Store(url, &NodeInfo{URL: url, LastSeen: time.Now()})
			}
		}
	}
}

func (c *Cluster) getNodes() []string {
	var nodes []string
	c.nodes.Range(func(key, _ interface{}) bool {
		nodes = append(nodes, key.(string))
		return true
	})
	return nodes
}

func (c *Cluster) rendezvousHash(key string, count int) []string {
	nodes := c.getNodes()
	if len(nodes) == 0 {
		return nil
	}

	type scored struct {
		node string
		hash uint32
	}

	scores := make([]scored, len(nodes))
	for i, node := range nodes {
		h := crc32.NewIEEE()
		h.Write([]byte(key + node))
		scores[i] = scored{node: node, hash: h.Sum32()}
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].hash > scores[j].hash
	})

	if count > len(scores) {
		count = len(scores)
	}

	result := make([]string, count)
	for i := 0; i < count; i++ {
		result[i] = scores[i].node
	}

	return result
}

func (c *Cluster) QuorumWrite(key string, value []byte) error {
	nodes := c.rendezvousHash(key, ReplicationFactor)
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes available")
	}

	quorum := (len(nodes) / 2) + 1
	results := make(chan error, len(nodes))
	timeout := time.After(WriteTimeout)

	for _, node := range nodes {
		<-c.workers
		go func(n string) {
			defer func() { c.workers <- struct{}{} }()

			var err error
			if n == c.self {
				err = c.storage.Set(key, value)
			} else {
				err = c.syncWrite(n, key, value)
			}
			results <- err
		}(node)
	}

	successes := 0
	for i := 0; i < len(nodes); i++ {
		select {
		case err := <-results:
			if err == nil {
				successes++
				if successes >= quorum {
					return nil
				}
			}
		case <-timeout:
			return fmt.Errorf("quorum timeout")
		}
	}

	return fmt.Errorf("quorum not reached: %d/%d", successes, quorum)
}

func (c *Cluster) QuorumRead(key string) ([]byte, error) {
	nodes := c.rendezvousHash(key, ReplicationFactor)

	for _, node := range nodes {
		var value []byte
		var err error

		if node == c.self {
			value, err = c.storage.Get(key)
		} else {
			value, err = c.fetchFrom(node, key)
		}

		if err == nil {
			if node != c.self {
				_ = c.storage.Set(key, value)
			}
			return value, nil
		}
	}

	return nil, fmt.Errorf("not found")
}

func (c *Cluster) syncWrite(node, key string, value []byte) error {
	hash := computeHash(value)
	msg := SyncMessage{Key: key, Value: value, Hash: hash}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, node+"/_sync", bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	c.signRequest(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sync failed: %d", resp.StatusCode)
	}

	return nil
}

func (c *Cluster) fetchFrom(node, key string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, node+"/"+key, nil)
	if err != nil {
		return nil, err
	}

	c.signRequest(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch failed: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (c *Cluster) signRequest(req *http.Request) {
	if c.authKey == "" {
		return
	}

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	req.Header.Set("X-Timestamp", timestamp)

	h := hmac.New(sha256.New, []byte(c.authKey))
	h.Write([]byte(req.Method + req.URL.Path + timestamp))
	sig := hex.EncodeToString(h.Sum(nil))

	req.Header.Set("X-Signature", sig)
}

func (c *Cluster) verifyRequest(req *http.Request) bool {
	if c.authKey == "" {
		return true
	}

	timestamp := req.Header.Get("X-Timestamp")
	signature := req.Header.Get("X-Signature")

	if timestamp == "" || signature == "" {
		return false
	}

	h := hmac.New(sha256.New, []byte(c.authKey))
	h.Write([]byte(req.Method + req.URL.Path + timestamp))
	expected := hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}

func computeHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
