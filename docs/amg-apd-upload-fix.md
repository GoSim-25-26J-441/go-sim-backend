# AMG-APD Analyze Upload 500 Error – Fix Guide

## Root Cause

The error `{"error":"fetch failed"}` with HTTP 500 occurs when:

1. **Frontend** (`go-sim-frontend`) calls `POST /api/amg-apd/analyze-upload` (Next.js API route)
2. **Next.js API route** proxies to `http://localhost:8080/api/v1/amg-apd/analyze-raw`
3. **Backend** is not running or not reachable at port 8080 → `fetch()` fails → returns `{"error":"fetch failed"}`

## Request Flow

```
Browser → POST /api/amg-apd/analyze-upload (Next.js on :3000)
         → Next.js API route fetches http://localhost:8080/api/v1/amg-apd/analyze-raw
         → Go backend (must be on :8080)
```

## Fixes Applied (Backend)

### 1. Docker Compose – Postgres & Redis

The backend requires **PostgreSQL** and **Redis** to start. Previously, `docker-compose.yaml` only had Neo4j, so the backend failed to start.

**Change:** Postgres and Redis services were added to `docker-compose.yaml`. The backend now waits for them before starting.

**To run with Docker:**

```bash
# Ensure .env has DB_PASSWORD=postgres for the new Postgres container
# Or create .env from .env.example and set DB_PASSWORD=postgres

docker-compose up -d
```

### 2. Configurable Paths for Local Dev (Windows)

The handlers used hardcoded `/app/incoming` and `/app/out`, which can fail on Windows.

**Change:** Added env vars:

- `AMG_APD_INCOMING_DIR` – temp dir for uploaded files (default: `/app/incoming`)
- `AMG_APD_OUT_DIR` – output dir (default: `/app/out`)

**For local dev on Windows**, add to `.env`:

```
AMG_APD_INCOMING_DIR=%TEMP%\amg-incoming
AMG_APD_OUT_DIR=./out
```

(Use `$env:TEMP\amg-incoming` in PowerShell if needed.)

## How to Fix Your Setup

### Option A: Run Backend with Docker

1. Copy `.env.example` to `.env`
2. Set `DB_PASSWORD=postgres` (matches Postgres container)
3. Run: `docker-compose up -d`
4. Wait for backend to be healthy (check `http://localhost:8080/health`)
5. Run frontend: `cd go-sim-frontend && npm run dev`
6. Ensure frontend `.env` has `NEXT_PUBLIC_BACKEND_BASE=http://localhost:8080` (default)

### Option B: Run Backend Locally (go run)

1. Start Postgres and Redis (e.g. via Docker or local install)
2. Copy `.env.example` to `.env` and set DB/Redis credentials
3. On Windows, add:
   ```
   AMG_APD_INCOMING_DIR=%TEMP%\amg-incoming
   AMG_APD_OUT_DIR=./out
   ```
4. Install Graphviz (`dot` in PATH) – required for analysis
5. Run: `go run cmd/api/main.go`
6. Run frontend: `cd go-sim-frontend && npm run dev`

### Option C: Frontend in Docker / Different Host

If the frontend runs in Docker or on another host, `localhost:8080` from inside the container will not reach the host backend.

- Set `NEXT_PUBLIC_BACKEND_BASE` in the frontend `.env` to the backend URL (e.g. `http://host.docker.internal:8080` or your host IP).

## Verify Backend Is Running

```bash
curl http://localhost:8080/health
```

Expected:

```json
{"status":"healthy","timestamp":"...","service":"amg-apd-service","version":"1.0.0"}
```

## Backend Endpoints (AMG-APD)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/amg-apd/analyze-raw` | Analyze YAML from JSON body (used by Next.js proxy) |
| POST | `/api/v1/amg-apd/analyze` | Analyze YAML from multipart file upload |

## Frontend API Route (Reference)

The Next.js route at `go-sim-frontend/src/app/api/amg-apd/analyze-upload/route.ts`:

- Reads the uploaded file
- Sends it as JSON to `{NEXT_PUBLIC_BACKEND_BASE}/api/v1/amg-apd/analyze-raw`
- On connection failure, returns `{"error":"fetch failed"}` with 500

Ensure `NEXT_PUBLIC_BACKEND_BASE` points to the running backend.
