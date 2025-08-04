package api_gateway

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"log"
	"net/http"
	"net/rpc"
	"os"
)

var userServiceClient *rpc.Client
var routePlannerClient *rpc.Client

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // allow all origins (or tighten in prod)
}

var clients = make(map[string]*websocket.Conn) // userID → connection

func wsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userID")
	if userID == "" {
		http.Error(w, "Missing userID", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	clients[userID] = conn
	log.Printf("User %s connected via WebSocket", userID)

	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				log.Printf("User %s disconnected", userID)
				conn.Close()
				delete(clients, userID)
				break
			}
		}
	}()
}

func SendToUser(userID, message string) error {
	conn, ok := clients[userID]
	if !ok {
		return fmt.Errorf("user %s not connected", userID)
	}
	return conn.WriteMessage(websocket.TextMessage, []byte(message))
}

func sendNotificationHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.FormValue("userID")
	msg := r.FormValue("msg")
	if err := SendToUser(userID, msg); err != nil {
		http.Error(w, "Failed to send", 500)
	}
}

// initClients establishes persistent connections to backend services.
func initClients() {
	var err error

	// Connect to User Service
	userServiceAddr := os.Getenv("USER_SERVICE_ADDR")
	if userServiceAddr == "" {
		userServiceAddr = "localhost:5001" // Fallback to a default address
	}
	userServiceClient, err = rpc.Dial("tcp", userServiceAddr)
	if err != nil {
		log.Fatalf("Failed to connect to User Service RPC at %s: %v", userServiceAddr, err)
	}
	log.Printf("Successfully connected to User Service RPC at %s", userServiceAddr)

	// Connect to Route Planner Service
	routePlannerAddr := os.Getenv("ROUTE_PLANNER_ADDR")
	if routePlannerAddr == "" {
		routePlannerAddr = "localhost:50051" // Fallback to a default address
	}
	routePlannerClient, err = rpc.Dial("tcp", routePlannerAddr)
	if err != nil {
		log.Fatalf("Failed to connect to Route Planner RPC at %s: %v", routePlannerAddr, err)
	}
	log.Printf("Successfully connected to Route Planner RPC at %s", routePlannerAddr)
}

func handleGetSavedRouteByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("userID")
	routeID := r.URL.Query().Get("routeID")

	if userID == "" || routeID == "" {
		http.Error(w, "Missing userID or routeID", http.StatusBadRequest)
		return
	}

	args := &common.RouteLookup{UserID: userID, RouteID: routeID}
	var reply common.UserSavedRoute

	if userServiceClient == nil {
		http.Error(w, "User service client not initialized", http.StatusInternalServerError)
		return
	}

	err := userServiceClient.Call("UserService.GetSavedRouteByID", args, &reply)
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC call failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

func handleAcceptSavedRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var savedRoute common.UserSavedRoute
	if err := json.NewDecoder(r.Body).Decode(&savedRoute); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if userServiceClient == nil {
		http.Error(w, "User service client not initialized", http.StatusInternalServerError)
		return
	}

	var rpcErr error
	err := userServiceClient.Call("UserService.CallAcceptSavedRoute", &savedRoute, &rpcErr)
	if err != nil || rpcErr != nil {
		http.Error(w, fmt.Sprintf("RPC call failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Saved route accepted successfully")
}

func handleUserSavedRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("userID")
	if userID == "" {
		http.Error(w, "Missing userID parameter", http.StatusBadRequest)
		return
	}

	if userServiceClient == nil {
		http.Error(w, "User service client not initialized", http.StatusInternalServerError)
		return
	}

	args := &common.NewRequest{UserID: userID}
	var reply []common.UserSavedRoute

	err := userServiceClient.Call("UserService.GetUserSavedRoutes", args, &reply)
	if err != nil {
		http.Error(w, "RPC call failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var authReq common.Auth
	if err := json.NewDecoder(r.Body).Decode(&authReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var reply common.SavedResp

	if userServiceClient == nil {
		http.Error(w, "User service client not initialized", http.StatusInternalServerError)
		return
	}

	err := userServiceClient.Call("UserService.AuthenticateUser", authReq, &reply)
	if err != nil {
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	// Respond to frontend
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

func handleSaveFavoriteRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req common.NewRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil || req.UserID == "" {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	if userServiceClient == nil {
		http.Error(w, "User service client not initialized", http.StatusInternalServerError)
		return
	}

	var rpcErr error
	err = userServiceClient.Call("UserService.SaveFavoriteRoute", &req, &rpcErr)
	if err != nil || rpcErr != nil {
		http.Error(w, "Failed to save route", http.StatusInternalServerError)
		log.Printf("RPC error: %v | inner: %v", err, rpcErr)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Favorite route saved successfully"}`))
}

func handleServeRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var userReq common.UserRequest
	err := json.NewDecoder(r.Body).Decode(&userReq)
	if err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if routePlannerClient == nil {
		http.Error(w, "Route planner service client not initialized", http.StatusInternalServerError)
		return
	}

	var result common.RouteResult
	err = routePlannerClient.Call("RoutePlanner.ServeRequest", &userReq, &result)
	if err != nil {
		http.Error(w, "RPC call failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleRecalculateRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req common.NewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if routePlannerClient == nil {
		http.Error(w, "Route planner service client not initialized", http.StatusInternalServerError)
		return
	}

	var result common.RouteResult
	err := routePlannerClient.Call("RoutePlanner.RecalculateRoute", &req, &result)
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleTerminateRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}

	var req common.NewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if routePlannerClient == nil {
		http.Error(w, "Route planner service client not initialized", http.StatusInternalServerError)
		return
	}

	var resp common.SavedResp
	err := routePlannerClient.Call("RoutePlanner.TerminateRoute", &req, &resp)
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC call failed: %v", err), http.StatusInternalServerError)
		return
	}

	if resp.Status != common.StatusDone {
		http.Error(w, "Route termination failed", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {

	initClients()

	defer userServiceClient.Close()
	defer routePlannerClient.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/user/saved-routes", handleUserSavedRoutes)
	mux.HandleFunc("/user/saved-route", handleGetSavedRouteByID)
	mux.HandleFunc("/user/accept-saved-route", handleAcceptSavedRoute)
	mux.HandleFunc("/user/save-favorite", handleSaveFavoriteRoute)
	mux.HandleFunc("/route/request", handleServeRequest)
	mux.HandleFunc("/route/recalculate", handleRecalculateRoute)
	mux.HandleFunc("/route/terminate", handleTerminateRoute)
	mux.HandleFunc("/ws", wsHandler)
	mux.HandleFunc("/send-notification", sendNotificationHandler)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("API Gateway listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
