# Firebase Auth + PostgreSQL Implementation Guide

## Architecture Overview

This guide outlines how to implement Firebase Authentication with PostgreSQL storage for application-specific user data.

### High-Level Flow

1. **Firebase Auth** handles authentication (sign in, sign up, token generation)
2. **PostgreSQL** stores application-specific user data (profiles, preferences, roles, etc.)
3. **Firebase ID Token** is validated on each protected request
4. **User data** is fetched from PostgreSQL after token validation

## Architecture

```
┌─────────────┐
│   Client    │
│  (Frontend) │
└──────┬──────┘
       │ 1. Sign in with Firebase
       ▼
┌─────────────────┐
│  Firebase Auth  │
│  (Google Cloud) │
└──────┬──────────┘
       │ 2. Returns ID Token
       ▼
┌─────────────────┐     3. Validate Token      ┌──────────────┐
│  Go Backend API │◄───────────────────────────│  Firebase    │
│                 │                             │  Admin SDK   │
│  - Middleware   │                             └──────────────┘
│  - Handlers     │
│  - Services     │     4. Query User Data     ┌──────────────┐
└────────┬────────┘───────────────────────────►│  PostgreSQL  │
         │                                     │  (users,     │
         │                                     │  profiles)   │
         │ 5. Return Response                  └──────────────┘
         ▼
┌─────────────┐
│   Client    │
└─────────────┘
```

## Implementation Steps

### Step 1: Install Dependencies

```bash
go get firebase.google.com/go/v4
go get github.com/lib/pq  # PostgreSQL driver
go get github.com/jmoiron/sqlx  # SQL toolkit (optional but recommended)
```

### Step 2: Database Schema Design

Create a migration file: `migrations/0001_auth_users.sql`

```sql
-- Users table stores application-specific user data
-- Firebase UID is the primary key (from Firebase Auth)
CREATE TABLE users (
    firebase_uid VARCHAR(128) PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    display_name VARCHAR(255),
    photo_url TEXT,
    
    -- Application-specific fields
    role VARCHAR(50) DEFAULT 'user',
    organization VARCHAR(255),
    preferences JSONB DEFAULT '{}',
    
    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_login_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for common queries
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_organization ON users(organization);

-- Update timestamp trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

### Step 3: Configuration Setup

Add Firebase config to `config/config.go`:

```go
type FirebaseConfig struct {
    CredentialsPath string  // Path to Firebase service account JSON
    ProjectID       string  // Firebase project ID (optional, can be in credentials)
}

type Config struct {
    // ... existing fields
    Firebase FirebaseConfig
}
```

### Step 4: Module Structure

Following the project's standard structure:

```
internal/auth/
├── domain/
│   └── model.go          # User domain model
├── repository/
│   └── user_repo.go      # PostgreSQL repository
├── service/
│   └── auth_service.go   # Business logic
├── middleware/
│   └── firebase_auth.go  # Token validation middleware
└── http/
    ├── types.go          # Handler struct
    ├── router.go         # Route registration
    ├── handlers_auth.go  # Auth endpoints (sync user, etc.)
    └── handlers_user.go  # User profile endpoints
```

### Step 5: Key Components

#### 5.1 Domain Model (`internal/auth/domain/model.go`)

```go
package domain

import "time"

type User struct {
    FirebaseUID  string    `json:"firebase_uid" db:"firebase_uid"`
    Email        string    `json:"email" db:"email"`
    DisplayName  *string   `json:"display_name,omitempty" db:"display_name"`
    PhotoURL     *string   `json:"photo_url,omitempty" db:"photo_url"`
    Role         string    `json:"role" db:"role"`
    Organization *string   `json:"organization,omitempty" db:"organization"`
    Preferences  map[string]interface{} `json:"preferences,omitempty" db:"preferences"`
    CreatedAt    time.Time `json:"created_at" db:"created_at"`
    UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
    LastLoginAt  *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
}
```

#### 5.2 Repository (`internal/auth/repository/user_repo.go`)

```go
package repository

import (
    "database/sql"
    "time"
    "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/domain"
)

type UserRepository struct {
    db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
    return &UserRepository{db: db}
}

func (r *UserRepository) GetByFirebaseUID(uid string) (*domain.User, error) {
    // Implementation
}

func (r *UserRepository) Create(user *domain.User) error {
    // Implementation
}

func (r *UserRepository) Update(user *domain.User) error {
    // Implementation
}

func (r *UserRepository) UpdateLastLogin(uid string) error {
    // Implementation
}
```

#### 5.3 Firebase Middleware (`internal/auth/middleware/firebase_auth.go`)

```go
package middleware

import (
    "context"
    "firebase.google.com/go/v4/auth"
    "github.com/gin-gonic/gin"
)

func FirebaseAuthMiddleware(authClient *auth.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        token := extractToken(c)
        if token == "" {
            c.JSON(401, gin.H{"error": "missing authorization token"})
            c.Abort()
            return
        }

        decodedToken, err := authClient.VerifyIDToken(context.Background(), token)
        if err != nil {
            c.JSON(401, gin.H{"error": "invalid token"})
            c.Abort()
            return
        }

        // Store user info in context
        c.Set("firebase_uid", decodedToken.UID)
        c.Set("email", decodedToken.Claims["email"])
        c.Next()
    }
}

func extractToken(c *gin.Context) string {
    // Check Authorization header: "Bearer <token>"
    bearerToken := c.GetHeader("Authorization")
    if len(bearerToken) > 7 && bearerToken[:7] == "Bearer " {
        return bearerToken[7:]
    }
    return ""
}
```

#### 5.4 HTTP Handlers (`internal/auth/http/handlers_user.go`)

```go
package http

func (h *Handler) GetProfile(c *gin.Context) {
    firebaseUID := c.GetString("firebase_uid")
    user, err := h.userService.GetByFirebaseUID(firebaseUID)
    // Handle response
}

func (h *Handler) SyncUser(c *gin.Context) {
    // Sync Firebase user data to PostgreSQL
    // Called when user first signs in or profile updates
}
```

## Integration Points

### 1. Initialize Firebase Admin SDK

In `cmd/api/main.go` or a separate initialization file:

```go
import (
    "firebase.google.com/go/v4"
    "firebase.google.com/go/v4/auth"
    "google.golang.org/api/option"
)

func initializeFirebase(cfg *config.FirebaseConfig) (*auth.Client, error) {
    opt := option.WithCredentialsFile(cfg.CredentialsPath)
    app, err := firebase.NewApp(context.Background(), nil, opt)
    if err != nil {
        return nil, err
    }
    
    authClient, err := app.Auth(context.Background())
    if err != nil {
        return nil, err
    }
    
    return authClient, nil
}
```

### 2. Initialize Database Connection

```go
import (
    "database/sql"
    _ "github.com/lib/pq"
)

func initializeDB(cfg *config.DatabaseConfig) (*sql.DB, error) {
    dsn := fmt.Sprintf(
        "host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
        cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name,
    )
    return sql.Open("postgres", dsn)
}
```

### 3. Register Auth Routes

In `cmd/api/main.go`:

```go
// Initialize Firebase Auth
authClient, err := initializeFirebase(&cfg.Firebase)
if err != nil {
    log.Fatalf("Failed to initialize Firebase: %v", err)
}

// Initialize Database
db, err := initializeDB(&cfg.Database)
if err != nil {
    log.Fatalf("Failed to connect to database: %v", err)
}
defer db.Close()

// Initialize Auth module
userRepo := authrepo.NewUserRepository(db)
authService := authservice.New(userRepo)
authHandler := authhttp.New(authService)

// Register routes
api := router.Group("/api/v1")
authGroup := api.Group("/auth")
authGroup.Use(authmiddleware.FirebaseAuthMiddleware(authClient))
authHandler.Register(authGroup)
```

## Security Considerations

1. **Token Validation**: Always validate Firebase ID tokens on the backend
2. **HTTPS Only**: Use HTTPS in production
3. **CORS**: Configure CORS appropriately
4. **Rate Limiting**: Implement rate limiting for auth endpoints
5. **SQL Injection**: Use parameterized queries (always)
6. **Secrets**: Store Firebase credentials securely (env vars, secrets manager)

## Environment Variables

Add to `.env`:

```env
# Firebase
FIREBASE_CREDENTIALS_PATH=./firebase-service-account.json
FIREBASE_PROJECT_ID=your-project-id

# Database (already exists)
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your-password
DB_NAME=gosim
```

## Testing Strategy

1. **Unit Tests**: Repository and service layers
2. **Integration Tests**: End-to-end auth flows
3. **Mock Firebase**: Use Firebase emulator for testing
4. **Test Database**: Use separate test database

## Next Steps

1. Install dependencies
2. Create database migration
3. Set up Firebase project and download service account JSON
4. Implement repository layer
5. Implement service layer
6. Implement middleware
7. Implement HTTP handlers
8. Integrate into main application
9. Write tests
10. Deploy and test

