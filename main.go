package main

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	listenAddress = ":8080"
)

func main() {
	prometheus.MustRegister(realtimeRequestsByPath)
	prometheus.MustRegister(realtimeRequestsByCode)

	//go fetchRealtimeRequestsByPath()
	//go fetchRealtimeRequestsByCode()
	go fetchReport()

	http.Handle("/metrics", promhttp.Handler())

	log.Printf("HTTP server listening on %s", listenAddress)
	if err := http.ListenAndServe(listenAddress, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %v", err)
	}
}
