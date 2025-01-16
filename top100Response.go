package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	realtimeRequestsByPath = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "ngenix",
			Subsystem: "realtime",
			Name:      "requests_by_path",
			Help:      "Realtime requests grouped by path",
		},
		[]string{"path"},
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

func fetchRealtimeRequestsByPath() {
	ticker := time.NewTicker(5 * time.Second)
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

			if v := realtimeRequestsByPath.WithLabelValues(category.Name); v != nil {
				v.Set(float64(category.Metrics.RealtimeRequests))
			}
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

	configId := os.Getenv("NGENIX_CONFIG_ID")
	date := time.Now()
	metrics := []string{"realtimeRequests"}

	url := getTop100URL(configId, date, metrics)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.SetBasicAuth(username, password)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	if err := json.NewDecoder(resp.Body).Decode(data); err != nil {
		return fmt.Errorf("error decoding response: %w", err)
	}

	return nil
}

func getTop100URL(configId string, date time.Time, metrics []string) string {
	if configId == "" || date.IsZero() || metrics == nil {
		return ""
	}

	params := url.Values{}
	params.Set("configId", configId)
	params.Set("date", date.Format("2006-01-02"))
	params.Set("metrics", strings.Join(metrics, ","))

	return fmt.Sprintf("https://api.ngenix.net/reports/v1/analytical/top100?%s", params.Encode())
}
