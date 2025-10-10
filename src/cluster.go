package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/OneOfOne/xxhash"
)

type Cluster struct {
	selfURL   string
	masterURL string
	nodes     sync.Map
	client    *http.Client
}

type Node struct {
	URL       string
	LastSeen  time.Time
	Heartbeat uint64
}

type SyncMsg struct {
	Type  string `json:"t"`
	Key   string `json:"k"`
	Value []byte `json:"v,omitempty"`
}

type MasterNodesResponse struct {
	Nodes []string `json:"nodes"`
}

var cluster *Cluster

func InitCluster(selfURL string, masterURL string) *Cluster {
	cluster = &Cluster{
		selfURL:   selfURL,
		masterURL: masterURL,
		client: &http.Client{
			Timeout: 100 * time.Millisecond,
			Transport: &http.Transport{
				MaxIdleConns:        1000,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	cluster.nodes.Store(selfURL, &Node{
		URL:       selfURL,
		LastSeen:  time.Now(),
		Heartbeat: 1,
	})

	if masterURL != "" {
		go cluster.registerWithMaster()
		go cluster.heartbeatWorker()
		go cluster.syncNodesFromMaster()
	}

	return cluster
}

func (c *Cluster) registerWithMaster() {
	data, _ := json.Marshal(map[string]string{"url": c.selfURL})
	req, _ := http.NewRequest(http.MethodPost, c.masterURL+"/register", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func (c *Cluster) heartbeatWorker() {
	ticker := time.NewTicker(5 * time.Second)

	for range ticker.C {
		data, _ := json.Marshal(map[string]string{"url": c.selfURL})
		req, _ := http.NewRequest(http.MethodPost, c.masterURL+"/heartbeat", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}
}

func (c *Cluster) syncNodesFromMaster() {
	ticker := time.NewTicker(5 * time.Second)

	for range ticker.C {
		req, _ := http.NewRequest(http.MethodGet, c.masterURL+"/nodes", nil)
		resp, err := c.client.Do(req)

		if err != nil {
			continue
		}

		var nodesResp MasterNodesResponse
		if json.NewDecoder(resp.Body).Decode(&nodesResp) == nil {
			for _, url := range nodesResp.Nodes {
				if _, exists := c.nodes.Load(url); !exists {
					c.nodes.Store(url, &Node{
						URL:       url,
						LastSeen:  time.Now(),
						Heartbeat: 0,
					})
				}
			}
		}

		resp.Body.Close()
	}
}

func (c *Cluster) getNodes() []string {
	var nodes []string

	c.nodes.Range(func(key, val interface{}) bool {
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

	type score struct {
		node string
		hash uint64
	}

	scores := make([]score, len(nodes))
	for i, node := range nodes {
		h := xxhash.New64()
		h.WriteString(key)
		h.WriteString(node)
		scores[i] = score{node: node, hash: h.Sum64()}
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

func (c *Cluster) QuorumWrite(key string, val []byte) error {
	nodes := c.rendezvousHash(key, 3)
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes available")
	}

	results := make(chan error, len(nodes))
	for _, node := range nodes {
		go func(n string) {
			if n == c.selfURL {
				results <- Set(key, val)
			} else {
				results <- c.sendWrite(n, key, val)
			}
		}(node)
	}

	successes := 0
	quorum := (len(nodes) / 2) + 1

	for i := 0; i < len(nodes); i++ {
		if err := <-results; err == nil {
			successes++
			if successes >= quorum {
				return nil
			}
		}
	}

	return fmt.Errorf("quorum not reached")
}

func (c *Cluster) QuorumRead(key string) []byte {
	nodes := c.rendezvousHash(key, 3)

	for _, node := range nodes {
		var val []byte

		if node == c.selfURL {
			val = Get(key)
		} else {
			val = c.fetchFrom(node, key)
		}

		if val != nil {
			if node != c.selfURL {
				Set(key, val)
			}
			return val
		}
	}

	return nil
}

func (c *Cluster) sendWrite(node, key string, val []byte) error {
	msg := SyncMsg{Type: "set", Key: key, Value: val}
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(msg)

	req, err := http.NewRequest(http.MethodPut, node+"/_sync/"+key, buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

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

func (c *Cluster) fetchFrom(node, key string) []byte {
	req, err := http.NewRequest(http.MethodGet, node+"/"+key, nil)
	if err != nil {
		return nil
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	data, _ := io.ReadAll(resp.Body)
	return data
}
