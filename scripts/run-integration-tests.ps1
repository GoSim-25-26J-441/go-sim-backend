# PowerShell script to run integration tests with PostgreSQL
# This script sets up the TEST_DB_DSN environment variable and runs the tests

param(
    [string]$DbHost = "localhost",
    [string]$DbPort = "5432",
    [string]$DbUser = "postgres",
    [string]$DbPassword = "",
    [string]$DbName = "gosim_test",
    [string]$TestDsn = "",  # If provided, use this directly instead of constructing
    [switch]$UseMainDb  # Use main DB config from .env instead of test DB
)

Write-Host "Running Integration Tests with PostgreSQL" -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan

# Load .env file first to get TEST_DB_DSN if it exists
if (Test-Path .env) {
    Get-Content .env | ForEach-Object {
        if ($_ -match '^\s*([^#][^=]+)\s*=\s*(.*)\s*$') {
            $key = $matches[1].Trim()
            $value = $matches[2].Trim()
            if ($key -eq "TEST_DB_DSN") {
                $env:TEST_DB_DSN = $value
            }
        }
    }
}

# If TEST_DB_DSN is already set, use it
if ($env:TEST_DB_DSN) {
    Write-Host "Using existing TEST_DB_DSN from environment" -ForegroundColor Yellow
    $TestDsn = $env:TEST_DB_DSN
}

# If UseMainDb is set, try to load from .env or existing DB_* vars
if ($UseMainDb) {
    Write-Host "Using main database configuration" -ForegroundColor Yellow
    
    # Try to load .env file if it exists
    if (Test-Path ".env") {
        Get-Content .env | ForEach-Object {
            if ($_ -match '^\s*([^#][^=]+)=(.*)$') {
                $key = $matches[1].Trim()
                $value = $matches[2].Trim()
                Set-Item -Path "env:$key" -Value $value
            }
        }
    }
    
    $DbHost = if ($env:DB_HOST) { $env:DB_HOST } else { $DbHost }
    $DbPort = if ($env:DB_PORT) { $env:DB_PORT } else { $DbPort }
    $DbUser = if ($env:DB_USER) { $env:DB_USER } else { $DbUser }
    $DbPassword = if ($env:DB_PASSWORD) { $env:DB_PASSWORD } else { $DbPassword }
    $DbName = if ($env:DB_NAME) { $env:DB_NAME } else { $DbName }
}

# Construct DSN if not provided directly
if ([string]::IsNullOrEmpty($TestDsn)) {
    if ([string]::IsNullOrEmpty($DbPassword)) {
        $TestDsn = "host=$DbHost port=$DbPort user=$DbUser dbname=$DbName sslmode=disable"
    } else {
        $TestDsn = "host=$DbHost port=$DbPort user=$DbUser password=$DbPassword dbname=$DbName sslmode=disable"
    }
}

Write-Host ""
Write-Host "Database Configuration:" -ForegroundColor Green
if ($env:TEST_DB_DSN -and $TestDsn -eq $env:TEST_DB_DSN) {
    # TEST_DB_DSN was loaded from .env or environment
    Write-Host "  Using TEST_DB_DSN from .env/environment" -ForegroundColor Yellow
    # Try to hide password in DSN if it contains password=
    if ($TestDsn -match 'password=([^\s]+)') {
        $maskedDsn = $TestDsn -replace 'password=[^\s]+', 'password=***'
        Write-Host "  DSN: $maskedDsn"
    } else {
        Write-Host "  DSN: $TestDsn"
    }
} else {
    # DSN was constructed from individual parameters
    Write-Host "  Host: $DbHost"
    Write-Host "  Port: $DbPort"
    Write-Host "  User: $DbUser"
    Write-Host "  Database: $DbName"
    # Hide password in output (only if password is not empty)
    if (-not [string]::IsNullOrEmpty($DbPassword)) {
        Write-Host "  DSN: $($TestDsn.Replace($DbPassword, '***'))"
    } else {
        Write-Host "  DSN: $TestDsn"
    }
}
Write-Host ""

# Set environment variable for this session
$env:TEST_DB_DSN = $TestDsn

# Run the integration tests
Write-Host "Running tests..." -ForegroundColor Cyan
Write-Host ""

go test ./tests/integration/... -v

$exitCode = $LASTEXITCODE

if ($exitCode -eq 0) {
    Write-Host ""
    Write-Host "All integration tests passed!" -ForegroundColor Green
} else {
    Write-Host ""
    Write-Host "Some tests failed. Exit code: $exitCode" -ForegroundColor Red
}

exit $exitCode
