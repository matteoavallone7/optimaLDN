package user_service

import (
	"github.com/google/uuid"
	"github.com/matteoavallone7/optimaLDN/src/common"
)

func ConvertToUserSavedRoute(userID string, chosen common.ChosenRoute) common.UserSavedRoute {
	var lineNames []string
	var stopsNames []string

	for _, leg := range chosen.Legs {
		lineNames = append(lineNames, leg.LineName)
		stopsNames = append(stopsNames, leg.Stops...)
	}

	return common.UserSavedRoute{
		RouteID:       uuid.New().String(),
		UserID:        userID,
		StartPoint:    chosen.Legs[0].From,
		EndPoint:      chosen.Legs[len(chosen.Legs)-1].To,
		TransportMode: chosen.Legs[0].Mode,
		Stops:         len(stopsNames),
		EstimatedTime: chosen.TotalDuration,
		LineNames:     lineNames,
		StopsNames:    stopsNames,
	}
}
