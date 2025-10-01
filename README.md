# Go Sim Backend

A Go-based simulation backend service.

## Getting Started

### Prerequisites

- Go 1.25.1 or higher

### Running the Application

```bash
go run cmd/api/main.go
```

The server will start on port 8080 by default. You can override this by setting the `PORT` environment variable.

### Health Check

The application provides health check endpoints:

- `GET /health` - Returns service health status
- `GET /healthz` - Kubernetes-style health check endpoint

Example response:
```json
{
  "status": "healthy",
  "timestamp": "2025-10-01T07:11:00Z",
  "service": "go-sim-backend",
  "version": "1.0.0"
}
```

### Testing

Run unit tests:
```bash
go test ./tests/unit/...
```

Run integration tests:
```bash
go test ./tests/integration/...
```

### Project Structure

```
├── cmd/
│   ├── api/           # HTTP API server
│   └── worker/        # Background workers
├── internal/
│   ├── api/
│   │   ├── http/      # HTTP handlers
│   │   └── grpc/      # gRPC handlers
│   ├── shared/        # Shared utilities
│   └── storage/       # Data persistence
├── migrations/        # Database migrations
├── scripts/          # Build and deployment scripts
└── tests/            # Test files
```