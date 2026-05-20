# my-app

This project was created with [Better Fullstack](https://github.com/Marve10s/Better-Fullstack), a high-performance Go stack.

## Features

- **Go** - Fast, reliable, and efficient programming language
- **Gin** - High-performance HTTP web framework
- **GORM** - Full-featured ORM for Go
- **Zap** - Blazing fast, structured logging
- **golang-jwt** - JWT token creation and validation
- **GoBetterAuth** - Embedded auth routes with email/password and session support

## Prerequisites

- [Go](https://go.dev/) 1.22 or higher

## Getting Started

First, copy the environment file:

```bash
cp .env.example .env
```

Then, install dependencies and run the server:

```bash
go mod tidy
go run cmd/server/main.go
```

The server will be running at [http://localhost:8080](http://localhost:8080).

## Database Setup

This project uses GORM with SQLite by default. To configure the database:

1. Copy the environment file:

```bash
cp .env.example .env
```

2. Update `DATABASE_URL` in `.env` with your database connection string.

Supported databases:

- SQLite (default): `DATABASE_URL=./data.db`
- PostgreSQL: `DATABASE_URL=postgres://user:pass@localhost:5432/dbname`

## Authentication Setup

This project mounts GoBetterAuth at `/api/auth` and ships with:

- email/password sign-in
- cookie-based sessions
- local SMTP defaults for development

Auth-specific environment variables in `.env`:

- `GO_BETTER_AUTH_BASE_URL=http://localhost:8080`
- `GO_BETTER_AUTH_SECRET=change-me-to-a-real-secret-at-least-32-chars`
- `GO_BETTER_AUTH_DATABASE_URL=./gobetterauth.db`
- `FROM_ADDRESS=noreply@localhost`
- `SMTP_HOST=localhost`
- `SMTP_PORT=1025`

The generated config disables verification emails on sign-up so the server can boot cleanly without extra setup. For password reset and email flows, point the SMTP settings at a local mail catcher or your real provider.

## Project Structure

```
my-app/
├── go.mod                # Module definition
├── cmd/
│   └── server/           # HTTP server entry point
│       └── main.go
├── internal/
│   ├── auth/             # GoBetterAuth bootstrap and config
│   │   └── auth.go
│   ├── database/         # Database configuration
│   │   └── database.go
│   ├── models/           # GORM models
│   │   └── models.go
│   └── handlers/         # HTTP handlers
│       └── handlers.go
├── .env.example          # Environment variables template
└── .gitignore
```

## Available Commands

- `go build ./...`: Build all packages
- `go run cmd/server/main.go`: Run the server
- `go test ./...`: Run all tests
- `go fmt ./...`: Format code
- `go vet ./...`: Run static analysis
