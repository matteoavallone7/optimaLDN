package manager

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

// InfluxDB client variables
var influxClient influxdb2.Client
var influxWriteAPI api.WriteAPIBlocking

// Environment variables for InfluxDB connection and token
var influxDBUrl string
var influxOrg string
var influxBucket string
var influxDBToken string

var tflAPIKey string

func init() {
	log.Println("Lambda cold start: Initializing...")

	// Retrieve TFL API Key directly from environment variable
	tflAPIKey = os.Getenv("TFL_API_KEY")
	if tflAPIKey == "" {
		log.Fatal("TFL_API_KEY environment variable is not set. Please configure it in Lambda settings.")
	}

	// InfluxDB specific environment variables for connection and token
	influxDBUrl = os.Getenv("INFLUXDB_URL")
	influxOrg = os.Getenv("INFLUXDB_ORG")
	influxBucket = os.Getenv("INFLUXDB_BUCKET")
	influxDBToken = os.Getenv("INFLUXDB_TOKEN") // Read InfluxDB token directly from environment variable

	if influxDBUrl == "" || influxOrg == "" || influxBucket == "" || influxDBToken == "" {
		log.Fatal("One or more InfluxDB environment variables (INFLUXDB_URL, INFLUXDB_ORG, INFLUXDB_BUCKET, INFLUXDB_TOKEN) not set. Please configure them in Lambda settings.")
	}
}

func FetchLineStatus() (*common.TfLLineStatusResponse, error) {

	url := fmt.Sprintf(
		"https://api.tfl.gov.uk/Line/Mode/tube,dlr,bus/Status?app_key=%s",
		tflAPIKey,
	)

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
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

	// 2. Initialize InfluxDB Client if not already initialized (optimizes for warm starts)
	// The InfluxDB token is now directly available from the global influxDBToken variable
	if influxClient == nil {
		influxClient = influxdb2.NewClient(influxDBUrl, influxDBToken)
		influxWriteAPI = influxClient.WriteAPIBlocking(influxOrg, influxBucket)
		log.Println("InfluxDB client initialized.")
	}
	// Ensure the InfluxDB client is closed after the handler finishes
	// This flushes any pending writes and cleans up resources.
	defer influxClient.Close()

	// 3. Transform TfL data into InfluxDB points and write
	points := []*write.Point{}
	for _, line := range *tflStatusResponse {
		for _, status := range line.LineStatuses {
			// Create a new InfluxDB point for each line status
			p := influxdb2.NewPointWithMeasurement("tfl_line_status").
				// Add tags (indexed dimensions for filtering/grouping)
				AddTag("line_id", line.ID).
				AddTag("line_name", line.Name).
				AddTag("mode_name", line.ModeName).
				// Add fields (the actual values you want to measure)
				AddField("status_severity", float64(status.StatusSeverity)). // Use float64 for numeric measures
				AddField("status_severity_description", status.StatusSeverityDescription).
				// Set the timestamp for the record. time.Now() is appropriate for real-time data.
				SetTime(time.Now())

			// Add optional reason field if present
			if status.Reason != "" {
				p.AddField("reason", status.Reason)
			}
			// Add a boolean field to easily identify disrupted lines
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

	log.Println("TfL status update and storage to Amazon Timestream for InfluxDB completed successfully.")
	return nil
}

func main() {
	lambda.Start(handler)
}
