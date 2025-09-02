package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func Login(username, password string) (bool, error) {
	auth := common.Auth{
		UserID:   username,
		Password: password,
	}

	payload, _ := json.Marshal(auth)
	url := fmt.Sprintf("http://%slogin", baseURL)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return false, fmt.Errorf("error sending login request to %s", url)
	}
	defer resp.Body.Close()

	var result common.SavedResp
	json.NewDecoder(resp.Body).Decode(&result)

	return result.Status == common.StatusDone, nil
}

func ViewFavoriteRoutes(userID string) ([]string, error) {
	url := fmt.Sprintf("http://%suser/saved-routes?userID=%s", baseURL, userID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch saved routes: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error from server: %s\n", body)
	}

	var routes []common.UserSavedRoute
	if err = json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		return nil, fmt.Errorf("failed to decode saved routes: %s", err)
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("no saved routes found")
	}

	fmt.Println("Saved Routes:")
	for i, route := range routes {
		fmt.Printf("\n%d) %s -> %s [%s] (%d stops, ~%d min)\n",
			i+1, route.StartPoint, route.EndPoint, route.TransportMode, route.Stops, route.EstimatedTime)
	}

	uuids := make([]string, len(routes))
	for i, route := range routes {
		uuids[i] = route.RouteID
	}

	return uuids, nil
}

func AcceptSavedRoute(userID string, routeUUID string) error {

	url := fmt.Sprintf("http://%suser/saved-route?userID=%s&routeID=%s", baseURL, userID, routeUUID)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch route: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to fetch route: status %d, body: %s", resp.StatusCode, string(body))
	}

	var route common.UserSavedRoute
	if err := json.NewDecoder(resp.Body).Decode(&route); err != nil {
		return fmt.Errorf("failed to decode saved route: %w", err)
	}

	acceptURL := fmt.Sprintf("http://%suser/accept-saved-route", baseURL)
	jsonData, _ := json.Marshal(route)
	res, err := http.Post(acceptURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to accept saved route: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to accept saved route: status %d, body: %s", res.StatusCode, string(body))
	}

	fmt.Println("Route accepted and activated successfully.")
	return nil
}

func SaveToFavorites(userID string) error {
	url := fmt.Sprintf("http://%suser/save-favorite", baseURL)

	payload := map[string]string{"userID": userID}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to save to favorites: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error from API: %s", bodyBytes)
	}

	fmt.Println("Favorite route saved successfully.")
	return nil
}

func RecalculateRoute(userID string) error {
	request := common.NewRequest{
		UserID: userID,
		Reason: "user_triggered",
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to encode request: %s", err)
	}

	url := fmt.Sprintf("http://%sroute/recalculate-route", baseURL)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("recalculation failed [%d]: %s", resp.StatusCode, string(body))
	}

	var result common.RouteResult
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %s", err)
	}

	fmt.Println("🚀 New Route Recalculated:")
	fmt.Printf("🧭 From: %s\n", result.From)
	fmt.Printf("🎯 To:   %s\n", result.To)
	fmt.Printf("📈 Score: %d\n", result.Score)
	fmt.Println("📝 Summary:")
	fmt.Println("------------------------------------")
	fmt.Println("   " + result.Summary)
	fmt.Println("------------------------------------")
	return nil
}

func TerminateRouteFromMenu(userID string) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter reason for terminating the route: ")
	reason, _ := reader.ReadString('\n')
	reason = strings.TrimSpace(reason)

	req := common.NewRequest{
		UserID: userID,
		Reason: reason,
	}

	url := fmt.Sprintf("http://%sroute/terminate", baseURL)
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to call API Gateway: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s", string(body))
	}

	var reply common.SavedResp
	if err = json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return fmt.Errorf("failed to decode API response: %w", err)
	}

	if reply.Status != common.StatusDone {
		return fmt.Errorf("termination failed: unexpected status")
	}

	return nil
}

func StartNewRoute(userID string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter start location: ")
	startPoint, _ := reader.ReadString('\n')
	startPoint = strings.TrimSpace(startPoint)

	fmt.Print("Enter destination: ")
	endPoint, _ := reader.ReadString('\n')
	endPoint = strings.TrimSpace(endPoint)

	departure := time.Now()

	req := common.UserRequest{
		UserID:     userID,
		StartPoint: startPoint,
		EndPoint:   endPoint,
		Departure:  departure,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("http://%sroute/request", baseURL)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to contact API Gateway: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("route request failed: %s", string(body))
	}

	var result common.RouteResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return fmt.Errorf("failed to decode response: %s", err)
	}

	fmt.Printf("New Route: %s ➜ %s\n", result.From, result.To)
	fmt.Printf("Summary: %s (Score: %f)\n", result.Summary, result.Score)
	return nil
}
