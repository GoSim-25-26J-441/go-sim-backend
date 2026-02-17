# Testing the Design Questionnaire Feature

## Overview

The Design Questionnaire feature allows you to gather design input (preferred vCPU, memory, concurrent users, budget) **before** the first LLM chat call. This only activates when a thread has no chat history.

**Design structure:**
```json
{
  "preferred_vcpu": 4,
  "preferred_memory_gb": 8,
  "workload": { "concurrent_users": 1000 },
  "budget": 2000
}
```

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

### Step 1: Get the Design Questions

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
    { "id": "preferred_vcpu", "label": "Preferred vCPU count?", "type": "number", "placeholder": "e.g., 4" },
    { "id": "preferred_memory_gb", "label": "Preferred memory (GB)?", "type": "number", "placeholder": "e.g., 8" },
    { "id": "concurrent_users", "label": "Expected concurrent users?", "type": "number", "placeholder": "e.g., 1000" },
    { "id": "budget", "label": "Budget (e.g. USD per month)?", "type": "number", "placeholder": "e.g., 2000" }
  ]
}
```

### Step 2: Create a Project and Thread

```powershell
# Create a project
$project = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"name":"Test Design Project","is_temporary":false}'

$projectId = $project.project.public_id

# Create a chat thread
$thread = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"title":"Design Test Chat","binding_mode":"FOLLOW_LATEST"}'

$threadId = $thread.thread.id
```

### Step 3: Post First Message WITH Design

This is the key test - posting the first message with design input:

```powershell
$body = @{
  message = "Help me size my system for production"
  mode = "default"
  design = @{
    preferred_vcpu = 4
    preferred_memory_gb = 8
    workload = @{ concurrent_users = 1000 }
    budget = 2000
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
- The LLM response should be more contextual and relevant to the design
- The design summary is injected into the LLM context (check server logs or LLM request)
- The original message (without summary) is stored in the database
- Summary format: "Design: preferred_vcpu=4, preferred_memory_gb=8, workload.concurrent_users=1000, budget=2000"

### Step 4: Post First Message WITHOUT Design

Test that it works without design and adds a note:

```powershell
# Create a new thread for this test
$thread2 = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"title":"No Design Test","binding_mode":"FOLLOW_LATEST"}'

$threadId2 = $thread2.thread.id

Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats/$threadId2/messages" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"message":"Hello, can you help me?","mode":"default"}'
```

**What to verify:**
- Chat works normally without design
- The message sent to LLM includes "Note: No design available."
- No errors occur
- The LLM response may acknowledge that no design was provided

### Step 5: Post Second Message (Should NOT Use Design)

After the first message, subsequent messages should NOT include design summary:

```powershell
Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8000/api/v1/projects/$projectId/chats/$threadId/messages" `
  -Headers @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" } `
  -Body '{"message":"Can you explain more about that?","mode":"default"}'
```

**What to verify:**
- Design summary is NOT injected (only for first message)
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
- User messages are stored with original content (without design summary prepended)
- Assistant messages are stored correctly
- Message order is correct

## Testing Edge Cases

### Test 1: Empty Design

```powershell
$body = @{
  message = "Test message"
  design = @{}
} | ConvertTo-Json -Depth 10

# Should work normally, no summary injected
```

### Test 2: Partial Design

```powershell
$body = @{
  message = "Test message"
  design = @{
    preferred_vcpu = 4
    preferred_memory_gb = 8
  }
} | ConvertTo-Json -Depth 10

# Should build summary with only provided fields
```

### Test 3: Full Design with Nested Workload

```powershell
$body = @{
  message = "Test message"
  design = @{
    preferred_vcpu = 4
    preferred_memory_gb = 8
    workload = @{ concurrent_users = 1000 }
    budget = 2000
  }
} | ConvertTo-Json -Depth 10

# Should include workload.concurrent_users in summary
```

### Test 4: Disabled Questionnaire

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

### Test 5: Extra Fields in Design

```powershell
$body = @{
  message = "Test message"
  design = @{
    preferred_vcpu = 4
    preferred_memory_gb = 8
    workload = @{ concurrent_users = 1000 }
    budget = 2000
    extra_field = "ignored"
  }
} | ConvertTo-Json -Depth 10

# Should work, extra fields are included in summary
```

## Verifying Design Summary Injection

To verify the design summary is actually being injected into the LLM request:

1. Check server logs - the LLM request should show the prepended summary
2. Look for console output like:
   ```
   [UIGP] Body: {"message":"Design: preferred_vcpu=4, preferred_memory_gb=8, workload.concurrent_users=1000, budget=2000\n\nHelp me size my system",...}
   ```

## Editing Questions

To modify the questions:

1. Edit `internal/design_input_processing/rag/questions.yaml`
2. Restart the API server (or call `ReloadQuestions()` if implemented)
3. Test the GET endpoint again to see updated questions
4. Ensure question IDs map to the design structure: preferred_vcpu, preferred_memory_gb, concurrent_users (-> workload.concurrent_users), budget

## Troubleshooting

### Questions file not found
- Ensure you're running from the project root (`go run ./cmd/api`)
- Check that `internal/design_input_processing/rag/questions.yaml` exists
- Verify file path resolution in logs

### Design summary not appearing
- Verify thread has no chat history (first message only)
- Check that `design` is not empty
- Ensure workload.concurrent_users is nested: `workload: { concurrent_users: 1000 }`
- Check server logs for errors

### YAML parsing errors
- Validate YAML syntax using an online validator
- Check indentation (YAML is sensitive to spaces)
- Ensure all required fields (id, label, type) are present
