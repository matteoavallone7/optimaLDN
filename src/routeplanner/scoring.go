package routeplanner

import (
	"fmt"
	"github.com/matteoavallone7/optimaLDN/src/common"
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

/* func EstimateCurrentStop(journey common.TFLJourney, startTime time.Time) string {
	now := time.Now()
	elapsed := now.Sub(startTime)

	var timePassed int
	for _, leg := range journey.Legs {
		timePassed += leg. * 60 // seconds
		if elapsed.Seconds() < float64(timePassed) {
			return leg.DeparturePoint.NaptanID
		}
	}
	return journey.Legs[len(journey.Legs)-1].ArrivalPoint.NaptanID // Last stop
}*/

func EstimateCurrentStop(journey common.TFLJourney, journeyStart time.Time) (string, error) {
	now := time.Now()
	for _, leg := range journey.Legs {
		depTime, err := time.Parse(time.RFC3339, leg.DepartureTime)
		if err != nil {
			continue
		}
		arrTime, err := time.Parse(time.RFC3339, leg.ArrivalTime)
		if err != nil {
			continue
		}

		// If current time is within this leg's window
		if now.After(depTime) && now.Before(arrTime) {
			// Estimate mid-point stop as current
			mid := len(leg.Path.StopPoints) / 2
			if mid >= 0 && mid < len(leg.Path.StopPoints) {
				return leg.Path.StopPoints[mid].ID, nil
			}
		}
	}

	// If time is past journey duration, return last stop
	if len(journey.Legs) > 0 {
		lastLeg := journey.Legs[len(journey.Legs)-1]
		if len(lastLeg.Path.StopPoints) > 0 {
			return lastLeg.Path.StopPoints[len(lastLeg.Path.StopPoints)-1].ID, nil
		}
	}

	return "", fmt.Errorf("unable to estimate current stop")
}

func ConvertToChosenRoute(userID string, tflJourney common.TFLJourney) common.ChosenRoute {
	var legs []common.RouteLeg

	for _, leg := range tflJourney.Legs {
		// Extract line info (if any)
		var lineName, lineID string
		if len(leg.RouteOptions) > 0 {
			lineName = leg.RouteOptions[0].LineIdentifier.Name
			lineID = leg.RouteOptions[0].LineIdentifier.ID
		}

		var stops, stopIDs []string
		for _, stop := range leg.Path.StopPoints {
			stops = append(stops, stop.Name)
			stopIDs = append(stopIDs, stop.ID)
		}

		routeLeg := common.RouteLeg{
			From:        leg.DeparturePoint.CommonName,
			FromID:      leg.DeparturePoint.NaptanID,
			To:          leg.ArrivalPoint.CommonName,
			ToID:        leg.ArrivalPoint.NaptanID,
			Mode:        leg.Mode.Name,
			StartTime:   leg.DepartureTime,
			EndTime:     leg.ArrivalTime,
			Description: leg.Instruction.Summary,
			LineName:    lineName,
			LineID:      lineID,
			Stops:       stops,
			StopIDs:     stopIDs,
		}

		legs = append(legs, routeLeg)
	}

	return common.ChosenRoute{
		UserID:        userID,
		TotalDuration: tflJourney.Duration,
		Description:   fmt.Sprintf("Journey with %d legs", len(tflJourney.Legs)),
		Legs:          legs,
	}
}
