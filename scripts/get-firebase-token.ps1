# Get a Firebase ID token (and UID) for testing the API.
# Uses the Firebase Auth Emulator - your backend must be started with
#   $env:FIREBASE_AUTH_EMULATOR_HOST = "127.0.0.1:9099"
# and the Auth emulator must be running: firebase emulators:start --only auth

param(
    [string]$EmulatorHost = "http://localhost:9099",
    [string]$Email = "malithgihan099@gmail.com",
    [string]$Password = "test1234",
    [switch]$TestApi,
    [string]$ApiBase = "http://localhost:8000"
)

$ErrorActionPreference = "Stop"
$apiKey = "fake-api-key"
# Firebase REST API uses colon in path: accounts:signUp, accounts:signInWithPassword
$signUpUrl = "$EmulatorHost/identitytoolkit.googleapis.com/v1/accounts:signUp?key=$apiKey"
$signInUrl = "$EmulatorHost/identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=$apiKey"

# 1) Try sign-up (creates user if new); if email exists, sign in instead
$signUpBody = @{
    email             = $Email
    password          = $Password
    returnSecureToken = $true
} | ConvertTo-Json

$idToken = $null
$localId = $null

try {
    $signUpResp = Invoke-RestMethod -Method Post `
        -Uri $signUpUrl `
        -ContentType "application/json" `
        -Body $signUpBody
    $idToken = $signUpResp.idToken
    $localId = $signUpResp.localId
    Write-Host "Created new user and got token."
} catch {
    $errBody = $_.ErrorDetails.Message | ConvertFrom-Json -ErrorAction SilentlyContinue
    if ($errBody.error.message -match "EMAIL_EXISTS") {
        $signInBody = @{
            email             = $Email
            password          = $Password
            returnSecureToken = $true
        } | ConvertTo-Json
        $signInResp = Invoke-RestMethod -Method Post `
            -Uri $signInUrl `
            -ContentType "application/json" `
            -Body $signInBody
        $idToken = $signInResp.idToken
        $localId = $signInResp.localId
        Write-Host "Signed in existing user and got token."
    } else {
        throw
    }
}

Write-Host ""
Write-Host "--- Firebase auth (emulator) ---"
Write-Host "UID (localId): $localId"
Write-Host "Email:         $Email"
Write-Host "ID token (use as Bearer):"
Write-Host $idToken
Write-Host ""
Write-Host "To call the API, run the following in PowerShell (API must be running with FIREBASE_AUTH_EMULATOR_HOST=127.0.0.1:9099):"
Write-Host ""
Write-Host "  `$token = '$idToken'" -ForegroundColor Cyan
Write-Host '  Invoke-RestMethod -Method Get -Uri "' -NoNewline -ForegroundColor Cyan
Write-Host $ApiBase -NoNewline -ForegroundColor Cyan
Write-Host '/api/v1/projects" -Headers @{ "Authorization" = "Bearer $token" }' -ForegroundColor Cyan
Write-Host ""

if ($TestApi) {
    Write-Host "Calling GET $ApiBase/api/v1/projects with Bearer token..."
    try {
        $projects = Invoke-RestMethod -Method Get `
            -Uri "$ApiBase/api/v1/projects" `
            -Headers @{ "Authorization" = "Bearer $idToken" }
        Write-Host "Success. Response:"
        $projects | ConvertTo-Json -Depth 5
    } catch {
        Write-Host "API call failed: $_"
        Write-Host "Ensure: 1) API is running, 2) Started with FIREBASE_AUTH_EMULATOR_HOST=127.0.0.1:9099"
        exit 1
    }
}

# To use in another script: run and parse the "UID" and "ID token" lines, or use -TestApi to verify.
