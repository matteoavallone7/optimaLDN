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

func listenToNotifications(userID string, done <-chan struct{}, notificationChan chan<- string) {
	var conn *websocket.Conn
	var err error
	wsURL := fmt.Sprintf("ws://%sws?userID=%s", baseURL, userID)
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

	stopReader := make(chan struct{})

	go func() {
		for {
			_, msg, err2 := conn.ReadMessage()
			if err2 != nil {
				select {
				case <-stopReader:
					return
				default:
					log.Printf("WebSocket read error for user %s: %v", userID, err2)
					return
				}
			}
			notificationChan <- string(msg)
		}
	}()

	<-done
	close(stopReader)
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
	notificationChan := make(chan string, 10)
	done := make(chan struct{})
	go listenToNotifications(userID, done, notificationChan)
	defer close(done)

	if readInput("🗂 Save to favorites? (y/n):") == "y" {
		err := SaveToFavorites(userID)
		if err != nil {
			return fmt.Errorf("could not save to favorites: %v", err)
		}
		fmt.Println("✅ Route saved to favorites. Now manage your journey.")
	}

	go func() {
		for note := range notificationChan {
			fmt.Printf("\n📬 Notification: %s\n", note)

			if strings.Contains(note, "Sudden service worsening") || strings.Contains(note, "Recalculate?") {
				answer := readInput("💡 Sudden delay detected. Recalculate route? (y/n): ")
				if answer == "y" {
					if err := RecalculateRoute(userID); err != nil {
						fmt.Println("❌ Failed to recalculate route:", err)
					} else {
						fmt.Println("✅ Route recalculated successfully.")
					}
				}
			} else if strings.Contains(note, "Critical delay") {
				fmt.Println("⚠️ Your route may be affected by a critical delay.")
			}
		}
	}()

	for {
		choice := readInput("🛑 Terminate journey? (y/n): ")
		if choice == "y" {
			if err := TerminateRouteFromMenu(userID); err != nil {
				return fmt.Errorf("could not terminate route: %w", err)
			}
			break
		}
		fmt.Println("Journey still active. Please terminate before exiting.")
	}

	return nil

}

func savedUserMenu(userID string, uuids []string) error {
	var selectedUUID string
	for {
		fmt.Println("\n\nWhere are we going next?")
		fmt.Println("1. 🛫 Embark on one of your saved routes")
		fmt.Println("2. Go back")
		choice := readInput("> ")
		switch choice {
		case "1":
			var chosen int
			fmt.Print("\nEnter the number of the route to embark: ")
			_, err := fmt.Scanf("%d", &chosen)
			if err != nil || chosen < 1 || chosen > len(uuids) {
				fmt.Println("❌ Invalid choice")
				continue
			}
			selectedUUID = uuids[chosen-1]

			if err2 := AcceptSavedRoute(userID, selectedUUID); err2 != nil {
				fmt.Println("❌ Error:", err2)
				continue
			}
			if err3 := sharedLogic(userID); err3 != nil {
				fmt.Println("❌ Error:", err3)
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
		fmt.Printf("\n\nGood to see you again, '%s'!\n", userID)
		fmt.Println("\nWhat do you want to do today?")
		fmt.Println("1. 🗂 Check favorite routes")
		fmt.Println("2. 🛫 Start new route")
		fmt.Println("3. Go back")
		choice := readInput("Select an option: ")
		switch choice {
		case "1":
			uuids, err := ViewFavoriteRoutes(userID)
			if err != nil {
				fmt.Println("❌ Error:", err)
				continue
			}
			err = savedUserMenu(userID, uuids)
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

				err = loggedUserMenu(user)
				if err != nil {
					fmt.Println("❌ Error:", err)
				}

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
	baseURL = "ec2-3-89-249-117.compute-1.amazonaws.com:8080/"
	setupInterruptHandler()

	mainMenu(reader)

}
