package main

import (
	"sync"
	"time"
)

type EdgeCache struct {
	local   *cache
	origin  *Cluster
	mu      sync.RWMutex
	origins []string
}

func NewEdgeCache(origins []string, originCluster *Cluster) *EdgeCache {
	return &EdgeCache{
		local:   newCache(1000000),
		origin:  originCluster,
		origins: origins,
	}
}

func (e *EdgeCache) Get(key string) ([]byte, error) {
	h := hash64str(key)

	if data, ok := e.local.get(h); ok {
		return data, nil
	}

	data, err := e.origin.read(key)
	if err != nil {
		return nil, err
	}

	e.local.set(h, data)
	return data, nil
}

func (e *EdgeCache) Set(key string, value []byte) error {
	h := hash64str(key)
	e.local.set(h, value)

	go func() {
		e.origin.write(key, value)
	}()

	return nil
}

func (e *EdgeCache) Delete(key string) error {
	h := hash64str(key)
	e.local.del(h)

	go func() {
		nodes := e.origin.hash(key, ReplicaCount)
		for _, node := range nodes {
			e.origin.sendDelete(node, key)
		}
	}()

	return nil
}

type AsyncReplicator struct {
	local   *Storage
	cluster *Cluster
	queue   chan replicaJob
	workers int
}

type replicaJob struct {
	key  string
	data []byte
}

func NewAsyncReplicator(local *Storage, cluster *Cluster, workers int) *AsyncReplicator {
	r := &AsyncReplicator{
		local:   local,
		cluster: cluster,
		queue:   make(chan replicaJob, 10000),
		workers: workers,
	}

	for i := 0; i < workers; i++ {
		go r.worker()
	}

	return r
}

func (r *AsyncReplicator) Write(key string, data []byte) error {
	if err := r.local.Set(key, data); err != nil {
		return err
	}

	select {
	case r.queue <- replicaJob{key: key, data: data}:
	default:
	}

	return nil
}

func (r *AsyncReplicator) worker() {
	for job := range r.queue {
		for i := 0; i < 5; i++ {
			if err := r.cluster.write(job.key, job.data); err == nil {
				break
			}
			time.Sleep(time.Duration(1<<i) * 100 * time.Millisecond)
		}
	}
}
