package main

import (
	"bufio"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"
)

var baseURL string

func listenToNotifications(userID string, done <-chan struct{}) {
	var conn *websocket.Conn
	var err error
	wsURL := fmt.Sprintf("ws://localhost:8080/ws?userID=%s", userID)
	for retries := 0; retries < 3; retries++ {
		conn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
		if err == nil {
			break
		}
		log.Printf("Retrying WebSocket connection... (%d)", retries+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		// Handle the final, unrecoverable error here.
		log.Fatalf("Failed to connect to WebSocket after 3 retries: %v", err)
	}
	defer conn.Close()

	msgChan := make(chan []byte)

	go func() {
		for {
			_, msg, err2 := conn.ReadMessage()
			if err2 != nil {
				log.Printf("WebSocket read error for user %s: %v", userID, err2)
				return
			}
			msgChan <- msg
		}
	}()

	for {
		select {
		case msg := <-msgChan:
			fmt.Println("📬 Notification:", string(msg))

			if strings.Contains(string(msg), "Recalculate?") {
				answer := readInput("Recalculate route? (y/n): ")
				if answer == "y" {
					err3 := RecalculateRoute(userID)
					if err3 != nil {
						log.Printf("Error recalculating route for user %s: %v", userID, err3)
						continue
					}
				}
			}
		case <-done:
			log.Println("Stopping notification listener.")
			return
		}
	}
}

func readInput(prompt string) string {
	fmt.Print(prompt + " ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

func setupInterruptHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		fmt.Println("\n\nExiting OptimaLDN. Goodbye!")
		os.Exit(0)
	}()
}

func sharedLogic(userID string) error {
	if readInput("🗂 Save to favorites? (y/n):") == "y" {
		err := SaveToFavorites(userID)
		if err != nil {
			return fmt.Errorf("could not save to favorites: %v", err)
		}
	}
	if readInput("🛑 Terminate journey? (y/n):") == "y" {
		err := TerminateRouteFromMenu(userID)
		if err != nil {
			return fmt.Errorf("could not terminate route: %w", err)
		}
	}

	return nil
}

func savedUserMenu(userID string) error {
	for {
		fmt.Println("\n\nWhere are we going next?")
		fmt.Println("1. 🛫 Embark on one of your saved routes")
		fmt.Println("2. Go back")
		choice := readInput("> ")
		switch choice {
		case "1":
			if err := AcceptSavedRoute(userID); err != nil {
				fmt.Println("❌ Error:", err)
				continue
			}
			if err := sharedLogic(userID); err != nil {
				fmt.Println("❌ Error:", err)
				continue
			}
		case "2":
			return nil
		default:
			fmt.Println("Invalid input. Try again.")
		}
	}
}

func loggedUserMenu(userID string) error {
	for {
		fmt.Printf("Good to see you again, '%s'!\n", userID)
		fmt.Println("What do you want to do today?")
		fmt.Println("1. 🗂 Check favorite routes")
		fmt.Println("2. 🛫 Start new route")
		fmt.Println("3. Go back")
		choice := readInput("Select an option: ")
		switch choice {
		case "1":
			err := ViewFavoriteRoutes(userID)
			if err != nil {
				fmt.Println("❌ Error:", err)
				continue
			}
			err = savedUserMenu(userID)
			if err != nil {
				fmt.Println("❌ Error:", err)
				continue
			}
		case "2":
			fmt.Println("Where are we heading to this time?")
			err := StartNewRoute(userID)
			if err != nil {
				fmt.Println("❌ Error:", err)
				continue
			}
			err = sharedLogic(userID)
			if err != nil {
				fmt.Println("❌ Error:", err)
				continue
			}
		case "3":
			fmt.Println("Logging out...")
			return nil
		default:
			fmt.Println("Invalid input. Try again.")
		}
	}

}

func mainMenu(reader *bufio.Reader) {
	fmt.Println("***Welcome to OptimaLDN***")
	for {
		fmt.Println("1. Login")
		fmt.Println("2. Exit")
		choice := readInput("Select an option:")
		switch choice {
		case "1":
			fmt.Print("Enter username: ")
			user, _ := reader.ReadString('\n')
			fmt.Print("Enter password: ")
			pass, _ := reader.ReadString('\n')

			user = strings.TrimSpace(user)
			pass = strings.TrimSpace(pass)

			_, err := Login(user, pass)
			if err != nil {
				fmt.Println("Login failed. Try again.")
			} else {

				// Start listening for notifications in a separate goroutine
				// and pass a channel to signal when the user logs out.
				done := make(chan struct{})
				go listenToNotifications(user, done)

				err = loggedUserMenu(user)
				if err != nil {
					fmt.Println("❌ Error:", err)
				}
				// When loggedUserMenu returns, signal the goroutine to stop.
				close(done)

			}
		case "2":
			fmt.Println("Goodbye!")
			return
		default:
			fmt.Println("Invalid input. Try again.")
		}
	}
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	baseURL = os.Getenv("API_GATEWAY_URL")
	setupInterruptHandler()

	mainMenu(reader)

}
