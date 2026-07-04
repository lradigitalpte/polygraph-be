//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
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

	email := "admin@gmail.com"
	newPassword := "12345678"

	// 1. Show user details
	fmt.Println("=== neon_auth.user ===")
	rows, _ := db.Raw(`SELECT * FROM neon_auth."user" WHERE email = ?`, email).Rows()
	cols, _ := rows.Columns()
	fmt.Println("Columns:", cols)
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		for i, c := range cols {
			fmt.Printf("  %-25s = %v\n", c, vals[i])
		}
	}
	rows.Close()

	// 2. Get user ID
	var userID string
	db.Raw(`SELECT id FROM neon_auth."user" WHERE email = ?`, email).Scan(&userID)

	// 3. Mark email as verified
	db.Exec(`UPDATE neon_auth."user" SET "emailVerified" = true, "updatedAt" = NOW() WHERE email = ?`, email)
	fmt.Println("\n✅ emailVerified set to true")

	// 4. Generate $2b$ hash (Node.js compatible)
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), 10)
	if err != nil {
		log.Fatal(err)
	}
	// Replace $2a$ with $2b$ for Node.js compatibility
	hashStr := strings.Replace(string(hashed), "$2a$", "$2b$", 1)
	fmt.Println("Hash (2b$):", hashStr)

	// 5. Update password + updatedAt
	result := db.Exec(`UPDATE neon_auth.account SET password = ?, "updatedAt" = NOW() WHERE "userId" = ? AND "providerId" = 'credential'`,
		hashStr, userID)
	if result.Error != nil {
		log.Fatal("Failed:", result.Error)
	}
	fmt.Printf("✅ Password updated (%d rows affected)\n", result.RowsAffected)
	fmt.Printf("\nYou can now sign in with:\n  Email: %s\n  Password: %s\n", email, newPassword)
}
