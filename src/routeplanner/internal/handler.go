package internal

import (
	"encoding/json"
	"fmt"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

func FetchRoutes(from, to string, departure time.Time) (*common.TFLJourneyResponse, error) {

	formattedDate := departure.Format("20060102")
	formattedTime := departure.Format("1504")

	url := fmt.Sprintf(
		"https://api.tfl.gov.uk/Journey/JourneyResults/%s/to/%s?date=%s&time=%s&timeIs=Departing&app_key=%s",
		from,
		to,
		formattedDate,
		formattedTime,
		TflAPIKey,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add the User-Agent header
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.36")

	client := http.Client{Timeout: 10 * time.Second}

	fmt.Println("TfL Request URL:", url)
	resp, err := client.Do(req)
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
		return nil, fmt.Errorf("TfL Crowding API returned status %s", resp.Status)
	}

	var data common.CrowdingResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
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
