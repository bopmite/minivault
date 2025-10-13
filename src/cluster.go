package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
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
	workers chan struct{}
	authKey string
	storage *Storage
}

type node struct {
	url  string
	seen time.Time
	load float64
}

func NewCluster(self, master, authKey string, storage *Storage) *Cluster {
	c := &Cluster{
		self:    self,
		master:  master,
		authKey: authKey,
		storage: storage,
		workers: make(chan struct{}, WorkerPool),
		client: &http.Client{
			Timeout: WriteTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	for range WorkerPool {
		c.workers <- struct{}{}
	}

	c.nodes.Store(self, &node{url: self, seen: time.Now()})
	return c
}

func (c *Cluster) start() {
	c.register()

	go func() {
		ticker := time.NewTicker(Heartbeat)
		defer ticker.Stop()
		for range ticker.C {
			c.heartbeat()
		}
	}()

	go func() {
		ticker := time.NewTicker(Heartbeat)
		defer ticker.Stop()
		for range ticker.C {
			c.syncNodes()
		}
	}()
}

type regReq struct {
	URL  string  `json:"url"`
	Load float64 `json:"load"`
}

func (c *Cluster) register() {
	if c.master == "" {
		return
	}

	load := float64(runtime.NumGoroutine())
	data, _ := json.Marshal(regReq{URL: c.self, Load: load})

	req, _ := http.NewRequest(http.MethodPost, c.master+"/register", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	c.sign(req)

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("register failed: %v", err)
		return
	}
	resp.Body.Close()
}

func (c *Cluster) heartbeat() {
	if c.master == "" {
		return
	}

	load := float64(runtime.NumGoroutine())
	data, _ := json.Marshal(regReq{URL: c.self, Load: load})

	req, _ := http.NewRequest(http.MethodPost, c.master+"/heartbeat", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	c.sign(req)

	resp, err := c.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

type nodesResp struct {
	Nodes []string `json:"nodes"`
}

func (c *Cluster) syncNodes() {
	req, _ := http.NewRequest(http.MethodGet, c.master+"/nodes", nil)
	c.sign(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var nr nodesResp
	json.NewDecoder(resp.Body).Decode(&nr)

	for _, url := range nr.Nodes {
		if _, ok := c.nodes.Load(url); !ok {
			c.nodes.Store(url, &node{url: url, seen: time.Now()})
		}
	}
}

func (c *Cluster) getNodes() []string {
	var nodes []string
	c.nodes.Range(func(key, _ any) bool {
		nodes = append(nodes, key.(string))
		return true
	})
	return nodes
}

func (c *Cluster) count() int {
	n := 0
	c.nodes.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func (c *Cluster) hash(key string, count int) []string {
	nodes := c.getNodes()
	if len(nodes) == 0 {
		return nil
	}

	type score struct {
		node string
		hash uint32
	}

	scores := make([]score, len(nodes))
	for i, n := range nodes {
		h := crc32.NewIEEE()
		h.Write([]byte(key + n))
		scores[i] = score{node: n, hash: h.Sum32()}
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

func (c *Cluster) write(key string, data []byte) error {
	nodes := c.hash(key, ReplicaCount)
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes")
	}

	quorum := (len(nodes) / 2) + 1
	results := make(chan error, len(nodes))
	timeout := time.After(WriteTimeout)

	for _, n := range nodes {
		<-c.workers
		go func(node string) {
			defer func() { c.workers <- struct{}{} }()

			var err error
			if node == c.self {
				err = c.storage.Set(key, data)
			} else {
				err = c.sync(node, key, data)
			}
			results <- err
		}(n)
	}

	ok := 0
	for range nodes {
		select {
		case err := <-results:
			if err == nil {
				ok++
				if ok >= quorum {
					return nil
				}
			}
		case <-timeout:
			return fmt.Errorf("timeout")
		}
	}

	return fmt.Errorf("quorum failed: %d/%d", ok, quorum)
}

func (c *Cluster) read(key string) ([]byte, error) {
	nodes := c.hash(key, ReplicaCount)

	for _, n := range nodes {
		var data []byte
		var err error

		if n == c.self {
			data, err = c.storage.Get(key)
		} else {
			data, err = c.fetch(n, key)
		}

		if err == nil {
			if n != c.self {
				_ = c.storage.Set(key, data)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("not found")
}

func (c *Cluster) sync(node, key string, data []byte) error {
	msg := syncMsg{
		Key:  key,
		Data: data,
		Hash: hash64(data),
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, node+"/_sync", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	c.sign(req)

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

func (c *Cluster) fetch(node, key string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, node+"/"+key, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Cluster-Fetch", "true")
	c.sign(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch failed: %d", resp.StatusCode)
	}

	var apiResp struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("fetch failed: %s", apiResp.Error)
	}

	return apiResp.Data, nil
}

func (c *Cluster) sign(req *http.Request) {
	if c.authKey == "" {
		return
	}

	ts := fmt.Sprintf("%d", time.Now().Unix())
	req.Header.Set("X-Timestamp", ts)

	h := hmac.New(sha256.New, []byte(c.authKey))
	h.Write([]byte(req.Method + req.URL.Path + ts))
	sig := hex.EncodeToString(h.Sum(nil))

	req.Header.Set("X-Signature", sig)
}

func (c *Cluster) verify(req *http.Request) bool {
	if c.authKey == "" {
		return true
	}

	ts := req.Header.Get("X-Timestamp")
	sig := req.Header.Get("X-Signature")

	if ts == "" || sig == "" {
		return false
	}

	h := hmac.New(sha256.New, []byte(c.authKey))
	h.Write([]byte(req.Method + req.URL.Path + ts))
	expected := hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}
