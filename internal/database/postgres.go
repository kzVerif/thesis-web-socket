package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func Open(connectionString string) (*sql.DB, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("database configuration is invalid; check DATABASE_URL")
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("database connection failed; check configuration and database availability")
	}
	return db, nil
}
