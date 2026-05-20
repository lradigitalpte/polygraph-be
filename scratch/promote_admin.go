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
	godotenv.Load()
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run scratch/promote_admin.go <email>")
	}
	email := os.Args[1]

	db, err := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	var adminRole rbac.Role
	if err := db.Where("name = ?", "Admin").First(&adminRole).Error; err != nil {
		log.Fatal("Admin role not found:", err)
	}

	var user auth.User
	err = db.Where("email = ?", email).First(&user).Error
	if err != nil {
		// Create user record (first login auto-creates it, but run this after signing up)
		user = auth.User{Email: email, Name: email, RoleID: adminRole.ID}
		db.Create(&user)
		fmt.Printf("✅ Created Admin user: %s\n", email)
	} else {
		db.Model(&user).Update("role_id", adminRole.ID)
		fmt.Printf("✅ Promoted to Admin: %s\n", email)
	}

	// Also set role in neon_auth.user
	db.Exec(`UPDATE neon_auth."user" SET role = 'admin', "emailVerified" = true WHERE email = ?`, email)
	fmt.Println("✅ Neon Auth role set to admin")
}
