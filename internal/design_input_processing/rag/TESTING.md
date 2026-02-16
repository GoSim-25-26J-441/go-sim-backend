# Testing the Requirements Questionnaire Feature

## Overview

The Requirements Questionnaire feature allows you to gather user requirements (server type, user count, RPS, latency, etc.) **before** the first LLM chat call. This only activates when a thread has no chat history.

## Prerequisites

1. Start Firebase Auth emulator:
   ```powershell
   firebase emulators:start --only auth
   ```

2. Start the API server (in a separate terminal):
   ```powershell
   $env:FIREBASE_AUTH_EMULATOR_HOST = "127.0.0.1:9099"
   go run ./cmd/api
   ```

3. Get a Firebase ID token:
   ```powershell
   .\scripts\get-firebase-token.ps1
   ```
   Then set it:
   ```powershell
   $token = "PASTE_THE_FULL_TOKEN_HERE"
   ```

4. Sync user (run once):
   ```powershell
   Invoke-RestMethod -Method Post `
     -Uri "http://localhost:8000/api/v1/auth/sync" `
     -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
     -Body '{"email":"malithgihana099@gmail.com"}'
   ```

## Testing Steps

### Step 1: Get the Requirements Questions

First, fetch the available questions:

```powershell
Invoke-RestMethod -Method Get `
  -Uri "http://localhost:8000/api/v1/design-input/rag/requirements-questions" `
  -Headers @{ "Authorization" = "Bearer $token" }
```

**Expected Response:**
```json
{
  "ok": true,
  "enabled": true,
  "questions": [
    {
      "id": "server_type",
      "label": "What type of server/workload?",
      "type": "select",
      "options": ["Web API", "gRPC", "Batch/Worker", "Mixed"]
    },
    {
      "id": "expected_users",
      "label": "Expected concurrent users (approximate)?",
      "type": "number",
      "placeholder": "e.g., 1000"
    },
    ...
  ]
}
```

### Step 2: Create a Project and Thread

```powershell
# Create a project
$project = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"name":"Test Requirements Project","is_temporary":false}'

$projectId = $project.project.public_id

# Create a chat thread
$thread = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"title":"Requirements Test Chat","binding_mode":"FOLLOW_LATEST"}'

$threadId = $thread.thread.id
```

### Step 3: Post First Message WITH Requirements Answers

This is the key test - posting the first message with requirements answers:

```powershell
$body = @{
  message = "Help me size my system for production"
  mode = "default"
  requirements_answers = @{
    server_type = "Web API"
    expected_users = 1000
    peak_rps = "200 peak, 50 average"
    latency_target = 150
    read_write_split = "80% read, 20% write, 2KB payload"
    cache_hit_rate = 60
    db_qps_budget = 1000
    critical_flows = "login, checkout, payment"
    availability_target = "99.9% SLA, multi-AZ"
    current_infra = "Intel Xeon, 4 cores, 8GB RAM"
    burst_tolerance = "2x for 5 minutes"
  }
} | ConvertTo-Json -Depth 10

$response = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats/$threadId/messages" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body $body

# Display the response
$response | ConvertTo-Json -Depth 10
```

**What to verify:**
- The LLM response should be more contextual and relevant to the requirements
- The requirements summary is injected into the LLM context (check server logs or LLM request)
- The original message (without summary) is stored in the database

### Step 4: Post First Message WITHOUT Requirements Answers

Test that it works without requirements and adds a note:

```powershell
# Create a new thread for this test
$thread2 = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"title":"No Requirements Test","binding_mode":"FOLLOW_LATEST"}'

$threadId2 = $thread2.thread.id

Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats/$threadId2/messages" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"message":"Hello, can you help me?","mode":"default"}'
```

**What to verify:**
- Chat works normally without requirements
- The message sent to LLM includes "Note: No requirements_answers available."
- No errors occur
- The LLM response may acknowledge that no requirements were provided

### Step 5: Post Second Message (Should NOT Use Requirements)

After the first message, subsequent messages should NOT include requirements summary:

```powershell
Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats/$threadId/messages" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"message":"Can you explain more about that?","mode":"default"}'
```

**What to verify:**
- Requirements summary is NOT injected (only for first message)
- Chat history is used normally

### Step 6: Verify Message Storage

Check that messages are stored correctly:

```powershell
$messages = Invoke-RestMethod -Method Get `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats/$threadId/messages" `
  -Headers @{ "Authorization" = "Bearer $token" }

$messages.messages | Format-Table id, role, content -AutoSize
```

**What to verify:**
- User messages are stored with original content (without requirements summary prepended)
- Assistant messages are stored correctly
- Message order is correct

## Testing Edge Cases

### Test 1: Empty Requirements Answers

```powershell
$body = @{
  message = "Test message"
  requirements_answers = @{}
} | ConvertTo-Json -Depth 10

# Should work normally, no summary injected
```

### Test 2: Partial Requirements Answers

```powershell
$body = @{
  message = "Test message"
  requirements_answers = @{
    server_type = "Web API"
    expected_users = 1000
  }
} | ConvertTo-Json -Depth 10

# Should build summary with only provided answers
```

### Test 3: Disabled Questionnaire

Edit `internal/design_input_processing/rag/questions.yaml`:
```yaml
enabled: false
```

Then test:
```powershell
$response = Invoke-RestMethod -Method Get `
  -Uri "http://localhost:8000/api/v1/design-input/rag/requirements-questions" `
  -Headers @{ "Authorization" = "Bearer $token" }

# Should return: {"ok": true, "enabled": false, "questions": []}
```

### Test 4: Invalid Question IDs

```powershell
$body = @{
  message = "Test message"
  requirements_answers = @{
    invalid_id = "some value"
    another_invalid = 123
  }
} | ConvertTo-Json -Depth 10

# Should still work, invalid IDs are ignored in summary
```

## Verifying Requirements Summary Injection

To verify the requirements summary is actually being injected into the LLM request:

1. Check server logs - the LLM request should show the prepended summary
2. Look for console output like:
   ```
   [UIGP] Body: {"message":"User requirements: server_type=Web API, expected_users=1000, ...\n\nHelp me size my system",...}
   ```

## Editing Questions

To modify the questions:

1. Edit `internal/design_input_processing/rag/questions.yaml`
2. Restart the API server (or call `ReloadQuestions()` if implemented)
3. Test the GET endpoint again to see updated questions

## Troubleshooting

### Questions file not found
- Ensure you're running from the project root (`go run ./cmd/api`)
- Check that `internal/design_input_processing/rag/questions.yaml` exists
- Verify file path resolution in logs

### Requirements summary not appearing
- Verify thread has no chat history (first message only)
- Check that `requirements_answers` is not empty
- Check server logs for errors

### YAML parsing errors
- Validate YAML syntax using an online validator
- Check indentation (YAML is sensitive to spaces)
- Ensure all required fields (id, label, type) are present
