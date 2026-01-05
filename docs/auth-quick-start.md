# Auth Module Quick Start Checklist

## Prerequisites

- [ ] Firebase project created
- [ ] Firebase service account JSON downloaded
- [ ] PostgreSQL database running
- [ ] Go dependencies installed

## Step-by-Step Implementation

### 1. Install Dependencies
```bash
go get firebase.google.com/go/v4
go get github.com/lib/pq
go get github.com/jmoiron/sqlx  # Recommended for easier SQL handling
```

### 2. Create Database Migration
- [ ] Create `migrations/0001_auth_users.sql`
- [ ] Run migration: `psql -d gosim -f migrations/0001_auth_users.sql`

### 3. Update Configuration
- [ ] Add `FirebaseConfig` to `config/config.go`
- [ ] Add Firebase config loading in `config.Load()`
- [ ] Add environment variables

### 4. Create Module Structure
- [ ] Create `internal/auth/domain/model.go`
- [ ] Create `internal/auth/repository/user_repo.go`
- [ ] Create `internal/auth/service/auth_service.go`
- [ ] Create `internal/auth/middleware/firebase_auth.go`
- [ ] Create `internal/auth/http/` directory structure

### 5. Implement Core Components
- [ ] Domain model (User struct)
- [ ] Repository (GetByFirebaseUID, Create, Update)
- [ ] Service (business logic)
- [ ] Middleware (token validation)
- [ ] HTTP handlers (GetProfile, SyncUser, UpdateProfile)

### 6. Database Connection Helper
- [ ] Create `internal/storage/postgres/connection.go`
- [ ] Implement connection initialization
- [ ] Add connection pooling configuration

### 7. Firebase Initialization
- [ ] Create Firebase initialization function
- [ ] Add to `cmd/api/main.go`

### 8. Integrate into Main Application
- [ ] Initialize Firebase Auth client
- [ ] Initialize database connection
- [ ] Initialize auth module components
- [ ] Register routes with middleware

### 9. Test Endpoints
- [ ] Test token validation
- [ ] Test user creation/sync
- [ ] Test profile endpoints
- [ ] Test protected routes

### 10. Documentation
- [ ] Update README with auth setup instructions
- [ ] Document API endpoints
- [ ] Document environment variables

## Common Patterns to Follow

### Repository Pattern
- Use interfaces for testability
- Parameterized queries only (no string concatenation)
- Handle errors properly
- Use transactions for multi-step operations

### Service Layer
- Business logic goes here, not in handlers
- Services use repositories
- Return domain models, not DB models

### Middleware Pattern
- Extract token from Authorization header
- Validate with Firebase Admin SDK
- Store user info in Gin context
- Abort on validation failure

### Handler Pattern
- Extract data from context/request
- Call service methods
- Handle errors appropriately
- Return JSON responses

## File Structure Reference

```
internal/auth/
├── domain/
│   ├── model.go              # User domain model
│   └── errors.go             # Domain-specific errors
├── repository/
│   ├── user_repo.go          # PostgreSQL implementation
│   └── interfaces.go         # Repository interfaces
├── service/
│   └── auth_service.go       # Business logic
├── middleware/
│   └── firebase_auth.go      # Token validation middleware
└── http/
    ├── types.go              # Handler struct
    ├── router.go             # Route registration
    ├── handlers_user.go      # User profile handlers
    └── handlers_auth.go      # Auth sync handlers
```

