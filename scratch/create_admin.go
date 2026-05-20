package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"my-app/internal/modules/auth"
	"my-app/internal/modules/rbac"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dsn := os.Getenv("DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// 1. Ensure Admin role exists
	var adminRole rbac.Role
	err = db.Where("name = ?", "Admin").First(&adminRole).Error
	if err != nil {
		log.Fatal("Admin role not found. Please run the seeder first.")
	}

	// 2. Ask for email or use a default one
	email := os.Args[1]
	if email == "" {
		log.Fatal("Please provide an email address as an argument")
	}

	// 3. Create or update user to Admin
	var user auth.User
	err = db.Where("email = ?", email).First(&user).Error
	if err != nil {
		// Create new
		user = auth.User{
			Email:  email,
			Name:   "Admin User",
			RoleID: adminRole.ID,
		}
		if err := db.Create(&user).Error; err != nil {
			log.Fatal("Failed to create admin user:", err)
		}
		fmt.Printf("Created new Admin user: %s\n", email)
	} else {
		// Update existing
		user.RoleID = adminRole.ID
		if err := db.Save(&user).Error; err != nil {
			log.Fatal("Failed to update user to admin:", err)
		}
		fmt.Printf("Updated existing user to Admin: %s\n", email)
	}
}
