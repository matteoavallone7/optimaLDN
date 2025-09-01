package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"log"
	"net/http"
	"os"
	"time"
)

var influxClient influxdb2.Client
var influxWriteAPI api.WriteAPIBlocking

var influxDBUrl string
var influxOrg string
var influxBucket string
var influxDBToken string

var tflAPIKey string

func init() {
	log.Println("Lambda cold start: Initializing...")

	tflAPIKey = os.Getenv("TFL_API_KEY")
	if tflAPIKey == "" {
		log.Fatal("TFL_API_KEY environment variable is not set. Please configure it in Lambda settings.")
	}
	log.Printf("Using TfL key: %q", tflAPIKey)

	influxDBUrl = os.Getenv("INFLUXDB_URL")
	influxOrg = os.Getenv("INFLUXDB_ORG")
	influxBucket = os.Getenv("INFLUXDB_BUCKET")
	influxDBToken = os.Getenv("INFLUXDB_TOKEN")

	if influxDBUrl == "" || influxOrg == "" || influxBucket == "" || influxDBToken == "" {
		log.Fatal("One or more InfluxDB environment variables (INFLUXDB_URL, INFLUXDB_ORG, INFLUXDB_BUCKET, INFLUXDB_TOKEN) not set. Please configure them in Lambda settings.")
	}
}

func FetchLineStatus() (*common.TfLLineStatusResponse, error) {

	url := fmt.Sprintf(
		"https://api.tfl.gov.uk/Line/Mode/tube,dlr,bus/Status?app_key=%s",
		tflAPIKey,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add the User-Agent header
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.36")

	client := http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error calling TfL API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TfL Journey API returned status %s", resp.Status)
	}

	var result common.TfLLineStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &result, nil

}

func handler(ctx context.Context) error {
	log.Println("Lambda function invoked for TfL status update.")

	tflStatusResponse, fetchErr := FetchLineStatus()
	if fetchErr != nil {
		log.Printf("Error fetching TfL status: %v", fetchErr)
		return fmt.Errorf("failed to fetch TfL status: %w", fetchErr)
	}

	if influxClient == nil {
		influxClient = influxdb2.NewClient(influxDBUrl, influxDBToken)
		influxWriteAPI = influxClient.WriteAPIBlocking(influxOrg, influxBucket)
		log.Println("InfluxDB client initialized.")
	}
	defer influxClient.Close()

	points := []*write.Point{}
	for _, line := range *tflStatusResponse {
		for _, status := range line.LineStatuses {
			// Create a new InfluxDB point for each line status
			p := influxdb2.NewPointWithMeasurement("tfl_line_status").
				// Add tags
				AddTag("line_id", line.ID).
				AddTag("line_name", line.Name).
				AddTag("mode_name", line.ModeName).
				// Add fields
				AddField("status_severity", float64(status.StatusSeverity)).
				AddField("status_severity_description", status.StatusSeverityDescription).
				SetTime(time.Now())

			if status.Reason != "" {
				p.AddField("reason", status.Reason)
			}
			isDisrupted := status.StatusSeverity != 10 // Assuming 10 means "Good Service"
			p.AddField("is_disrupted", isDisrupted)

			points = append(points, p)
		}
	}

	// Write all collected points to InfluxDB
	if len(points) > 0 {
		writeErr := influxWriteAPI.WritePoint(ctx, points...)
		if writeErr != nil {
			log.Printf("Error writing to InfluxDB: %v", writeErr)
			return fmt.Errorf("failed to write %d points to InfluxDB: %w", len(points), writeErr)
		}
		log.Printf("Successfully wrote %d points to InfluxDB.", len(points))
	} else {
		log.Println("No TfL status points to write to InfluxDB.")
	}

	log.Println("TfL status update and storage to InfluxDB completed successfully.")
	return nil
}

func main() {
	lambda.Start(handler)
}
