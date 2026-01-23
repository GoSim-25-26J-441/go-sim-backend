# Branch Alignment Plan: features/uip/temp → Aligned with dev

## Objective
Create a new branch from `features/uip/temp`, merge `dev` branch changes, and align the code structure to match dev branch standards while preserving new features (projects, chats, diagrams).

---

## Current State Analysis

### Features/uip/temp Branch
**New Features:**
- ✅ Projects module (`internal/projects/`)
- ✅ Chats module (`internal/design_input_processing/chats/`)
- ✅ Diagrams module (`internal/design_input_processing/diagrams/`)
- ✅ Centralized route registration (`internal/api/http/routes/v1.go`)

**Issues:**
- ❌ Missing `realtime_system_simulation` module (deleted)
- ❌ Missing Redis connection
- ❌ Dual database connections (sql.DB for auth, pgxpool for new features)
- ❌ Different architectural pattern (flat structure vs layered)
- ❌ Route registration pattern inconsistent with dev

### Dev Branch
**Structure:**
- ✅ Layered architecture: `domain/`, `repository/`, `service/`, `http/`
- ✅ Single database connection pattern (`database/sql`)
- ✅ Redis connection for simulation
- ✅ Module-specific route registration (`Register()` method per module)
- ✅ `realtime_system_simulation` module fully implemented

---

## Alignment Plan

### Phase 1: Branch Setup
**Steps:**
1. Create new branch from `features/uip/temp`
   ```bash
   git checkout features/uip/temp
   git checkout -b features/uip/aligned-with-dev
   ```

2. Merge dev branch
   ```bash
   git merge dev
   # Resolve conflicts if any
   ```

3. Verify merge result
   - Check that simulation module is restored
   - Check that Redis connection is restored
   - Identify conflicts

---

### Phase 2: Remove Redundant Components

#### 2.1 Remove Redundant Database Connections
**Current State (temp):**
- `sql.DB` connection for auth (`authSQL`)
- `pgxpool.Pool` connection for projects/chats/diagrams (`dbPool`)

**Target State (aligned with dev):**
- Single `sql.DB` connection for all modules
- Convert pgxpool usage to `database/sql`

**Files to Modify:**
1. `cmd/api/main.go`
   - Remove `pgxpool` import and connection
   - Remove `pgxDSN()` function
   - Use single `postgres.NewConnection()` returning `*sql.DB`
   - Pass `*sql.DB` to all repositories

2. `internal/projects/repo.go`
   - Change `*pgxpool.Pool` to `*sql.DB`
   - Convert all `pgx` queries to `database/sql` queries
   - Update error handling (pgx.ErrNoRows → sql.ErrNoRows)

3. `internal/design_input_processing/chats/repo.go`
   - Same conversion as projects

4. `internal/design_input_processing/diagrams/repo.go`
   - Same conversion as projects

5. **Note:** `internal/users/repo.go` does NOT exist - user management is handled by `internal/auth/repository/user_repo.go`
   - Do NOT create a separate users module
   - Use `auth/repository/UserRepository` for all user-related operations

**Query Conversion Examples:**
```go
// FROM (pgxpool):
rows, err := r.db.Query(ctx, q, args...)
if errors.Is(err, pgx.ErrNoRows) { ... }

// TO (database/sql):
rows, err := r.db.QueryContext(ctx, q, args...)
if err == sql.ErrNoRows { ... }
```

#### 2.2 Remove Redundant API Route Registration
**Current State (temp):**
- Centralized registration in `internal/api/http/routes/v1.go`
- Projects, chats, diagrams registered through centralized routes

**Target State (aligned with dev):**
- Each module has its own `Register()` method
- Modules register themselves in `main.go`

**Files to Remove:**
- `internal/api/http/routes/v1.go` (if exists)
- `internal/design_input_processing/api/http/routes/projects.go` (if exists)

**Files to Create/Modify:**
1. `internal/projects/http/router.go` (or add to `http.go`)
   ```go
   func Register(rg *gin.RouterGroup, repo *Repo) {
       h := &Handler{repo: repo}
       rg.POST("", h.create)
       rg.GET("", h.list)
       // ...
   }
   ```

2. `internal/design_input_processing/chats/http/router.go` (or modify existing)
   - Rename `RegisterProjectRoutes` to `Register` if needed
   - Or keep as `RegisterProjectRoutes` but align pattern

3. `internal/design_input_processing/diagrams/http/router.go` (or modify existing)
   - Same as chats

4. `cmd/api/main.go`
   - Remove `apiroutes.RegisterV1()` call
   - Add individual module registrations:
     ```go
     projectsGroup := api.Group("/projects")
     projectRepo := projects.NewRepo(db)
     projects.Register(projectsGroup, projectRepo)
     ```

---

### Phase 3: Restructure Modules to Match Dev Pattern

#### 3.1 Projects Module Restructure
**Current Structure:**
```
internal/projects/
├── http.go
├── repo.go
├── route.go
└── public_id.go
```

**Target Structure (aligned with auth module pattern):**
```
internal/projects/
├── domain/
│   ├── model.go      # Project entity (Project struct, CreateProjectRequest, UpdateProjectRequest)
│   └── errors.go     # Domain errors (ErrProjectNotFound, ErrProjectAlreadyExists, etc.)
├── repository/
│   └── project_repo.go  # ProjectRepository (matches auth/repository/user_repo.go pattern)
├── service/
│   └── project_service.go  # ProjectService (matches auth/service/auth_service.go pattern)
└── http/
    ├── handlers_project.go  # Handler methods (matches auth/http/handlers_user.go pattern)
    ├── router.go            # Register() method (matches auth/http/router.go pattern)
    └── types.go             # HTTP request/response types (matches auth/http/types.go pattern)
```

**Note:** Keep `public_id.go` at root level (utility function, similar to how auth has `firebase.go` at root)

**Migration Steps (Following auth module pattern exactly):**

1. **Create `domain/` directory**
   - Create `domain/model.go`:
     - Move `Project` struct from `http.go` or `repo.go`
     - Add `CreateProjectRequest` struct (like `CreateUserRequest` in auth)
     - Add `UpdateProjectRequest` struct (like `UpdateUserRequest` in auth)
   - Create `domain/errors.go`:
     - Define domain errors: `ErrProjectNotFound`, `ErrProjectAlreadyExists`, `ErrInvalidInput`
     - Follow same pattern as `auth/domain/errors.go`

2. **Create `repository/` directory**
   - Move `repo.go` → `repository/project_repo.go`
   - Rename type: `Repo` → `ProjectRepository` (matches `UserRepository` pattern)
   - Rename constructor: `NewRepo` → `NewProjectRepository` (matches `NewUserRepository` pattern)
   - Update package name to `repository`
   - Update imports to use `domain` package
   - Convert pgxpool to `*sql.DB` (matches auth pattern)

3. **Create `service/` directory**
   - Create `service/project_service.go`:
     - Type: `ProjectService` (matches `AuthService` pattern)
     - Constructor: `NewProjectService(repo *repository.ProjectRepository)`
     - Move business logic from handlers to service methods
     - Service methods use repository (matches auth service pattern)

4. **Create `http/` directory**
   - Move `http.go` → `http/handlers_project.go` (matches `handlers_user.go` naming)
   - Move `route.go` → `http/router.go`
   - Create `http/types.go` for HTTP-specific request/response DTOs
   - Update package name to `http`
   - Handler struct: `Handler` with `projectService *service.ProjectService` field
   - Constructor: `New(service *service.ProjectService) *Handler`

5. **Keep utilities at root**
   - `public_id.go` stays at root (like `firebase.go` in auth module)

#### 3.2 Chats Module Restructure
**Current Structure:**
```
internal/design_input_processing/chats/
├── http.go
├── repo.go
├── route.go
├── types.go
└── id.go
```

**Target Structure (aligned with auth module pattern):**
```
internal/design_input_processing/chats/
├── domain/
│   ├── model.go      # Thread, Message, Attachment entities + request types
│   └── errors.go     # ErrThreadNotFound, ErrMessageNotFound, etc.
├── repository/
│   └── chat_repo.go  # ChatRepository (matches auth pattern)
├── service/
│   └── chat_service.go  # ChatService (matches auth pattern)
└── http/
    ├── handlers_chat.go  # Handler methods (matches handlers_user.go pattern)
    ├── router.go         # RegisterProjectRoutes() or Register() method
    └── types.go         # HTTP request/response types
```

**Migration Steps:**
- Follow same pattern as projects module
- Keep `id.go` at root (utility function)
- Note: Chats are nested under projects, so router might be `RegisterProjectRoutes()` instead of `Register()`

#### 3.3 Diagrams Module Restructure
**Current Structure:**
```
internal/design_input_processing/diagrams/
├── http.go
├── repo.go
└── route.go
```

**Target Structure (aligned with auth module pattern):**
```
internal/design_input_processing/diagrams/
├── domain/
│   ├── model.go      # DiagramVersion entity + request types
│   └── errors.go     # ErrDiagramNotFound, etc.
├── repository/
│   └── diagram_repo.go  # DiagramRepository (matches auth pattern)
├── service/
│   └── diagram_service.go  # DiagramService (matches auth pattern)
└── http/
    ├── handlers_diagram.go  # Handler methods (matches handlers_user.go pattern)
    ├── router.go            # RegisterProjectRoutes() method
    └── types.go            # HTTP request/response types
```

**Migration Steps:**
- Follow same pattern as projects module
- Note: Diagrams are nested under projects, so router might be `RegisterProjectRoutes()` instead of `Register()`

---

### Phase 4: Database Migration Alignment

#### 4.1 Check Current Migrations
**Dev Branch:**
- `0001_auth_users.sql` - Auth tables only

**Temp Branch:**
- `0001_auth_users.sql` - Auth tables only (same as dev)

**Note:** Projects, chats, diagrams tables must be in a separate migration

**Important:** User management is handled by `internal/auth/repository/user_repo.go` - do NOT create a separate users module or repository. The `users` table is already defined in `0001_auth_users.sql` and managed by the auth module.

#### 4.2 Create Proper Migration Sequence
**Target Migration Structure:**
```
migrations/
├── 0001_auth_users.sql              # Auth tables (from dev)
├── 0002_simulation_data_storage.sql # Simulation tables (from dev, if exists)
└── 0003_projects_chats_diagrams.sql # New feature tables (extract from temp)
```

**Steps:**
1. Check if `0002_simulation_data_storage.sql` exists in dev
   - If yes, copy it
   - If no, check if simulation tables are in `0001_auth_users.sql`

2. Extract new tables from temp branch
   - Check if projects/chats/diagrams tables are defined anywhere
   - Create `0003_projects_chats_diagrams.sql` with:
     - `projects` table
     - `chat_threads` table
     - `chat_messages` table
     - `chat_message_attachments` table
     - `diagram_versions` table
     - All related indexes and constraints

3. Verify migration sequence
   - Ensure migrations can run in order
   - Test migration rollback

---

### Phase 5: Update Main.go

#### 5.1 Remove Redundant Code
- Remove `pgxpool` imports and connection
- Remove `pgxDSN()` function
- Remove `apiroutes.RegisterV1()` call
- Remove dual database connection setup

#### 5.2 Add Module Registrations
Following dev branch pattern:
```go
// Single database connection
db, err := postgres.NewConnection(&cfg.Database)
// ...

// Projects module (following auth pattern)
projectsGroup := api.Group("/projects")
projectRepo := projectsrepository.NewProjectRepository(db)  // matches authrepo.NewUserRepository pattern
projectService := projectsservice.NewProjectService(projectRepo)  // matches authservice.NewAuthService pattern
projectHandler := projectshttp.New(projectService)  // matches authhttp.New pattern
projectHandler.Register(projectsGroup)  // matches authhttp.Register pattern

// Chats module (under projects)
chatRepo := chats.NewRepo(db)
chatService := chatservice.NewChatService(chatRepo)
chatHandler := chatshttp.New(chatService, uigpClient)
chats.RegisterProjectRoutes(projectsGroup, chatHandler)

// Diagrams module (under projects)
diagramRepo := diagrams.NewRepo(db)
diagramService := diagramservice.NewDiagramService(diagramRepo)
diagramHandler := diagramshttp.New(diagramService)
diagrams.RegisterProjectRoutes(projectsGroup, diagramRepo)
```

#### 5.3 Restore Simulation Module
```go
// Initialize simulation module (from dev)
simRunRepo := simrepo.NewRunRepository(redisClient)
simService := simservice.NewSimulationService(simRunRepo)
simHandler := simhttp.New(...)
simHandler.Register(simGroup)
```

---

### Phase 6: Code Pattern Standardization

#### 6.1 Repository Pattern (Match auth/repository/user_repo.go exactly)
**Standard (from auth module):**
- Package name: `repository`
- Type name: `*Repository` (e.g., `ProjectRepository`, `ChatRepository`, `DiagramRepository`)
- Constructor: `NewProjectRepository(db *sql.DB)` (matches `NewUserRepository` pattern)
- Uses `*sql.DB` (not pgxpool)
- Methods return domain entities and domain errors
- Error handling: `sql.ErrNoRows` → domain errors

**Apply to:**
- `internal/projects/repository/project_repo.go` → `ProjectRepository`
- `internal/design_input_processing/chats/repository/chat_repo.go` → `ChatRepository`
- `internal/design_input_processing/diagrams/repository/diagram_repo.go` → `DiagramRepository`

**Note:** No separate `internal/users` module needed - user management is handled by `internal/auth/repository/user_repo.go`

#### 6.2 Service Pattern (Match auth/service/auth_service.go exactly)
**Standard (from auth module):**
- Package name: `service`
- Type name: `*Service` (e.g., `ProjectService`, `ChatService`, `DiagramService`)
- Constructor: `NewProjectService(repo *repository.ProjectRepository)` (matches `NewAuthService` pattern)
- Contains business logic, validation, orchestration
- Service methods call repository methods
- Returns domain entities and domain errors

**Apply to:**
- `internal/projects/service/project_service.go` → `ProjectService`
- `internal/design_input_processing/chats/service/chat_service.go` → `ChatService`
- `internal/design_input_processing/diagrams/service/diagram_service.go` → `DiagramService`

#### 6.3 Handler Pattern (Match auth/http/handlers_user.go exactly)
**Standard (from auth module):**
- Package name: `http`
- Type name: `Handler`
- File name: `handlers_<module>.go` (e.g., `handlers_project.go`, `handlers_chat.go`)
- Constructor: `New(service *service.ProjectService)` (matches `auth/http.New` pattern)
- Handler struct has service field: `projectService *service.ProjectService`
- Handlers are thin - delegate to service
- Use domain entities and errors from service

**Apply to:**
- `internal/projects/http/handlers_project.go` → `Handler` with `ProjectService`
- `internal/design_input_processing/chats/http/handlers_chat.go` → `Handler` with `ChatService`
- `internal/design_input_processing/diagrams/http/handlers_diagram.go` → `Handler` with `DiagramService`

#### 6.4 Route Registration Pattern
**Standard (from dev):**
- Each module has `Register(rg *gin.RouterGroup)` method
- Called directly in `main.go`
- No centralized route registry

**Apply to:**
- Remove centralized route registration
- Add `Register()` methods to each module

---

### Phase 7: Testing & Verification

#### 7.1 Compilation Check
- Ensure all modules compile
- Fix import errors
- Resolve type mismatches

#### 7.2 Integration Test
- Test all modules together
- Verify database connections work
- Test route registration

#### 7.3 Migration Test
- Run migrations in sequence
- Verify all tables created
- Test rollback if needed

---

## Detailed File Changes

### Files to Delete
1. `internal/api/http/routes/v1.go` (centralized routes)
2. `internal/storage/postgres/dsn.go` (if exists, redundant)
3. Any pgxpool-specific utilities

### Files to Create
1. `internal/projects/domain/model.go`
2. `internal/projects/domain/errors.go`
3. `internal/projects/repository/project_repo.go`
4. `internal/projects/service/project_service.go`
5. `internal/projects/http/handlers_project.go` (matches `auth/http/handlers_user.go` naming)
6. `internal/projects/http/router.go`
7. `internal/projects/http/types.go`
8. Same structure for chats and diagrams (handlers_chat.go, handlers_diagram.go)
9. `migrations/0003_projects_chats_diagrams.sql`

**Important:** Do NOT create `internal/users` module - user management is handled by `internal/auth/repository/user_repo.go`

### Files to Modify
1. `cmd/api/main.go` - Major refactor
2. All repository files - Convert pgxpool to database/sql
3. All handler files - Add service layer
4. Route files - Align registration pattern

---

## Risk Mitigation

### High Risk Areas
1. **Database Query Conversion**
   - Risk: pgx and database/sql have different APIs
   - Mitigation: Test each query conversion thoroughly
   - Backup: Keep pgx version in comments for reference

2. **Service Layer Creation**
   - Risk: Business logic might be tightly coupled in handlers
   - Mitigation: Extract logic carefully, maintain functionality
   - Test: Ensure all endpoints work after refactoring

3. **Migration Conflicts**
   - Risk: Migration numbering conflicts
   - Mitigation: Use proper sequence (0001, 0002, 0003)
   - Test: Run migrations on clean database

### Medium Risk Areas
1. **Route Registration Changes**
   - Risk: Routes might not register correctly
   - Mitigation: Test all endpoints after changes
   - Verify: Check route paths match expected

2. **Import Path Updates**
   - Risk: Circular dependencies or missing imports
   - Mitigation: Update imports systematically
   - Verify: Compile after each module restructure

---

## Execution Order

### Step 1: Setup Branch (5 min)
- Create branch from temp
- Merge dev
- Resolve obvious conflicts

### Step 2: Database Connection Unification (2-3 hours)
- Convert all pgxpool to database/sql
- Update all repositories
- Test compilation

### Step 3: Remove Centralized Routes (1 hour)
- Delete centralized route files
- Add Register() methods to modules
- Update main.go

### Step 4: Restructure Modules (4-6 hours)
- Projects module (2 hours)
- Chats module (2 hours)
- Diagrams module (2 hours)

### Step 5: Migration Alignment (1 hour)
- Extract tables to new migration
- Verify sequence

### Step 6: Testing (2-3 hours)
- Compilation
- Integration tests
- Manual endpoint testing

**Total Estimated Time: 10-15 hours**

---

## Success Criteria

✅ All modules follow dev branch structure (domain/repository/service/http)
✅ Single database connection pattern (database/sql only)
✅ Module-specific route registration (no centralized routes)
✅ All new features (projects, chats, diagrams) preserved
✅ Simulation module restored and working
✅ Migrations properly sequenced
✅ Code compiles without errors
✅ All endpoints functional

---

## Notes

- **Preserve Functionality:** All new features must continue to work
- **Follow Auth Module Pattern Exactly:** Match the exact structure used in `internal/auth/`
- **No User Module Duplication:** User management is handled by `internal/auth/repository/user_repo.go` - do NOT create separate users module
- **Test Thoroughly:** Verify each change doesn't break functionality
- **Incremental Changes:** Make changes in small, testable increments

## Pattern Reference (Auth Module)

Use `internal/auth/` as the exact template:

```
internal/auth/
├── domain/
│   ├── errors.go          → Define domain errors
│   └── model.go           → Domain entities + request types
├── repository/
│   └── user_repo.go       → UserRepository with *sql.DB
├── service/
│   └── auth_service.go     → AuthService with business logic
└── http/
    ├── handlers_user.go    → Handler methods (thin, delegate to service)
    ├── router.go           → Register() method
    └── types.go            → HTTP-specific types
```

**Apply this exact pattern to:**
- `internal/projects/` → ProjectRepository, ProjectService, handlers_project.go
- `internal/design_input_processing/chats/` → ChatRepository, ChatService, handlers_chat.go
- `internal/design_input_processing/diagrams/` → DiagramRepository, DiagramService, handlers_diagram.go
