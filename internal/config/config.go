package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address         string
	DatabaseURL     string
	FrontendOrigins []string
	PublicBaseURL   string
	FileStorageRoot string
	DownloadTTL     time.Duration
}

func Load() Config {
	return Config{
		Address:         envOrDefault("SERVER_ADDRESS", ":8081"),
		DatabaseURL:     envOrDefault("DATABASE_URL", "user=postgres password=kanghunz12 dbname=ratsystem sslmode=disable"),
		FrontendOrigins: splitCSV(envOrDefault("FRONTEND_ORIGINS", "localhost:3000,127.0.0.1:3000")),
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
