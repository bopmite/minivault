package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"
)

const (
	MaxValueSize   = 100 * 1024 * 1024
	MaxRequestSize = 100 * 1024 * 1024
	MaxCacheSize   = 512 * 1024 * 1024
	WriteTimeout   = 30 * time.Second
	ReadTimeout    = 30 * time.Second
	WorkerPool     = 100
	RateLimit      = 100000
	Heartbeat      = 10 * time.Second
	NodeTimeout    = 35 * time.Second
	ReplicaCount   = 3
)

type Vault struct {
	storage *Storage
	cluster *Cluster
	limiter *RateLimiter
}

func main() {
	mode := flag.String("mode", "worker", "worker or master")
	port := flag.Int("port", 3000, "port")
	pubURL := flag.String("public-url", "", "public url")
	master := flag.String("master", "", "master url")
	dataDir := flag.String("data", "/data", "data dir")
	volumes := flag.String("volumes", "", "volume urls")
	authKey := flag.String("auth", "", "auth key")
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	if *pubURL == "" {
		*pubURL = fmt.Sprintf("http://localhost:%d", *port)
	}

	if *mode == "master" {
		runMaster(*port, *volumes)
		return
	}

	storage, err := NewStorage(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	vault := &Vault{
		storage: storage,
		cluster: NewCluster(*pubURL, *master, *authKey, storage),
		limiter: NewRateLimiter(RateLimit),
	}

	if *master != "" {
		go vault.cluster.start()
	}

	srv := &http.Server{
		Addr:           fmt.Sprintf(":%d", *port),
		Handler:        vault,
		ReadTimeout:    ReadTimeout,
		WriteTimeout:   WriteTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	log.Printf("starting on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
