package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

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
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	workingDir, _ := os.Getwd()
	storageRoot, _ := filepath.Abs(cfg.FileStorageRoot)
	log.Printf("distribution config mode=%s server_address=%s public_base_url=%s file_storage_root=%s file_storage_root_abs=%s download_ttl=%s frontend_origins=%v working_dir=%s",
		cfg.TransportMode, cfg.Address, cfg.PublicBaseURL, cfg.FileStorageRoot, storageRoot, cfg.DownloadTTL, cfg.FrontendOrigins, workingDir)
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	server := wsserver.New(repository.NewAgentRepository(db), log.Default(), cfg.FrontendOrigins)
	server.ConfigureProductionTransport(cfg.TransportMode == "production")
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
