package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// Check all schemas
	var schemas []string
	db.Raw("SELECT schema_name FROM information_schema.schemata").Scan(&schemas)
	fmt.Println("Schemas:")
	for _, s := range schemas {
		fmt.Println("-", s)
	}

	// Check all tables across all schemas
	fmt.Println("\nAll tables:")
	rows, _ := db.Raw("SELECT table_schema, table_name FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema') ORDER BY table_schema, table_name").Rows()
	defer rows.Close()
	for rows.Next() {
		var schema, table string
		rows.Scan(&schema, &table)
		fmt.Printf("  %s.%s\n", schema, table)
	}
}
