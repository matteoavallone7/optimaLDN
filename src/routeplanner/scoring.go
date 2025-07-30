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

func EstimateCurrentStop(route common.ChosenRoute) (string, error) {
	now := time.Now()

	// 1. Check if journey is already completed
	if len(route.Legs) == 0 {
		return "", fmt.Errorf("no legs in the route")
	}
	lastLeg := route.Legs[len(route.Legs)-1]
	journeyEndTime, err := time.Parse(time.RFC3339, lastLeg.EndTime)
	if err == nil && now.After(journeyEndTime) {
		return "", fmt.Errorf("journey already completed")
	}

	// 2. Iterate over legs and find current one based on time
	for _, leg := range route.Legs {
		depTime, err1 := time.Parse(time.RFC3339, leg.StartTime)
		arrTime, err2 := time.Parse(time.RFC3339, leg.EndTime)

		if err1 != nil || err2 != nil {
			continue // Skip legs with invalid timestamps
		}

		// Check if 'now' is within the current leg's time frame (inclusive of start, exclusive of end)
		// Using !now.Before(depTime) is equivalent to now.After(depTime) || now.Equal(depTime)
		if !now.Before(depTime) && now.Before(arrTime) {
			// 3. Estimate current stop based on progress through this leg
			if len(leg.StopIDs) == 0 {
				return "", fmt.Errorf("no stops available in current leg")
			}

			progress := now.Sub(depTime).Seconds() / arrTime.Sub(depTime).Seconds()
			index := int(progress * float64(len(leg.StopIDs)))
			if index >= len(leg.StopIDs) {
				index = len(leg.StopIDs) - 1 // Clamp to last index
			}
			return leg.StopIDs[index], nil
		}
	}

	return "", fmt.Errorf("unable to estimate current stop from time range")
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

func ConvertToActiveRoute(userID string, route *common.ChosenRoute) *common.ActiveRoute {
	// Extract all line IDs from the route
	lineNameSet := make(map[string]struct{})
	for _, leg := range route.Legs {
		lineNameSet[leg.LineName] = struct{}{}
	}

	var lineIDs []string
	for id := range lineNameSet {
		lineIDs = append(lineIDs, id)
	}

	return &common.ActiveRoute{
		UserID:  userID,
		LineIDs: lineIDs,
	}
}
