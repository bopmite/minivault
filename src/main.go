package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Vault struct {
	selfURL string
	cluster *Cluster
}

func main() {
	mode := flag.String("mode", "worker", "Mode: worker or master")
	port := flag.Int("port", 3000, "Port to listen on")
	publicURL := flag.String("public-url", "", "Public URL for this worker (required in worker mode)")
	masterURL := flag.String("master", "", "Master server URL (required in worker mode)")
	dataDir := flag.String("data", "./data", "Data directory")
	volumes := flag.String("volumes", "", "Comma-separated list of volume URLs (master mode only)")
	flag.Parse()

	if *mode == "master" {
		runMaster(*port, *volumes)
		return
	}

	if *publicURL == "" {
		*publicURL = fmt.Sprintf("http://localhost:%d", *port)
	}

	InitStorage(*dataDir)
	cluster := InitCluster(*publicURL, *masterURL)

	v := &Vault{
		selfURL: *publicURL,
		cluster: cluster,
	}

	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 1000
	http.DefaultTransport.(*http.Transport).MaxConnsPerHost = 1000

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      v,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("Worker starting on %s (public: %s, master: %s)", fmt.Sprintf(":%d", *port), *publicURL, *masterURL)
	log.Fatal(server.ListenAndServe())
}
