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

### Best-Candidate Storage (S3)

When S3 is configured (see `.env.example`), the realtime simulation module will:

- On simulation completion, call the simulation-core export endpoint to retrieve the `scenario_yaml`.
- Upload it to object storage under:
  - `simulation/{run_id}/best_scenario.yaml` (within the configured `S3_BUCKET`).
- Upsert the S3 object key into Postgres:
  - `simulation_summaries.best_candidate_s3_path`.

The API exposes this data via:

- `GET /api/v1/simulation/runs/{run_id}/best-candidate`
  - Returns the stored S3 path and a normalized view of the scenario’s hosts and services.

If S3 is not configured (`S3_BUCKET` empty), best-candidate uploads and this endpoint are effectively disabled.

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