# Design Input Processing Module - Architecture Documentation

## Overview

The `design_input_processing` module serves as a proxy/gateway layer between the frontend and upstream services (LLM service, Ollama). It provides job management, chat functionality, RAG search, and graph visualization capabilities.

## Architecture

### High-Level Flow

```
HTTP Request → Handler → Service Layer → Upstream Client → External Service
                ↓
            Response ← Service ← Upstream Client ← External Service
```

### Component Structure

```
internal/
├── design_input_processing/
│   ├── http/          # HTTP handlers (thin layer, delegates to services)
│   ├── service/       # Business logic and upstream communication
│   ├── graph/         # Graph utilities (DOT generation, Neo4j TODO)
│   ├── llm/           # LLM client (UIGP)
│   ├── middleware/    # Request ID, Firebase auth
│   └── rag/           # RAG search functionality
└── projects/
    ├── chats/         # Chat module (domain/repository/service/http)
    ├── diagrams/      # Diagram module (domain/repository/service/http)
    ├── domain/        # Project domain models
    ├── http/          # Project HTTP handlers
    └── repository/    # Project repository
```

## Service Layer

### UpstreamClient

**Purpose:** Handles all communication with the upstream LLM service.

**Key Methods:**
- `GetIntermediate(ctx, jobID)` - Fetch intermediate graph
- `Fuse(ctx, jobID, userID)` - Trigger fusion operation
- `Export(ctx, jobID, opts)` - Export job data
- `GetReport(ctx, jobID, query)` - Fetch job report
- `Ingest(ctx, body, headers)` - Forward ingest requests

**HTTP Clients:**
- `defaultClient` (30s timeout) - Standard operations
- `longClient` (90s timeout) - Export/ingest operations
- `fuseClient` (3min timeout) - Fuse operations

**Error Handling:**
- Returns `*http.Response` and `error`
- Errors are wrapped with context using `fmt.Errorf`

### JobService

**Purpose:** Handles job-related business logic.

**Key Methods:**
- `ListJobIDs(userID)` - List all job IDs for a user
- `GetJobSummaries(ctx, userID)` - Get summaries for all user jobs
- `SummarizeJob(id, ig, report)` - Create job summary from data

**Dependencies:**
- `UpstreamClient` - For fetching job data

### GraphService

**Purpose:** Handles graph/DOT generation operations.

**Key Methods:**
- `GetGraphvizDOT(ctx, jobID)` - Generate GraphViz DOT from job export

**Flow:**
1. Fetch export as JSON via `UpstreamClient`
2. Convert to `graph.Architecture` type
3. Build `graph.Graph` using `graph.FromSpec()`
4. Generate DOT using `graph.ToDOT()`

### SignalService

**Purpose:** Extracts signals (RPS, latency, CPU, etc.) from chat history.

**Key Methods:**
- `LoadSignalsFromHistory(jobID, userID)` - Extract signals from chat logs

**Signal Types:**
- `rps_peak` - Peak requests per second
- `latency_p95_ms` - 95th percentile latency
- `cpu_vcpu` - CPU vCPU count
- `payload_kb` - Payload size in KB
- `burst_factor` - Burst multiplier

## HTTP Handlers

### Handler Structure

All handlers follow a consistent pattern:
1. Extract parameters from request
2. Validate input
3. Call appropriate service method
4. Handle errors consistently
5. Return response

### Proxy Pattern

For handlers that proxy requests to upstream services:
- Use `proxyResponse()` for simple pass-through
- Use `proxyResponseWithBody()` when modifying response body

**Example:**
```go
func (h *Handler) intermediate(c *gin.Context) {
    resp, err := h.upstreamClient.GetIntermediate(c.Request.Context(), id)
    if err != nil {
        c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
        return
    }
    defer resp.Body.Close()
    proxyResponse(c, resp)
}
```

### Error Handling

**Standard Error Responses:**
- `502 Bad Gateway` - Upstream service errors
- `500 Internal Server Error` - Server-side errors
- `401 Unauthorized` - Authentication required
- `400 Bad Request` - Invalid input

**Response Format:**
```json
{
  "ok": false,
  "error": "error message"
}
```

## Constants

### Timeouts

Defined in `service/constants.go`:
- `DefaultTimeout` (30s) - Standard operations
- `LongTimeout` (90s) - Export/ingest operations
- `FuseTimeout` (3min) - Fuse operations

## Modules

### Chats Module

**Location:** `internal/projects/chats/`

**Structure:**
- `domain/` - Models, errors, IDs
- `repository/` - Database operations
- `service/` - Business logic (LLM integration, message processing)
- `http/` - HTTP handlers

**Routes:**
- `POST /api/v1/projects/:public_id/chats` - Create thread
- `GET /api/v1/projects/:public_id/chats` - List threads
- `POST /api/v1/projects/:public_id/chats/:thread_id/messages` - Post message
- `GET /api/v1/projects/:public_id/chats/:thread_id/messages` - List messages

**Features:**
- Database-backed (not file-based)
- Integrated with UIGP LLM client
- Supports attachments
- Firebase auth required

### Diagrams Module

**Location:** `internal/projects/diagrams/`

**Structure:**
- `domain/` - Models, errors, IDs
- `repository/` - Database operations
- `service/` - Business logic (LLM integration, message processing)
- `http/` - HTTP handlers

**Purpose:** Manages diagram versions for projects.

## Graph Package

**Location:** `internal/design_input_processing/graph/`

**Components:**
- `model.go` - Graph, Node, Edge types
- `from_spec.go` - Convert Architecture spec to Graph
- `dot.go` - Generate GraphViz DOT format
- `neo4j.go` - TODO: Neo4j integration (future work)

**Usage:**
Used by `GraphService` to generate DOT visualizations from job exports.

## RAG (Retrieval-Augmented Generation)

**Location:** `internal/design_input_processing/rag/`

**Purpose:** Provides semantic search over documentation snippets.

**Components:**
- `store.go` - Document storage and search
- `snippets/` - Markdown documentation files
- `types.go` - Result types

**Routes:**
- `GET /api/v1/design-input/rag/search?q=query` - Search documentation
- `POST /api/v1/design-input/rag/reload` - Reload snippets

## Middleware

### RequestIDMiddleware

**Purpose:** Ensures every request has a unique request ID.

**Behavior:**
- Reads `X-Request-Id` header if present
- Generates new ID if missing
- Stores in context as `request_id`
- Echoes back in response header
- Logs request with ID, method, path, status, latency

### FirebaseAuthMiddleware

**Purpose:** Validates Firebase authentication tokens.

**Behavior:**
- Extracts token from `Authorization` header
- Validates with Firebase
- Stores `firebase_uid` in context
- Returns 401 if invalid/missing

## Error Handling Strategy

### Service Layer

Services return errors with context:
```go
return nil, fmt.Errorf("operation failed: %w", err)
```

### HTTP Handlers

Handlers convert service errors to HTTP responses:
- Upstream errors → `502 Bad Gateway`
- Validation errors → `400 Bad Request`
- Auth errors → `401 Unauthorized`
- Server errors → `500 Internal Server Error`

## Testing Strategy

### Unit Tests

Test service layer in isolation:
- Mock `UpstreamClient` for service tests
- Test business logic without HTTP concerns
- Verify error handling and edge cases

### Integration Tests

Test full request/response flow:
- Use test HTTP server for upstream mocking
- Verify handler behavior with real services
- Test error propagation

## Future Improvements

1. **Structured Logging** - Add context-aware logging with request IDs
2. **Metrics** - Add observability metrics (latency, error rates)
3. **Neo4j Integration** - Implement graph storage (see `graph/neo4j.go`)
4. **Rate Limiting** - Add rate limiting for upstream calls
5. **Caching** - Cache frequently accessed job data
