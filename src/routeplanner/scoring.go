package routeplanner

import (
	"fmt"
	"time"
)

func GetNaptan(location string) (string, bool) {

	if NaptanMap == nil {
		return "", false
	}

	code, ok := NaptanMap[location]
	return code, ok
}

func TimeStringToTfLTimeBand(timeStr string) (string, error) {
	t, err := time.Parse("2006-01-02T15:04:05", timeStr)
	if err != nil {
		return "", fmt.Errorf("invalid time format: %w", err)
	}

	return TimeToTfLTimeBand(t), nil
}

func TimeToTfLTimeBand(t time.Time) string {
	minutes := (t.Minute() / 15) * 15
	start := time.Date(0, 1, 1, t.Hour(), minutes, 0, 0, time.UTC)
	end := start.Add(15 * time.Minute)
	return fmt.Sprintf("%02d:%02d-%02d:%02d", start.Hour(), start.Minute(), end.Hour(), end.Minute())
}

func GetScore(totalCrowding float64, totalStops, duration int) float64 {
	var avgCrowding float64
	if totalStops > 0 {
		avgCrowding = totalCrowding / float64(totalStops)
	}

	return float64(duration) * (1 + avgCrowding)
}
