# Testing Setup Guide

This document explains how to set up and run tests for the go-sim-backend project.

## Test Structure

- **Unit Tests** (`tests/unit/`): Fast tests that don't require external dependencies
  - Repository tests use `sqlmock` for database mocking
  - Handler tests validate request/response structures
  - Can run without any external services

- **Integration Tests** (`tests/integration/`): Tests that require real services
  - Redis tests use `miniredis` (in-memory Redis)
  - PostgreSQL tests require a real database connection
  - Simulation service tests require both Redis and PostgreSQL

## Setting Up PostgreSQL for Integration Tests

Integration tests that use PostgreSQL require a database connection. You can configure this in several ways:

### Option 1: Set TEST_DB_DSN Directly (Recommended)

Set the `TEST_DB_DSN` environment variable with a PostgreSQL connection string:

**PowerShell:**
```powershell
$env:TEST_DB_DSN = "host=localhost port=5432 user=postgres password=yourpassword dbname=gosim_test sslmode=disable"
go test ./tests/integration/... -v
```

**Command Prompt:**
```cmd
set TEST_DB_DSN=host=localhost port=5432 user=postgres password=yourpassword dbname=gosim_test sslmode=disable
go test ./tests/integration/... -v
```

**Linux/Mac:**
```bash
export TEST_DB_DSN="host=localhost port=5432 user=postgres password=yourpassword dbname=gosim_test sslmode=disable"
go test ./tests/integration/... -v
```

### Option 2: Use Individual Environment Variables

Set individual database configuration variables:

```powershell
$env:TEST_DB_HOST = "localhost"
$env:TEST_DB_PORT = "5432"
$env:TEST_DB_USER = "postgres"
$env:TEST_DB_PASSWORD = "yourpassword"
$env:TEST_DB_NAME = "gosim_test"
go test ./tests/integration/... -v
```

### Option 3: Use Main Database Configuration

If you have your main database configured (via `.env` file or `DB_*` environment variables), the tests will automatically fall back to using those if `TEST_DB_*` variables are not set.

### Option 4: Use the PowerShell Script

We provide a convenient PowerShell script to run integration tests:

```powershell
# Use default test database (gosim_test)
.\scripts\run-integration-tests.ps1

# Use custom database
.\scripts\run-integration-tests.ps1 -DbHost localhost -DbPort 5432 -DbUser postgres -DbPassword mypass -DbName gosim_test

# Use main database configuration from .env
.\scripts\run-integration-tests.ps1 -UseMainDb
```

## Creating a Test Database

Before running integration tests, make sure you have a test database created:

```sql
-- Connect to PostgreSQL
psql -U postgres

-- Create test database
CREATE DATABASE gosim_test;

-- Run migrations on test database
\c gosim_test
\i migrations/0001_auth_users.sql
\i migrations/0002_simulation_data_storage.sql
```

Or using the migration script:

```powershell
# Set database connection for test DB
$env:DB_NAME = "gosim_test"
.\scripts\run-migration.ps1
```

## Running Tests

### Run All Tests
```powershell
go test ./tests/... -v
```

### Run Only Unit Tests
```powershell
go test ./tests/unit/... -v
```

### Run Only Integration Tests
```powershell
# Make sure TEST_DB_DSN is set first
go test ./tests/integration/... -v
```

### Run Specific Test
```powershell
go test ./tests/integration/... -v -run TestSimulationService_StoreRunSummaryAndMetrics
```

## Test Environment Variables Summary

| Variable | Description | Required For |
|----------|-------------|--------------|
| `TEST_DB_DSN` | Complete PostgreSQL connection string | PostgreSQL integration tests |
| `TEST_DB_HOST` | Database host | PostgreSQL integration tests (alternative) |
| `TEST_DB_PORT` | Database port | PostgreSQL integration tests (alternative) |
| `TEST_DB_USER` | Database user | PostgreSQL integration tests (alternative) |
| `TEST_DB_PASSWORD` | Database password | PostgreSQL integration tests (alternative) |
| `TEST_DB_NAME` | Database name | PostgreSQL integration tests (alternative) |
| `DB_HOST`, `DB_PORT`, etc. | Main database config | Fallback for tests if TEST_DB_* not set |

## Troubleshooting

### Tests Skip with "TEST_DB_DSN not set"
- Make sure you've set `TEST_DB_DSN` or the individual `TEST_DB_*` variables
- Check that the environment variable is set in the current shell session
- Try using the PowerShell script: `.\scripts\run-integration-tests.ps1`

### Connection Refused Errors
- Verify PostgreSQL is running: `pg_isready` or check service status
- Check that the host and port are correct
- Ensure firewall rules allow connections

### Authentication Failed
- Verify username and password are correct
- Check PostgreSQL `pg_hba.conf` allows connections from your IP
- Try connecting manually: `psql -h localhost -U postgres -d gosim_test`

### Database Does Not Exist
- Create the test database: `CREATE DATABASE gosim_test;`
- Run migrations on the test database

### Tables Don't Exist
- Run migrations on your test database
- Check that migrations were applied: `\dt` in psql

## Best Practices

1. **Use a Separate Test Database**: Always use a different database for tests (e.g., `gosim_test`) to avoid affecting your development data.

2. **Clean Up After Tests**: Consider adding test cleanup logic or using database transactions that roll back after each test.

3. **Environment-Specific Config**: Use `.env.test` or separate environment files for test configuration.

4. **CI/CD Setup**: In CI/CD pipelines, set `TEST_DB_DSN` as a secret environment variable.
