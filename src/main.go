package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	MaxValueSize         = 100 * 1024 * 1024
	MaxRequestSize       = 100 * 1024 * 1024
	WriteTimeout         = 30 * time.Second
	ReadTimeout          = 30 * time.Second
	WorkerPoolSize       = 100
	RateLimit            = 10000
	HeartbeatInterval    = 10 * time.Second
	NodeTimeout          = 35 * time.Second
	ReplicationFactor    = 3
	CompressionThreshold = 4096
)

type Vault struct {
	storage  *Storage
	cluster  *Cluster
	limiter  *RateLimiter
	isMaster bool
}

var vault *Vault

func main() {
	mode := flag.String("mode", "worker", "Mode: worker or master")
	port := flag.Int("port", 3000, "Port to listen on")
	publicURL := flag.String("public-url", "", "Public URL for this worker")
	masterURL := flag.String("master", "", "Master server URL")
	dataDir := flag.String("data", "/data", "Data directory")
	volumes := flag.String("volumes", "", "Comma-separated volume URLs (master mode)")
	authKey := flag.String("auth", "", "Shared secret for cluster auth")
	flag.Parse()

	if *publicURL == "" {
		*publicURL = fmt.Sprintf("http://localhost:%d", *port)
	}

	if *mode == "master" {
		runMaster(*port, *volumes)
		return
	}

	storage, err := NewStorage(*dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	cluster := NewCluster(*publicURL, *masterURL, *authKey, storage)

	vault = &Vault{
		storage:  storage,
		cluster:  cluster,
		limiter:  NewRateLimiter(RateLimit),
		isMaster: false,
	}

	if *masterURL != "" {
		go cluster.registerWithMaster()
		go cluster.heartbeatLoop()
		go cluster.syncNodesLoop()
	}

	server := &http.Server{
		Addr:           fmt.Sprintf(":%d", *port),
		Handler:        vault,
		ReadTimeout:    ReadTimeout,
		WriteTimeout:   WriteTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	log.Printf("Worker starting on %s (public: %s)", server.Addr, *publicURL)
	log.Fatal(server.ListenAndServe())
}
