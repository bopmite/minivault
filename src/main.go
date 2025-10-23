package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"runtime"
	"time"
)

const (
	MaxValueSize   = 100 * 1024 * 1024
	MaxRequestSize = 100 * 1024 * 1024
	MaxCacheSize   = 512 * 1024 * 1024
	WriteTimeout   = 30 * time.Second
	ReadTimeout    = 30 * time.Second
	WorkerPool     = 50
	RateLimit      = 100000
	Heartbeat      = 10 * time.Second
	NodeTimeout    = 35 * time.Second
	ReplicaCount   = 3
)

type Vault struct {
	storage *Storage
	cluster *Cluster
	limiter *RateLimiter
	edge    *EdgeCache
	async   *AsyncReplicator
}

func main() {
	port := flag.Int("port", 3000, "port")
	pubURL := flag.String("public-url", "", "public url")
	dataDir := flag.String("data", "/data", "data dir")
	authKey := flag.String("auth", "", "auth key")
	edgeMode := flag.Bool("edge", false, "edge mode")
	asyncMode := flag.Bool("async", false, "async replication")
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	if *pubURL == "" {
		*pubURL = fmt.Sprintf("localhost:%d", *port)
	}

	storage, err := NewStorage(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	cluster := NewCluster(*pubURL, *authKey, storage)

	vault := &Vault{
		storage: storage,
		cluster: cluster,
		limiter: NewRateLimiter(RateLimit),
	}

	if *edgeMode {
		vault.edge = NewEdgeCache([]string{}, cluster)
	}

	if *asyncMode {
		vault.async = NewAsyncReplicator(storage, cluster, 10)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
	}

	server := NewBinaryServer(vault)

	log.Printf("starting on %s", ln.Addr())
	log.Fatal(server.Serve(ln))
}
