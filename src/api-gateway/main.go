package api_gateway

import (
	"log"
	"net/http"
	"os"
)

func main() {

	mux := http.NewServeMux()

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("API Gateway listening on port %s", port)

}
