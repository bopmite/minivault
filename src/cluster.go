package main

import (
	"fmt"
	"hash/crc32"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type Cluster struct {
	self           string
	nodes          sync.Map
	client         *BinaryClient
	workers        chan struct{}
	authKey        string
	storage        *Storage
	workerPoolSize int
}

type node struct {
	url  string
	seen time.Time
}

func NewCluster(self, authKey string, storage *Storage, workerPoolSize int) *Cluster {
	c := &Cluster{
		self:           self,
		authKey:        authKey,
		storage:        storage,
		workerPoolSize: workerPoolSize,
		workers:        make(chan struct{}, workerPoolSize),
		client:         NewBinaryClient(),
	}

	for range workerPoolSize {
		c.workers <- struct{}{}
	}

	c.nodes.Store(self, &node{url: self, seen: time.Now()})

	nodes := os.Getenv("CLUSTER_NODES")
	if nodes != "" {
		for _, n := range strings.Split(nodes, ",") {
			n = strings.TrimSpace(n)
			if n != "" && n != self {
				c.nodes.Store(n, &node{url: n, seen: time.Now()})
			}
		}
	}

	return c
}

func (c *Cluster) getNodes() []string {
	var nodes []string
	c.nodes.Range(func(key, _ any) bool {
		nodes = append(nodes, key.(string))
		return true
	})
	return nodes
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

	acquired := 0
	for _, n := range nodes {
		select {
		case <-c.workers:
			acquired++
			go func(node string) {
				defer func() { c.workers <- struct{}{} }()
				var err error
				if node == c.self {
					err = c.storage.Set(key, data)
				} else {
					err = c.client.Sync(node, key, data)
				}
				results <- err
			}(n)
		case <-time.After(50 * time.Millisecond):
			results <- fmt.Errorf("worker timeout")
		}
	}

	ok := 0
	for i := 0; i < acquired; i++ {
		select {
		case err := <-results:
			if err == nil {
				ok++
				if ok >= quorum {
					go func() {
						for j := i + 1; j < acquired; j++ {
							<-results
						}
					}()
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
			data, err = c.client.Get(n, key)
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

func (c *Cluster) sendDelete(node, key string) {
	c.client.Delete(node, key)
}
