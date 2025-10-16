package main

import (
	"log"
	"net/http"
	"os"

	httpamg "github.com/GoSim-25-26J-441/go-sim-backend/internal/api/http/amg_apd"
)

func main() {
	mux := http.NewServeMux()
	httpamg.RegisterRoutes(mux)

	port := os.Getenv("PORT")
	if port == "" { port = "8080" }

	log.Printf("AMG&APD API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
