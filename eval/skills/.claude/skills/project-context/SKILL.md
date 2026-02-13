---
description: "Project context knowledge with directory structure, key APIs, and conventions"
when_to_use: "When navigating, editing, or understanding the project structure"
context: inline
---

# Project Context

You are working in a Go web application project. Use the following context to navigate and make changes effectively.

## Directory Structure

```
myapp/
├── cmd/
│   ├── server/main.go       # HTTP server entry point
│   └── migrate/main.go      # Database migration CLI
├── internal/
│   ├── api/
│   │   ├── handler.go        # HTTP handlers (one per resource)
│   │   ├── middleware.go      # Auth, logging, recovery middleware
│   │   └── router.go         # Route registration
│   ├── domain/
│   │   ├── user.go            # User entity + business logic
│   │   ├── project.go         # Project entity + business logic
│   │   └── errors.go          # Domain-specific error types
│   ├── storage/
│   │   ├── postgres/
│   │   │   ├── user_repo.go   # User repository (SQL)
│   │   │   └── project_repo.go
│   │   └── redis/
│   │       └── cache.go       # Cache layer
│   └── service/
│       ├── user_service.go    # Orchestrates domain + storage
│       └── project_service.go
├── pkg/
│   ├── config/config.go       # Env-based configuration
│   └── httputil/response.go   # JSON response helpers
├── migrations/                # SQL migration files (goose)
├── go.mod
└── go.sum
```

## Architecture Pattern

The project follows a layered architecture:

1. **Handler layer** (`internal/api/`): Parses HTTP requests, calls services, writes responses. No business logic here.
2. **Service layer** (`internal/service/`): Orchestrates business logic. Depends on domain types and repository interfaces.
3. **Domain layer** (`internal/domain/`): Pure business entities and rules. No external dependencies.
4. **Storage layer** (`internal/storage/`): Implements repository interfaces. Contains SQL queries and cache logic.

Dependencies flow inward: Handler -> Service -> Domain <- Storage.

## Key Conventions

- **Repository pattern**: Each entity has an interface in `internal/domain/` and an implementation in `internal/storage/`:
  ```go
  // internal/domain/user.go
  type UserRepository interface {
      GetByID(ctx context.Context, id string) (*User, error)
      Create(ctx context.Context, u *User) error
      List(ctx context.Context, opts ListOpts) ([]*User, error)
  }
  ```
- **Error handling**: Domain errors are defined in `internal/domain/errors.go`:
  ```go
  var (
      ErrNotFound      = errors.New("not found")
      ErrAlreadyExists = errors.New("already exists")
      ErrForbidden     = errors.New("forbidden")
  )
  ```
  Handlers map domain errors to HTTP status codes in a central `errorResponse()` helper.

- **Configuration**: All config is loaded from environment variables via `pkg/config/`:
  ```go
  type Config struct {
      Port        int    `env:"PORT" default:"8080"`
      DatabaseURL string `env:"DATABASE_URL" required:"true"`
      RedisURL    string `env:"REDIS_URL"`
  }
  ```

- **Testing**: Tests live next to the code they test (`user_service_test.go`). Use `testify/require` for assertions. Integration tests use `testcontainers-go` for Postgres.

- **Migrations**: Use `goose` for SQL migrations. Files are numbered: `001_create_users.sql`, `002_add_projects.sql`.

## API Patterns

- All endpoints return JSON via `httputil.JSON(w, statusCode, payload)`.
- List endpoints accept `?limit=` and `?offset=` query parameters.
- Create/Update endpoints parse JSON bodies via `json.NewDecoder(r.Body).Decode(&req)`.
- Auth middleware extracts user ID from JWT and stores it in context: `api.UserIDFromContext(ctx)`.
- Routes are registered in `internal/api/router.go`:
  ```go
  r.Route("/api/v1", func(r chi.Router) {
      r.Use(authMiddleware)
      r.Get("/users/{id}", h.GetUser)
      r.Post("/users", h.CreateUser)
      r.Get("/projects", h.ListProjects)
  })
  ```

## Common Tasks

- **Add a new entity**: Create domain type -> repository interface -> Postgres implementation -> service -> handler -> route
- **Add a new endpoint**: Handler function -> register route -> add tests
- **Add a migration**: `goose create add_column_x sql` in `migrations/`
- **Run tests**: `go test ./...` from project root
- **Run server**: `go run ./cmd/server`
