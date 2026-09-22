package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TransportMode   string
	Address         string
	DatabaseURL     string
	FrontendOrigins []string
	PublicBaseURL   string
	FileStorageRoot string
	DownloadTTL     time.Duration
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required; configure the environment or .env")
	}
	return c.validateTransport()
}

func Load() Config {
	mode := envOrDefault("TRANSPORT_MODE", "development")
	originsFallback := "localhost:3000,127.0.0.1:3000"
	if mode == "production" {
		originsFallback = ""
	}
	return Config{
		TransportMode:   mode,
		Address:         envOrDefault("SERVER_ADDRESS", ":8081"),
		DatabaseURL:     strings.TrimSpace(os.Getenv("DATABASE_URL")),
		FrontendOrigins: splitCSV(envOrDefault("FRONTEND_ORIGINS", originsFallback)),
		PublicBaseURL:   envOrDefault("PUBLIC_BASE_URL", "http://localhost:8081"),
		FileStorageRoot: envOrDefault("FILE_STORAGE_ROOT", "./storage"),
		DownloadTTL:     time.Duration(envInt("DOWNLOAD_URL_TTL_SECONDS", 600)) * time.Second,
	}
}
func envInt(name string, fallback int) int {
	n, e := strconv.Atoi(os.Getenv(name))
	if e != nil || n <= 0 {
		return fallback
	}
	return n
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
