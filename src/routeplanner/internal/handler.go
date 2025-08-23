package internal

import (
	"encoding/json"
	"fmt"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"net/http"
	"net/url"
	"time"
)

func FetchRoutes(from, to string, departure time.Time) (*common.TFLJourneyResponse, error) {

	formattedDate := departure.Format("20060102")
	formattedTime := departure.Format("15:04")

	url := fmt.Sprintf(
		"https://api.tfl.gov.uk/Journey/JourneyResults/%s/to/%s?date=%s&time=%s&timeIs=Departing&app_key=%s",
		from,
		to,
		formattedDate,
		formattedTime,
		TflAPIKey,
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

	var result common.TFLJourneyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &result, nil
}

func FetchCrowding(naptan, weekday string) (*common.CrowdingResp, error) {
	url := fmt.Sprintf("https://api.tfl.gov.uk/crowding/%s/%s", naptan, weekday)

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TfL Crowding API returned status %s", resp.Status)
	}

	var data common.CrowdingResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

func NotifyUser(userID, msg string) {
	_, err := http.PostForm("http://api-gateway/send-notification", url.Values{
		"userID": {userID},
		"msg":    {msg},
	})
	if err != nil {
		return
	}
}
