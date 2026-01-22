# Run database migration
# Usage: .\scripts\run-migration.ps1 [migration-file]

param(
    [string]$MigrationFile = "migrations/0001_auth_users.sql"
)

# Load environment variables from .env if it exists
if (Test-Path .env) {
    Get-Content .env | ForEach-Object {
        if ($_ -match '^\s*([^#][^=]*)\s*=\s*(.*)\s*$') {
            $name = $matches[1].Trim()
            $value = $matches[2].Trim()
            [Environment]::SetEnvironmentVariable($name, $value, "Process")
        }
    }
}

# Get database connection details
$DB_HOST = if ($env:DB_HOST) { $env:DB_HOST } else { "localhost" }
$DB_PORT = if ($env:DB_PORT) { $env:DB_PORT } else { "5432" }
$DB_USER = if ($env:DB_USER) { $env:DB_USER } else { "postgres" }
$DB_NAME = if ($env:DB_NAME) { $env:DB_NAME } else { "gosim" }

Write-Host "Running migration: $MigrationFile" -ForegroundColor Cyan
Write-Host "Database: $DB_NAME on ${DB_HOST}:${DB_PORT}" -ForegroundColor Yellow
Write-Host ""

# Run psql command
$psqlArgs = @(
    "-h", $DB_HOST
    "-p", $DB_PORT
    "-U", $DB_USER
    "-d", $DB_NAME
    "-f", $MigrationFile
)

& psql $psqlArgs

if ($LASTEXITCODE -eq 0) {
    Write-Host "`nMigration completed successfully!" -ForegroundColor Green
} else {
    Write-Host "`nMigration failed with exit code: $LASTEXITCODE" -ForegroundColor Red
    exit $LASTEXITCODE
}

