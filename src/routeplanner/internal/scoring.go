package internal

import (
	"fmt"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"log"
	"strings"
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

	if len(route.Legs) == 0 {
		return "", fmt.Errorf("no legs in the route")
	}
	lastLeg := route.Legs[len(route.Legs)-1]
	log.Printf("📌 Last leg start: %s, end: %s", lastLeg.StartTime, lastLeg.EndTime)
	journeyEndTime, err := time.Parse("2006-01-02T15:04:05", lastLeg.EndTime)
	if err == nil && now.After(journeyEndTime) {
		return "", fmt.Errorf("journey already completed")
	}

	for _, leg := range route.Legs {
		loc, _ := time.LoadLocation("Europe/Rome")
		depTime, err1 := time.ParseInLocation("2006-01-02T15:04:05", leg.StartTime, loc)
		arrTime, err2 := time.ParseInLocation("2006-01-02T15:04:05", leg.EndTime, loc)

		if err1 != nil || err2 != nil {
			continue
		}

		if !now.Before(depTime) && now.Before(arrTime) {
			if len(leg.StopIDs) == 0 {
				return "", fmt.Errorf("no stops available in current leg")
			}

			progress := now.Sub(depTime).Seconds() / arrTime.Sub(depTime).Seconds()
			index := int(progress * float64(len(leg.StopIDs)))
			if index >= len(leg.StopIDs) {
				index = len(leg.StopIDs) - 1
			}
			return leg.StopIDs[index], nil
		}
	}

	return "", fmt.Errorf("unable to estimate current stop from time range")
}

func ConvertToChosenRoute(userID string, tflJourney common.TFLJourney) common.ChosenRoute {
	var legs []common.RouteLeg

	for _, leg := range tflJourney.Legs {
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

func ConvertUserSavedToChosenRoute(saved *common.UserSavedRoute) common.ChosenRoute {
	desc := fmt.Sprintf("%s → %s via %s", saved.StartPoint, saved.EndPoint, strings.Join(saved.LineNames, ", "))

	legs := make([]common.RouteLeg, len(saved.LineNames))
	for i := range saved.LineNames {
		leg := common.RouteLeg{
			From:        saved.StartPoint,
			To:          saved.EndPoint,
			Mode:        saved.TransportMode,
			LineName:    saved.LineNames[i],
			Stops:       []string{saved.StopsNames[i]},
			Description: fmt.Sprintf("%s via %s", saved.StartPoint, saved.LineNames[i]),
		}
		legs[i] = leg
	}

	return common.ChosenRoute{
		UserID:        saved.UserID,
		TotalDuration: saved.EstimatedTime,
		Description:   desc,
		Legs:          legs,
	}
}

func NormalizeStopID(stopID string) string {
	if strings.HasPrefix(stopID, "940G") {
		return "9400" + stopID[4:]
	}
	return stopID
}
