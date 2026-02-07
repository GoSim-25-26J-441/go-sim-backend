# Getting a Firebase token for local API testing

The `/api/v1/projects` (and other protected) endpoints require a **Firebase ID token** in the `Authorization: Bearer <token>` header. The backend gets the user UID from the token, not from `X-User-Id` headers.

To get a token and UID for testing **without changing any backend code**, use the **Firebase Auth Emulator** and the provided script.

## 1. One-time setup

### Install Firebase CLI (required first)

You need the Firebase CLI so you can start the Auth emulator. Run **each command separately** (no `then` in PowerShell).

**Option A – using Node.js (recommended)**  
If you have [Node.js](https://nodejs.org/) installed:

```powershell
npm install -g firebase-tools
```

Then **close and reopen PowerShell** (or open a new terminal) so `firebase` is on your PATH. Check with:

```powershell
firebase --version
```

**Option B – standalone install**  
If you don’t use Node, install the CLI using the [Firebase CLI install guide](https://firebase.google.com/docs/cli#install_the_firebase_cli) (e.g. Windows standalone installer).

### Start the Auth emulator

This repo already has `firebase.json` and `.firebaserc` (project `arc-find`). From the repo root:

**First time only** – log in to Firebase:

```powershell
firebase login
```

**Every time you want to test** – start the Auth emulator:

```powershell
firebase emulators:start --only auth
```

Leave this terminal open. The Auth emulator runs on port **9099** by default.

## 2. Run the API with the emulator

In a **second** terminal, point the backend at the emulator and start the API:

```powershell
$env:FIREBASE_AUTH_EMULATOR_HOST = "127.0.0.1:9099"
# Optional: set PORT if your API uses something other than 8000
go run ./cmd/api
```

Important: use `127.0.0.1:9099` **without** `http://`. The Go Firebase Admin SDK reads this env var and then accepts tokens issued by the emulator.

## 3. Get a token and UID

In a **third** terminal (or after the API is running):

```powershell
cd d:\Research\go-sim-backend
.\scripts\get-firebase-token.ps1
```

This will:

- Create a user `demo@local.test` in the emulator (or sign in if it already exists).
- Print the **UID** (`localId`) and the **ID token**.
- You can copy the token and use it in `Authorization: Bearer <token>`.

Custom email/password:

```powershell
.\scripts\get-firebase-token.ps1 -Email "you@test.com" -Password "mypass"
```

## 4. Test the projects API

**Option A – script does the call for you**

```powershell
.\scripts\get-firebase-token.ps1 -TestApi
```

This gets a token and then calls `GET http://localhost:8000/api/v1/projects` with that token. If you see a JSON response (e.g. list of projects), it’s working.

**Option B – call the API yourself**

1. Run `.\scripts\get-firebase-token.ps1` and copy the printed ID token.
2. Call the API:

```powershell
$token = "PASTE_ID_TOKEN_HERE"
Invoke-RestMethod -Method Get `
  -Uri "http://localhost:8000/api/v1/projects" `
  -Headers @{ "Authorization" = "Bearer $token" }
```

You should get a 200 response (e.g. `[]` or a list of projects). If you get `401` and "missing authorization token" or "invalid token", the backend is not using the emulator (check `FIREBASE_AUTH_EMULATOR_HOST`) or the token was not sent correctly.

## Summary

| Step | What to do |
|------|------------|
| 1 | Start Auth emulator: `firebase emulators:start --only auth` |
| 2 | Start API with emulator: `$env:FIREBASE_AUTH_EMULATOR_HOST = "127.0.0.1:9099"; go run ./cmd/api` |
| 3 | Get token: `.\scripts\get-firebase-token.ps1` (copy UID + token) |
| 4 | Test: `.\scripts\get-firebase-token.ps1 -TestApi` or use the token in `Authorization: Bearer <token>` |

No backend code changes are required; only environment (emulator) and request (Bearer token) are used.
