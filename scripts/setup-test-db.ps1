# Setup test database by running migrations
# This script uses the Go database connection instead of psql

param(
    [string]$TestDbName = "gosim_test"
)

Write-Host "Setting up test database: $TestDbName" -ForegroundColor Cyan
Write-Host "=====================================" -ForegroundColor Cyan
Write-Host ""

# Load .env file to get database connection details
if (Test-Path .env) {
    Get-Content .env | ForEach-Object {
        if ($_ -match '^\s*([^#][^=]+)\s*=\s*(.*)\s*$') {
            $name = $matches[1].Trim()
            $value = $matches[2].Trim()
            [Environment]::SetEnvironmentVariable($name, $value, "Process")
        }
    }
}

# Override DB_NAME for test database
$env:DB_NAME = $TestDbName

Write-Host "Database Configuration:" -ForegroundColor Green
Write-Host "  Host: $($env:DB_HOST)"
Write-Host "  Port: $($env:DB_PORT)"
Write-Host "  User: $($env:DB_USER)"
Write-Host "  Database: $env:DB_NAME"
Write-Host ""

# Check if we can connect
Write-Host "Checking database connection..." -ForegroundColor Yellow
$testConn = "host=$($env:DB_HOST) port=$($env:DB_PORT) user=$($env:DB_USER) password=$($env:DB_PASSWORD) dbname=postgres sslmode=disable"

# Try to create database if it doesn't exist (using psql if available, otherwise manual instruction)
Write-Host ""
Write-Host "To set up the test database, you need to:" -ForegroundColor Yellow
Write-Host "1. Create the database (if it doesn't exist):" -ForegroundColor White
Write-Host "   psql -U $($env:DB_USER) -h $($env:DB_HOST) -c `"CREATE DATABASE $TestDbName;`"" -ForegroundColor Cyan
Write-Host ""
Write-Host "2. Run the migrations:" -ForegroundColor White
Write-Host "   psql -U $($env:DB_USER) -h $($env:DB_HOST) -d $TestDbName -f migrations/0002_simulation_data_storage.sql" -ForegroundColor Cyan
Write-Host ""
Write-Host "Or if you have psql in your PATH, run:" -ForegroundColor Yellow
Write-Host "   `$env:DB_NAME = '$TestDbName'; .\scripts\run-migration.ps1 -MigrationFile migrations/0002_simulation_data_storage.sql" -ForegroundColor Cyan
Write-Host ""
