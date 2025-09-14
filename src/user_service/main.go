package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"github.com/matteoavallone7/optimaLDN/src/rabbitmq"
	"github.com/matteoavallone7/optimaLDN/src/user_service/logic"
	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var routePlannerClient *rpc.Client

type UserService struct{}

const (
	userOutboundNotifications        = "user_exchange"
	userType                         = "direct"
	notificationOutboundExchangeName = "notification_outbound_events_exchange"
	notificationsQueue               = "notifications_user_queue"
	bindingKey                       = "user.update.#"
)

var db *pgxpool.Pool

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func (u *UserService) AuthenticateUser(args *common.Auth, reply *common.SavedResp) error {
	var passwordHash string
	query := `SELECT password_hash FROM users WHERE username = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := db.QueryRow(ctx, query, args.UserID).Scan(&passwordHash)
	if err != nil {
		log.Printf("User '%s' not found or query error: %v", args.UserID, err)
		reply.Status = common.StatusError
		return nil
	}

	err = bcrypt.CompareHashAndPassword([]byte(strings.TrimSpace(passwordHash)), []byte(args.Password))
	if err != nil {
		log.Printf("Invalid password for user '%s'", args.UserID)
		reply.Status = common.StatusError
		return nil
	}

	reply.UserID = args.UserID
	reply.Status = common.StatusDone
	return nil
}

func saveRouteToPostgres(ctx context.Context, db *pgxpool.Pool, route common.UserSavedRoute) error {
	_, err := db.Exec(ctx, `
        INSERT INTO user_saved_routes 
        (route_id, user_id, start_point, end_point, transport_mode, 
         stops, estimated_time, line_names, stops_names)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (user_id, start_point, end_point, transport_mode) DO NOTHING
    `, route.RouteID, route.UserID, route.StartPoint, route.EndPoint,
		route.TransportMode, route.Stops, route.EstimatedTime,
		route.LineNames, route.StopsNames)

	if err != nil {
		return fmt.Errorf("failed to insert favorite route: %w", err)
	}
	return nil
}

func (u *UserService) GetUserSavedRoutes(args *common.NewRequest, reply *[]common.UserSavedRoute) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
        SELECT route_id, user_id, start_point, end_point, transport_mode, 
               stops, estimated_time, line_names, stops_names
        FROM  user_saved_routes
        WHERE user_id = $1`, args.UserID)
	if err != nil {
		return fmt.Errorf("failed to query saved routes: %w", err)
	}
	defer rows.Close()

	var routes []common.UserSavedRoute
	for rows.Next() {
		var route common.UserSavedRoute
		err = rows.Scan(
			&route.RouteID,
			&route.UserID,
			&route.StartPoint,
			&route.EndPoint,
			&route.TransportMode,
			&route.Stops,
			&route.EstimatedTime,
			&route.LineNames,
			&route.StopsNames,
		)
		if err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}
		routes = append(routes, route)
	}

	*reply = routes
	return nil
}

func (u *UserService) SaveFavoriteRoute(args *common.NewRequest, reply *common.SavedResp) error {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var route common.ChosenRoute
	err := routePlannerClient.Call("RoutePlanner.GetCurrentRoute", args, &route)
	if err != nil {
		return fmt.Errorf("rpc error getting active route: %w", err)
	}

	saved := logic.ConvertToUserSavedRoute(args.UserID, route)

	err = saveRouteToPostgres(ctx, db, saved)
	if err != nil {
		return fmt.Errorf("failed to save favorite route: %w", err)
	}

	reply.Status = common.StatusDone
	return nil
}

func (u *UserService) CallAcceptSavedRoute(savedRoute common.UserSavedRoute, reply *common.SavedResp) error {

	err := routePlannerClient.Call("RoutePlanner.AcceptSavedRouteRequest", &savedRoute, &reply)
	if err != nil {
		return fmt.Errorf("AcceptSavedRoute RPC call failed: %w", err)
	}

	reply.Status = common.StatusDone
	return nil
}

func (u *UserService) GetSavedRouteByID(req *common.RouteLookup, reply *common.UserSavedRoute) error {
	query := `SELECT * FROM user_saved_routes WHERE user_id=$1 AND route_id=$2`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := db.QueryRow(ctx, query, req.UserID, req.RouteID)

	var saved common.UserSavedRoute
	err := row.Scan(&saved.RouteID, &saved.UserID, &saved.StartPoint, &saved.EndPoint,
		&saved.TransportMode, &saved.Stops, &saved.EstimatedTime, &saved.LineNames, &saved.StopsNames)

	if err != nil {
		return fmt.Errorf("route not found: %w", err)
	}

	*reply = saved
	return nil
}

func main() {
	fmt.Println("Starting User Service...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	userService := new(UserService)
	server := rpc.NewServer()
	err := server.Register(userService)
	failOnError(err, "Failed to register User Service")
	log.Println("User Service successfully registered.")

	port := os.Getenv("US_PORT")
	if port == "" {
		log.Fatal("US_PORT is not set.")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", port))
	failOnError(err, "Failed to listen to TCP port")
	log.Printf("Listening on 0.0.0.0:%s", port)

	fmt.Println("Attempting to connect to PostgreSQL...")
	dsn := os.Getenv("DATABASE_URL")
	log.Println(dsn)
	db, err = pgxpool.New(ctx, dsn)
	failOnError(err, "Failed to connect to PostgreSQL")
	log.Println("Connected to PostgreSQL.")

	routePlannerAddr := os.Getenv("ROUTE_PLANNER_ADDR")
	routePlannerClient, err = rpc.Dial("tcp", routePlannerAddr)
	if err != nil {
		log.Fatalf("Failed to connect to Route Planner RPC at %s: %v", routePlannerAddr, err)
	}
	log.Printf("Successfully connected to Route Planner RPC at %s", routePlannerAddr)

	conn, ch, err := rabbitmq.InitRabbitMQ(userOutboundNotifications, userType)
	failOnError(err, "Failed to connect to RabbitMQ")

	defer func() {
		if cleanupErr := rabbitmq.CloseResources(ch, conn)(); cleanupErr != nil {
			log.Printf("Error during RabbitMQ resource cleanup: %v", cleanupErr)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	_, err = rabbitmq.DeclareAndBindQueue(ch, notificationsQueue, bindingKey, notificationOutboundExchangeName)
	failOnError(err, fmt.Sprintf("Failed to declare and bind queue '%s' for Notification Service", notificationsQueue))

	userHandler := func(delivery amqp.Delivery) bool {
		log.Printf("[User Service] Received Delay Event: %s (Key: %s)", string(delivery.Body), delivery.RoutingKey)
		var payload common.NewRequest
		err2 := json.Unmarshal(delivery.Body, &payload)
		if err2 != nil {
			log.Printf("[User Service] Failed to unmarshal New Request: %v", err2)
			return false
		}

		logic.NotifyUser(payload.UserID, "⚠️ Sudden service worsening on your route. Recalculate? (y/n)")

		return true
	}

	routeConsumer := rabbitmq.NewConsumer(ch, notificationsQueue, userHandler)
	wg.Add(1)
	go func() {
		defer wg.Done()
		routeConsumer.StartConsume(ctx)
	}()

	go func() {
		for {
			server.Accept(listener)
			if err != nil {
				log.Printf("RPC Accept error: %v", err)
				break
			}
		}
	}()

	<-sigChan
	log.Println("Shutdown signal received. Stopping consumers...")

	cancel()

	wg.Wait()
	log.Println("User service shut down cleanly.")

}
