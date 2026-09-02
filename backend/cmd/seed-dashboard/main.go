package main

import (
	"fmt"
	"os"
	"path/filepath"

	"zyrouter/backend/internal/db"
)

func main() {
	seedPath := getenvOr("SEED_SQL", filepath.Join("..", "tests", "dashboard_fixture.sql"))
	dbPath := getenvOr("DB_PATH", filepath.Join("..", "tests", "dashboard_fixture.sqlite"))

	sqlBytes, err := os.ReadFile(seedPath)
	if err != nil {
		fail("read seed SQL", err)
	}

	database, err := db.OpenDatabase(dbPath)
	if err != nil {
		fail("open database", err)
	}
	defer database.Close()

	if _, err := database.Exec(string(sqlBytes)); err != nil {
		fail("execute seed SQL", err)
	}

	fmt.Printf("dashboard fixture seeded into %s\n", dbPath)
}

func getenvOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func fail(operation string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", operation, err)
	os.Exit(1)
}
