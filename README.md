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

- On simulation completion, the backend receives a callback and (for **batch optimization** runs) the payload may include `best_run_id` and `top_candidates`.
- **Batch optimization:** When `best_run_id` or `top_candidates` are present, the backend fetches the best scenario from `GET /v1/runs/{best_run_id}/export` and builds the candidates list by calling `GET /v1/runs/{id}/export` for each unique id in `{best_run_id} ∪ top_candidates`. Each candidate is persisted; the best candidate’s S3 path is stored as `best_scenario.yaml`.
- **Normal / online runs:** When no optimization IDs are present, the backend calls `GET /v1/runs/{engine_run_id}/export` once and uses that response for the single best scenario and (if present) the export’s `candidates` array or a single synthesized candidate.
- The best scenario is uploaded to object storage under:
  - `simulation/{run_id}/best_scenario.yaml` (within the configured `S3_BUCKET`).
- The S3 object key is upserted into Postgres:
  - `simulation_summaries.best_candidate_s3_path`.

The API exposes this data via the unified candidates endpoint:

- `GET /api/v1/simulation/runs/{run_id}/candidates`
  - Returns all candidates for the run (array), plus optional `best_candidate_id` and `best_candidate` (S3 path and normalized hosts/services) when a best scenario is stored.

If S3 is not configured (`S3_BUCKET` empty), best-candidate uploads and the `best_candidate` section in the response are effectively disabled.

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
