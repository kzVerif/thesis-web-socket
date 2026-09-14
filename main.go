package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"ws-rat/internal/config"
	"ws-rat/internal/database"
	"ws-rat/internal/distribution"
	"ws-rat/internal/repository"
	"ws-rat/internal/virusscan"
	"ws-rat/internal/wsserver"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: could not load .env: %v", err)
	}

	cfg := config.Load()
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	server := wsserver.New(repository.NewAgentRepository(db), log.Default(), cfg.FrontendOrigins)
	server.ConfigureAudit(repository.NewAgentRepository(db))
	server.ConfigureDistribution(&distribution.Repository{DB: db}, cfg.PublicBaseURL, cfg.FileStorageRoot, cfg.DownloadTTL)
	server.ConfigureVirusScan(&virusscan.Repository{DB: db})
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWebSocket)
	mux.HandleFunc("/ws/frontend", server.HandleFrontendWebSocket)
	mux.HandleFunc("/files/download/", server.HandleFileDownload)

	log.Printf("WebSocket server running on %s", cfg.Address)
	if err := http.ListenAndServe(cfg.Address, mux); err != nil {
		log.Fatal(err)
	}
}
