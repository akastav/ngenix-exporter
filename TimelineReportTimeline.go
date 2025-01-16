package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricName = "realtime_traffic"
	metricHelp = "Realtime traffic report"
)

var (
	trafficCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "ngenix",
			Subsystem: "realtime",
			Name:      metricName,
			Help:      metricHelp,
		},
		[]string{"httpStatus", "realtimeTraffic"},
	)
	mu sync.Mutex
)

type Report struct {
	Query struct {
		Metrics []string `json:"metrics"`
		Filters struct {
			ConfigID  int    `json:"configId"`
			ModelName string `json:"modelName"`
		} `json:"filters"`
		GroupBy   []string `json:"groupBy"`
		Start     string   `json:"start"`
		End       string   `json:"end"`
		Interval  int      `json:"interval"`
		ModelName string   `json:"modelName"`
	} `json:"query"`
	GroupedByValuesDescription struct {
		HTTPStatus struct {
			Num101 string `json:"101"`
			Num200 string `json:"200"`
			Num201 string `json:"201"`
			Num202 string `json:"202"`
			Num204 string `json:"204"`
			Num301 string `json:"301"`
			Num302 string `json:"302"`
			Num400 string `json:"400"`
			Num401 string `json:"401"`
			Num403 string `json:"403"`
			Num404 string `json:"404"`
			Num405 string `json:"405"`
			Num406 string `json:"406"`
			Num409 string `json:"409"`
			Num410 string `json:"410"`
			Num413 string `json:"413"`
			Num415 string `json:"415"`
			Num500 string `json:"500"`
			Num502 string `json:"502"`
			Num503 string `json:"503"`
			Num504 string `json:"504"`
		} `json:"httpStatus"`
	} `json:"groupedByValuesDescription"`
	Data []struct {
		Timestamp time.Time `json:"timestamp"`
		Values    []struct {
			GroupedBy struct {
				HTTPStatus int    `json:"httpStatus"`
				ModelName  string `json:"modelName"`
			} `json:"groupedBy"`
			Metrics struct {
				RealtimeTraffic int `json:"realtimeTraffic"`
			} `json:"metrics"`
		} `json:"values"`
		ModelName string `json:"modelName"`
	} `json:"data"`
	Summary []struct {
		GroupedBy struct {
			HTTPStatus int    `json:"httpStatus"`
			ModelName  string `json:"modelName"`
		} `json:"groupedBy"`
		Metrics struct {
			RealtimeTraffic struct {
				Max int     `json:"max"`
				Min int     `json:"min"`
				Avg float64 `json:"avg"`
			} `json:"realtimeTraffic"`
			ModelName string `json:"modelName"`
		} `json:"metrics"`
		ModelName string `json:"modelName"`
	} `json:"summary"`
	ModelName string `json:"modelName"`
}

func init() {
	prometheus.MustRegister(trafficCounter)
}

func fetchReport() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		var report Report
		if err := fetchData(&report); err != nil {
			log.Printf("error fetching data: %v", err)
			continue
		}

		processReport(&report)
	}
}

func fetchData(report *Report) error {
	log.Println("Fetching data from NGENIX API")

	username, password := os.Getenv("NGENIX_USERNAME"), os.Getenv("NGENIX_PASSWORD")
	if username == "" || password == "" {
		return errors.New("missing basic auth credentials")
	}

	configID := os.Getenv("NGENIX_CONFIG_ID")
	if configID == "" {
		return errors.New("missing config id")
	}

	url := buildReportURL(configID, time.Now())
	log.Printf("Fetching data from URL: %s", url)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.SetBasicAuth(username, password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	log.Println("Decoding JSON response")
	return json.NewDecoder(resp.Body).Decode(report)
}

func buildReportURL(configID string, date time.Time) string {
	return fmt.Sprintf("https://api.ngenix.net/reports/v1/timeline/configs?configId=%s&start=%s&end=%s&metrics=realtimeRequests&interval=30&groupBy=httpStatus",
		configID,
		date.Format("2006-01-02")+"T09:00:00",
		date.Format("2006-01-02")+"T09:59:59")
}

func processReport(report *Report) {
	log.Println("Processing report")

	if report == nil {
		log.Println("warning: report is nil")
		return
	}

	if report.Data == nil {
		log.Println("warning: report.Data is nil")
		return
	}

	mu.Lock()
	defer mu.Unlock()

	for _, data := range report.Data {
		for _, value := range data.Values {
			trafficCounter.WithLabelValues(strconv.Itoa(value.GroupedBy.HTTPStatus)).Add(float64(value.Metrics.RealtimeTraffic))
		}
	}
}
