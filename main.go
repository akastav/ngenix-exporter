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

var addr = flag.String("listen-address", ":8080",
	"The address to listen on for HTTP requests.")

var totalRealtimeRequests = prometheus.NewSummary(
	prometheus.SummaryOpts{
		Name: "realtime_requests_total",
		Help: "Total count of realtime requests",
	},
)

var realtimeRequestsByPath = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "realtime_requests_by_path",
		Help: "Realtime requests grouped by path",
	},
	[]string{"path"},
)

var realtimeRequestsByCode = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "realtime_requests_by_code",
		Help: "Realtime requests grouped by code",
	},
	[]string{"code"},
)

type TOP100 struct {
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

type HTTPSTATUS struct {
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

	go safelyExecute(fetchRealtimeRequestsByPath)
	go safelyExecute(fetchRealtimeRequestsByCode)

	http.Handle("/metrics", promhttp.Handler())

	log.Printf("HTTP server listening on %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %v", err)
	}
}

func safelyExecute(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic: %v", r)
		}
	}()
	fn()
}

func fetchRealtimeRequestsByPath() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			var response TOP100
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
				totalRealtimeRequests.Observe(float64(category.Metrics.RealtimeRequests))
			}
		}
	}()
}

func fetchRealtimeRequestsByCode() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			var exporter HTTPSTATUS
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
	}()
}

func fetchDataTOP100(data *TOP100) error {
	if data == nil {
		return errors.New("data parameter is nil")
	}

	username := os.Getenv("NGENIX_USERNAME")
	password := os.Getenv("NGENIX_PASSWORD")
	if username == "" || password == "" {
		return errors.New("missing basic auth credentials")
	}

	url, err := getTop100URL()
	if err != nil {
		return fmt.Errorf("error getting top 100 URL: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)

	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.SetBasicAuth(username, password)

	client := &http.Client{}
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

func fetchDataHTTPSTATUS(data *HTTPSTATUS) error {
	if data == nil {
		return errors.New("data parameter is nil")
	}

	username := os.Getenv("NGENIX_USERNAME")
	password := os.Getenv("NGENIX_PASSWORD")
	if username == "" || password == "" {
		return errors.New("missing basic auth credentials")
	}

	url, err := getHTTPStatusURL()
	if err != nil {
		return fmt.Errorf("error getting http status URL: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)

	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.SetBasicAuth(username, password)

	client := &http.Client{}
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

func getTop100URL() (string, error) {
	configID := os.Getenv("NGENIX_CONFIG_ID")
	if configID == "" {
		return "", fmt.Errorf("NGENIX_CONFIG_ID environment variable is not set")
	}

	apiURL := fmt.Sprintf("https://api.ngenix.net/reports/v1/analytical/top100?%s&date=2025-01-02&metrics=realtimeRequests", configID)
	return apiURL, nil
}

func getHTTPStatusURL() (string, error) {
	configID := os.Getenv("NGENIX_CONFIG_ID")
	if configID == "" {
		return "", fmt.Errorf("environment variable NGENIX_CONFIG_ID is not set")
	}

	httpStatusURL := fmt.Sprintf(
		"https://api.ngenix.net/reports/v1/analytical/httpstatuses?%s&start=2025-01-02T00:00:00&end=2025-01-02T23:59:59&metrics=realtimeRequests",
		configID,
	)
	return httpStatusURL, nil
}
