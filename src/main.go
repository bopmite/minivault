package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"time"
)

const (
	MaxValueSize = 100 * 1024 * 1024
	MaxCacheSize = 512 * 1024 * 1024
	WriteTimeout = 30 * time.Second
	WorkerPool   = 50
	ReplicaCount = 3
)

type AuthMode int

const (
	AuthNone AuthMode = iota
	AuthWrites
	AuthAll
)

type Vault struct {
	storage *Storage
	cluster *Cluster
}

func main() {
	port := flag.Int("port", 3000, "port")
	pubURL := flag.String("public-url", "", "public url")
	dataDir := flag.String("data", "/data", "data dir")
	authKey := flag.String("auth", "", "auth key")
	authMode := flag.String("authmode", "none", "auth mode: none, writes, all")
	rateLimit := flag.Int("ratelimit", 0, "rate limit (ops/sec, 0=unlimited)")
	cacheSize := flag.Int64("cache", 512, "cache size (MB)")
	workers := flag.Int("workers", 50, "worker pool size")
	httpPort := flag.Int("http", 0, "http port (0=disabled)")
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	if *pubURL == "" {
		*pubURL = fmt.Sprintf("localhost:%d", *port)
	}

	var mode AuthMode
	switch *authMode {
	case "none":
		mode = AuthNone
	case "writes":
		mode = AuthWrites
	case "all":
		mode = AuthAll
	default:
		log.Fatalf("invalid authmode: %s (use: none, writes, all)", *authMode)
	}

	if mode != AuthNone && *authKey == "" {
		log.Fatal("auth key required when authmode is not 'none'")
	}

	MaxCacheSizeRuntime := *cacheSize * 1024 * 1024

	storage, err := NewStorage(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	storage.maxSize = MaxCacheSizeRuntime

	cluster := NewCluster(*pubURL, *authKey, storage, *workers)

	vault := &Vault{
		storage: storage,
		cluster: cluster,
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
	}

	startTime := time.Now()
	server := NewBinaryServer(vault, *authKey, mode, *rateLimit, startTime)

	if *httpPort > 0 {
		httpServer := NewHTTPServer(vault, startTime)
		go func() {
			log.Printf("http server on :%d", *httpPort)
			if err := http.ListenAndServe(fmt.Sprintf(":%d", *httpPort), httpServer); err != nil {
				log.Fatal(err)
			}
		}()
	}

	log.Printf("starting on %s (auth=%s, ratelimit=%d, cache=%dMB, workers=%d)",
		ln.Addr(), *authMode, *rateLimit, *cacheSize, *workers)
	log.Fatal(server.Serve(ln))
}
