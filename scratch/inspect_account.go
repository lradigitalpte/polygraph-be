//go:build ignore

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

	// Show account table columns
	fmt.Println("=== neon_auth.account columns ===")
	rows, _ := db.Raw(`
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'neon_auth' AND table_name = 'account'
		ORDER BY ordinal_position
	`).Rows()
	defer rows.Close()
	for rows.Next() {
		var col, dtype, nullable string
		rows.Scan(&col, &dtype, &nullable)
		fmt.Printf("  %-30s %-20s %s\n", col, dtype, nullable)
	}

	// Show actual account row for admin@gmail.com
	fmt.Println("\n=== Account row for admin@gmail.com ===")
	var userID string
	db.Raw(`SELECT id FROM neon_auth."user" WHERE email = 'admin@gmail.com'`).Scan(&userID)
	fmt.Println("User ID:", userID)

	rows2, _ := db.Raw(`SELECT * FROM neon_auth.account WHERE "userId" = ?`, userID).Rows()
	defer rows2.Close()
	cols, _ := rows2.Columns()
	fmt.Println("Columns:", cols)
	for rows2.Next() {
		vals := make([]interface{}, len(cols))
		valPtrs := make([]interface{}, len(cols))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}
		rows2.Scan(valPtrs...)
		for i, c := range cols {
			v := vals[i]
			if v == nil {
				fmt.Printf("  %-30s = <nil>\n", c)
			} else {
				s := fmt.Sprintf("%v", v)
				if len(s) > 80 {
					s = s[:80] + "..."
				}
				fmt.Printf("  %-30s = %s\n", c, s)
			}
		}
	}
}
