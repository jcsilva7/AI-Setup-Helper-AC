package main

import (
	"ai-setup-helper-backend/internal"
	"log"
	"net/http"
	"os"
	"time"
)

var SetupCache *internal.Cache

func getSetupRequest(res http.ResponseWriter, req *http.Request) {
	// TODO
}

func main() {
	// Start HTTP server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/setup", getSetupRequest)

	Server := http.Server{
		Addr: ":" + port,
	}

	// Create internal (before server start)
	SetupCache = internal.NewCache(24*time.Hour, 1*time.Hour)

	err := Server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Listening on port " + port)
}
