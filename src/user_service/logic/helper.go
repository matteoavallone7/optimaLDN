package logic

import (
	"github.com/google/uuid"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"io"
	"log"
	"net/http"
	"net/url"
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

func NotifyUser(userID, msg string) {
	resp, err := http.PostForm("http://api_gateway:8080/send-notification", url.Values{
		"userID": {userID},
		"msg":    {msg},
	})
	if err != nil {
		log.Printf("❌ Failed to send notification for user=%s: %v", userID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("⚠️ Notification API returned status %d: %s", resp.StatusCode, string(body))
	} else {
		log.Printf("✅ Notification sent successfully for user=%s", userID)
	}
}
