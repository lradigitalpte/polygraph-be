// @title my-app API
// @version 1.0
// @description This is a sample server for my-app.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// @host localhost:8080
// @BasePath /api
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "my-app/docs"
	"my-app/internal/database"
	"my-app/internal/dbseed"
	"my-app/internal/middleware"
	"my-app/internal/models"
	"my-app/internal/modules/appointments"
	"my-app/internal/modules/auditlogs"
	"my-app/internal/modules/auth"
	"my-app/internal/modules/availability"
	"my-app/internal/modules/exams"
	"my-app/internal/modules/forms"
	"my-app/internal/modules/leads"
	"my-app/internal/modules/rbac"
	"my-app/internal/modules/settings"
	"my-app/internal/modules/subjects"
	"my-app/internal/modules/users"
	"my-app/internal/storage"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

var logger *zap.Logger

func initLogger() {
	var err error
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "production" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}
	if err != nil {
		panic(err)
	}
}

func main() {
	// Load environment variables
	godotenv.Load()

	// Initialize logger
	initLogger()
	defer logger.Sync()

	logger.Info("Starting my-app server")

	if os.Getenv("GIN_MODE") == "release" || os.Getenv("LOG_LEVEL") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize database
	db, err := database.InitDB()
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	logger.Info("Database connected successfully")

	// Run Migrations
	err = db.AutoMigrate(
		&auth.User{},
		&models.AuditLog{},
		&rbac.Permission{},
		&rbac.Role{},
		&rbac.UserPermission{},
		&subjects.Subject{},
		&appointments.Client{},
		&appointments.Appointment{},
		&appointments.ClientDocument{},
		&appointments.SubjectDocument{},
		&appointments.Quotation{},
		&availability.Block{},
		&exams.ExamType{},
		&exams.Exam{},
		&exams.ExamQuestion{},
		&exams.ExamReport{},
		&exams.Document{},
		&exams.CaseReferral{},
		&exams.ClinicalAssessment{},
		&exams.ExamPhase{},
		&leads.Lead{},
		&forms.FormTemplate{},
		&forms.FormRequest{},
		&settings.OrganizationSettings{},
	)
	if err != nil {
		logger.Warn("Database migration completed with warnings (some columns may already exist)", zap.Error(err))
	} else {
		logger.Info("Database migration successful")
	}

	// AutoMigrate does not remove obsolete indexes; drop legacy unique index on id_number if it exists.
	if db.Migrator().HasIndex(&subjects.Subject{}, "idx_subjects_id_number") {
		if dropErr := db.Migrator().DropIndex(&subjects.Subject{}, "idx_subjects_id_number"); dropErr != nil {
			logger.Warn("Failed to drop legacy subject id_number unique index", zap.Error(dropErr))
		} else {
			logger.Info("Dropped legacy subject id_number unique index")
		}
	}

	// Seed database with roles and permissions
	dbseed.SeedDatabase(db, logger)
	forms.SeedTemplates(db)

	// Initialize JWKS for token validation (optional — falls back to X-User-Email header if unavailable)
	if jwksURL := os.Getenv("AUTH_JWKS_URL"); jwksURL != "" {
		if err := middleware.InitJWKS(jwksURL); err != nil {
			logger.Warn("JWKS initialization failed — JWT validation disabled, falling back to session header auth", zap.Error(err))
		} else {
			logger.Info("JWKS initialized", zap.String("url", jwksURL))
		}
	} else {
		logger.Warn("AUTH_JWKS_URL not set — JWT validation disabled")
	}

	// Get host from environment
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	// Get HTTP port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := host + ":" + port

	logger.Info("Starting HTTP server", zap.String("address", addr))

	// Create Gin router
	r := gin.Default()

	// Security Headers
	r.Use(middleware.SecureHeaders())

	// Rate Limiting
	r.Use(middleware.RateLimiter())

	// Audit logging (global)
	r.Use(middleware.AuditMiddleware())

	allowedOrigins := middleware.AllowedOrigins()
	logger.Info("CORS allowed origins", zap.Strings("origins", allowedOrigins))
	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-User-Email"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "Server is running",
		})
	})

	// Swagger documentation
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Root endpoint
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Welcome to my-app API!",
		})
	})

	// Initialize Storage
	var fileStorage storage.Storage
	s3Storage, err := storage.NewS3Storage()
	if err != nil {
		logger.Warn("S3 Storage not initialized, using local uploads directory", zap.Error(err))
		fileStorage = storage.NewLocalStorage("uploads")
	} else {
		fileStorage = s3Storage
	}

	// Initialize Services & Controllers
	rbacService := rbac.NewService()
	rbacCtrl := rbac.NewController(rbacService)

	subjectService := subjects.NewService()
	subjectCtrl := subjects.NewController(subjectService)

	examService := exams.NewService(db, s3Storage)
	examCtrl := exams.NewController(examService)

	appService := appointments.NewService(fileStorage)
	appCtrl := appointments.NewController(appService)

	availabilityService := availability.NewService()
	availabilityCtrl := availability.NewController(availabilityService)

	leadService := leads.NewService()
	leadCtrl := leads.NewController(leadService)

	auditService := auditlogs.NewService()
	auditCtrl := auditlogs.NewController(auditService)

	userService := users.NewService()
	userCtrl := users.NewController(userService)

	settingsService := settings.NewService()
	settingsCtrl := settings.NewController(settingsService)

	formsService := forms.NewService()
	formsCtrl := forms.NewController(formsService)

	// Public API (no auth) — client form fill links
	publicAPI := r.Group("/api/public")
	forms.RegisterPublicRoutes(publicAPI, formsCtrl)

	// API Routes
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware()) // Protect all /api routes
	{
		// Register Modular Routes
		rbac.RegisterRoutes(api, rbacCtrl)
		users.RegisterRoutes(api, userCtrl, middleware.PermissionMiddleware)
		settings.RegisterRoutes(api, settingsCtrl, middleware.PermissionMiddleware)
		subjects.RegisterRoutes(api, subjectCtrl, middleware.PermissionMiddleware)
		exams.RegisterRoutes(api, examCtrl, middleware.PermissionMiddleware)
		appointments.RegisterRoutes(api, appCtrl, middleware.PermissionMiddleware)
		availability.RegisterRoutes(api, availabilityCtrl, middleware.PermissionMiddleware)
		leads.RegisterRoutes(api, leadCtrl, middleware.PermissionMiddleware)
		auditlogs.RegisterRoutes(api, auditCtrl, middleware.PermissionMiddleware)
		forms.RegisterRoutes(api, formsCtrl, middleware.PermissionMiddleware)
	}

	// Setup server
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// 5 second timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exiting")
}
