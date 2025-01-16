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

func fetchRealtimeRequestsByCode() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		var httpStatus httpStatusResponse
		if err := fetchDataHTTPStatus(&httpStatus); err != nil {
			log.Printf("Error fetching data: %v", err)
			continue
		}

		if httpStatus.ModelName == "" || httpStatus.Categories == nil {
			log.Println("Incomplete data received")
			continue
		}

		for _, category := range httpStatus.Categories {
			if category.Name == "" || category.Metrics.RealtimeRequests == 0 {
				continue
			}

			metric := realtimeRequestsByCode.WithLabelValues(category.Name)
			if metric != nil {
				metric.Set(float64(category.Metrics.RealtimeRequests))
			}
		}
	}
}

func fetchDataHTTPStatus(data *httpStatusResponse) error {
	if data == nil {
		return errors.New("data parameter is nil")
	}

	username, password := os.Getenv("NGENIX_USERNAME"), os.Getenv("NGENIX_PASSWORD")
	if username == "" || password == "" {
		return errors.New("missing basic auth credentials")
	}

	configID := os.Getenv("NGENIX_CONFIG_ID")
	date := time.Now()
	metrics := []string{"realtimeRequests"}

	url := getHTTPStatusURL(configID, date, metrics)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.SetBasicAuth(username, password)

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	err = json.NewDecoder(resp.Body).Decode(data)
	if err != nil {
		return fmt.Errorf("error decoding response: %w", err)
	}

	return nil
}

func getHTTPStatusURL(configId string, date time.Time, metrics []string) string {
	if configId == "" || date.IsZero() || metrics == nil {
		return ""
	}

	params := url.Values{}
	params.Set("configId", configId)
	params.Set("start", date.Format("2006-01-02")+"T00:00:00")
	params.Set("end", date.Format("2006-01-02")+"T23:59:59")
	params.Set("metrics", strings.Join(metrics, ","))

	return fmt.Sprintf("https://api.ngenix.net/reports/v1/analytical/httpstatuses?%s", params.Encode())
}
