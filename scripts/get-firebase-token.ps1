# Get a Firebase ID token (and UID) for testing the API.
# Uses the Firebase Auth Emulator - your backend must be started with
#   $env:FIREBASE_AUTH_EMULATOR_HOST = "127.0.0.1:9099"
# and the Auth emulator must be running: firebase emulators:start --only auth

param(
    [string]$EmulatorHost = "http://localhost:9099",
    [string]$Email = "demo@local.test",
    [string]$Password = "testpassword123",
    [switch]$TestApi,
    [string]$ApiBase = "http://localhost:8000"
)

$ErrorActionPreference = "Stop"
$apiKey = "fake-api-key"
$baseUrl = "$EmulatorHost/identitytoolkit.googleapis.com/v1/accounts"

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
        -Uri "$baseUrl/signUp?key=$apiKey" `
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
            -Uri "$baseUrl/signInWithPassword?key=$apiKey" `
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
Write-Host "Example request:"
Write-Host '  Invoke-RestMethod -Method Get -Uri "' + $ApiBase + '/api/v1/projects" -Headers @{ "Authorization" = "Bearer ' + $idToken.Substring(0, [Math]::Min(40, $idToken.Length)) + '..." }'
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
