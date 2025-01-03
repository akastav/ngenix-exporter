package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	listenAddress = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
)

var (
	totalRealtimeRequests = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: "ngenix",
			Subsystem: "realtime",
			Name:      "requests_total",
			Help:      "Total count of realtime requests",
		},
		[]string{"path"},
	)

	realtimeRequestsByPath = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "ngenix",
			Subsystem: "realtime",
			Name:      "requests_by_path",
			Help:      "Realtime requests grouped by path",
		},
		[]string{"path"},
	)

	realtimeRequestsByCode = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "ngenix",
			Subsystem: "realtime",
			Name:      "requests_by_code",
			Help:      "Realtime requests grouped by code",
		},
		[]string{"code"},
	)
)

type top100Response struct {
	Query struct {
		Metrics []string `json:"metrics"`
		Filters struct {
			ConfigID  int    `json:"configId"`
			ModelName string `json:"modelName"`
		} `json:"filters"`
		GroupBy   []interface{} `json:"groupBy"`
		Date      string        `json:"date"`
		ModelName string        `json:"modelName"`
	} `json:"query"`
	Categories []struct {
		Name    string `json:"name"`
		Metrics struct {
			RealtimeRequests int `json:"realtimeRequests"`
		} `json:"metrics"`
	} `json:"categories"`
	ModelName string `json:"modelName"`
}

type httpStatusResponse struct {
	Query struct {
		Metrics []string `json:"metrics"`
		Filters struct {
			ConfigID  int    `json:"configId"`
			ModelName string `json:"modelName"`
		} `json:"filters"`
		GroupBy   []interface{} `json:"groupBy"`
		Start     time.Time     `json:"start"`
		End       time.Time     `json:"end"`
		ModelName string        `json:"modelName"`
	} `json:"query"`
	Categories []struct {
		Name    string `json:"name"`
		Metrics struct {
			RealtimeRequests int `json:"realtimeRequests"`
		} `json:"metrics"`
	} `json:"categories"`
	ModelName string `json:"modelName"`
}

func main() {
	flag.Parse()

	prometheus.MustRegister(totalRealtimeRequests)
	prometheus.MustRegister(realtimeRequestsByPath)
	prometheus.MustRegister(realtimeRequestsByCode)

	go fetchRealtimeRequestsByPath()
	go fetchRealtimeRequestsByCode()

	http.Handle("/metrics", promhttp.Handler())

	log.Printf("HTTP server listening on %s", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %v", err)
	}
}

func fetchRealtimeRequestsByPath() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		var response top100Response
		if err := fetchDataTOP100(&response); err != nil {
			log.Printf("Error fetching data: %v", err)
			continue
		}

		if response.ModelName == "" || response.Categories == nil {
			log.Println("Incomplete data received")
			continue
		}

		for _, category := range response.Categories {
			if category.Name == "" || category.Metrics.RealtimeRequests == 0 {
				log.Printf("Invalid category: %v", category)
				continue
			}

			realtimeRequestsByPath.WithLabelValues(category.Name).Set(float64(category.Metrics.RealtimeRequests))
			totalRealtimeRequests.WithLabelValues(category.Name).Observe(float64(category.Metrics.RealtimeRequests))
		}
	}
}

func fetchRealtimeRequestsByCode() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		var exporter httpStatusResponse
		if err := fetchDataHTTPSTATUS(&exporter); err != nil {
			log.Printf("Error fetching data: %v", err)
			continue
		}

		if exporter.ModelName == "" {
			continue
		}

		if exporter.Categories == nil {
			continue
		}

		for _, category := range exporter.Categories {
			if category.Name == "" {
				continue
			}

			if category.Metrics.RealtimeRequests == 0 {
				continue
			}

			realtimeRequestsByCode.WithLabelValues(category.Name).Set(float64(category.Metrics.RealtimeRequests))
		}
	}
}

func fetchDataTOP100(data *top100Response) error {
	if data == nil {
		return errors.New("data parameter is nil")
	}

	username := os.Getenv("NGENIX_USERNAME")
	password := os.Getenv("NGENIX_PASSWORD")
	if username == "" || password == "" {
		return errors.New("missing basic auth credentials")
	}

	url := getTop100URL()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.SetBasicAuth(username, password)

	client := &http.Client{
		Transport: nil,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
		Jar:     nil,
		Timeout: 0,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error executing request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error fetching response: %v %v", resp.StatusCode, resp.Status)
	}

	if resp.Body == nil {
		return errors.New("error fetching response: response body is nil")
	}

	err = json.NewDecoder(resp.Body).Decode(data)
	if err != nil {
		return fmt.Errorf("error decoding response: %v", err)
	}

	return nil
}

func fetchDataHTTPSTATUS(data *httpStatusResponse) error {
	if data == nil {
		return errors.New("data parameter is nil")
	}

	username := os.Getenv("NGENIX_USERNAME")
	password := os.Getenv("NGENIX_PASSWORD")
	if username == "" || password == "" {
		return errors.New("missing basic auth credentials")
	}

	url := getHTTPStatusURL()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.SetBasicAuth(username, password)

	client := &http.Client{
		Transport: nil,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
		Jar:     nil,
		Timeout: 0,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error executing request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error fetching response: %v %v", resp.StatusCode, resp.Status)
	}

	if resp.Body == nil {
		return errors.New("error fetching response: response body is nil")
	}

	err = json.NewDecoder(resp.Body).Decode(data)
	if err != nil {
		return fmt.Errorf("error decoding response: %v", err)
	}

	return nil
}

// TODO: Add variables configId, date, metrics
func getTop100URL() string {
	return "https://api.ngenix.net/reports/v1/analytical/top100?configId=91051&date=2025-01-02&metrics=realtimeRequests"
}

func getHTTPStatusURL() string {
	return "https://api.ngenix.net/reports/v1/analytical/httpstatuses?configId=91051&start=2025-01-02T00:00:00&end=2025-01-02T23:59:59&metrics=realtimeRequests"
}
