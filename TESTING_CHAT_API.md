# Chat API Testing Guide

This guide explains how to test the new chat API integration with the LLM service.

## Prerequisites

1. **LLM Service Running**: Ensure the UIGP LLM service is running at `http://localhost:8081`
2. **API Key**: Set `LLM_API_KEY=dev-key` in your `.env` file (or use the key configured in your LLM service)
3. **Database**: Ensure PostgreSQL is running and migrations are applied
4. **Firebase Auth**: Configure Firebase credentials if authentication is required

## Environment Setup

Add to your `.env` file:
```env
LLM_SVC_URL=http://localhost:8081
LLM_API_KEY=dev-key
```

## API Endpoints

### Base URL
```
http://localhost:8000/api/v1/projects
```

### Authentication
All endpoints require Firebase authentication. Include the Firebase ID token in the `Authorization` header:
```
Authorization: Bearer <firebase-token>
```

## Testing Workflow

### 1. Create a Project (if needed)

```bash
POST /api/v1/projects
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "Test Project",
  "is_temporary": false
}
```

**Response:**
```json
{
  "ok": true,
  "project": {
    "public_id": "proj_xxxxx",
    "name": "Test Project",
    ...
  }
}
```

Save the `public_id` for subsequent requests.

### 2. Create a Chat Thread

```bash
POST /api/v1/projects/{public_id}/chats
Content-Type: application/json
Authorization: Bearer <token>

{
  "title": "My Chat Thread",
  "binding_mode": "FOLLOW_LATEST"
}
```

**Response:**
```json
{
  "ok": true,
  "thread": {
    "id": "thr_xxxxx",
    "title": "My Chat Thread",
    ...
  }
}
```

Save the `thread_id` for message requests.

### 3. Send a Message (Basic)

```bash
POST /api/v1/projects/{public_id}/chats/{thread_id}/messages
Content-Type: application/json
Authorization: Bearer <token>

{
  "message": "How do I reduce p95 latency in microservices?",
  "mode": "instant"
}
```

**Response:**
```json
{
  "ok": true,
  "answer": "Here are some strategies to reduce p95 latency...",
  "source": "ollama/llama3:instruct",
  "refs": [],
  "diagram_version_id_used": null,
  "user_message": {
    "id": "msg_xxxxx",
    "role": "user",
    "content": "How do I reduce p95 latency in microservices?",
    ...
  },
  "assistant_message": {
    "id": "msg_yyyyy",
    "role": "assistant",
    "content": "Here are some strategies...",
    ...
  }
}
```

### 4. Send a Message with Detail Level

```bash
POST /api/v1/projects/{public_id}/chats/{thread_id}/messages
Content-Type: application/json
Authorization: Bearer <token>

{
  "message": "Deeply analyze p95 latency causes in microservices",
  "mode": "thinking",
  "detail": "high"
}
```

**Available detail levels:**
- `"low"` - Quick, concise responses
- `"medium"` - Balanced detail
- `"high"` - Detailed, comprehensive responses

**Available modes:**
- `"instant"` - Fast response
- `"thinking"` - More thorough analysis
- `"auto"` - Let the LLM decide

### 5. Send a Message with Attachments

```bash
POST /api/v1/projects/{public_id}/chats/{thread_id}/messages
Content-Type: application/json
Authorization: Bearer <token>

{
  "message": "Check if the attached file implies any missing components",
  "attachments": [
    {
      "object_key": "s3://bucket/arch-diagram.png",
      "mime_type": "image/png",
      "file_name": "architecture.png",
      "file_size_bytes": 245120,
      "width": 1920,
      "height": 1080
    }
  ]
}
```

### 6. Send a Message with Diagram Context

First, create a diagram version for your project:

```bash
POST /api/v1/projects/{public_id}/diagram
Content-Type: application/json
Authorization: Bearer <token>

{
  "diagram_json": {
    "metadata": {
      "diagram_version_id": "dv_0004"
    },
    "nodes": [
      {"id": "api", "label": "api-gateway", "type": "service"},
      {"id": "u", "label": "user-service", "type": "service"},
      {"id": "db", "label": "postgres", "type": "db"}
    ],
    "edges": [
      {"from": "api", "to": "u", "protocol": "REST"},
      {"from": "u", "to": "db", "protocol": "SQL"}
    ]
  },
  "spec_summary": {
    "services": ["api-gateway", "user-service", "postgres"],
    "dependencies": [
      "api-gateway->user-service(rest)",
      "user-service->postgres(db)"
    ]
  }
}
```

Then send a message - the diagram context will be automatically included:

```bash
POST /api/v1/projects/{public_id}/chats/{thread_id}/messages
Content-Type: application/json
Authorization: Bearer <token>

{
  "message": "Spot missing dependencies or gaps in this architecture"
}
```

### 7. Test Chat History (Conversation Continuity)

Send multiple messages in the same thread to test history:

**Message 1:**
```bash
POST /api/v1/projects/{public_id}/chats/{thread_id}/messages
{
  "message": "We have 12 services, REST calls, Postgres, and Kafka."
}
```

**Message 2:**
```bash
POST /api/v1/projects/{public_id}/chats/{thread_id}/messages
{
  "message": "What should I improve?"
}
```

The second message should reference the first message, demonstrating that chat history is being passed to the LLM.

### 8. List Messages (View Chat History)

```bash
GET /api/v1/projects/{public_id}/chats/{thread_id}/messages
Authorization: Bearer <token>
```

**Response:**
```json
{
  "ok": true,
  "messages": [
    {
      "id": "msg_xxxxx",
      "role": "user",
      "content": "How do I reduce p95 latency?",
      "created_at": "2025-02-09T10:00:00Z",
      ...
    },
    {
      "id": "msg_yyyyy",
      "role": "assistant",
      "content": "Here are some strategies...",
      "source": "ollama/llama3:instruct",
      "created_at": "2025-02-09T10:00:01Z",
      ...
    }
  ]
}
```

### 9. List Threads

```bash
GET /api/v1/projects/{public_id}/chats
Authorization: Bearer <token>
```

## PowerShell Testing Script

Create a file `test-chat-api.ps1`:

```powershell
$ErrorActionPreference = "Stop"

$BASE = "http://localhost:8000"
$TOKEN = "your-firebase-token-here"  # Replace with actual token
$H = @{
    "Authorization" = "Bearer $TOKEN"
    "Content-Type" = "application/json"
}

function Pretty($obj, $depth=50) {
    $obj | ConvertTo-Json -Depth $depth
}

# Replace these with actual values from your project
$PROJECT_ID = "proj_xxxxx"
$THREAD_ID = "thr_xxxxx"

Write-Host "`n--- POST /api/v1/projects/$PROJECT_ID/chats/$THREAD_ID/messages (basic) ---"
$body = @{
    message = "How do I reduce p95 latency in microservices?"
    mode = "instant"
} | ConvertTo-Json -Depth 20

try {
    $response = Invoke-RestMethod -Method Post -Uri "$BASE/api/v1/projects/$PROJECT_ID/chats/$THREAD_ID/messages" -Headers $H -Body $body
    Pretty $response 50
} catch {
    Write-Host "Error: $($_.Exception.Message)"
    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $responseBody = $reader.ReadToEnd()
        Write-Host "Response: $responseBody"
    }
}

Write-Host "`n--- POST /api/v1/projects/$PROJECT_ID/chats/$THREAD_ID/messages (with detail) ---"
$body = @{
    message = "Deeply analyze p95 latency causes"
    mode = "thinking"
    detail = "high"
} | ConvertTo-Json -Depth 20

try {
    $response = Invoke-RestMethod -Method Post -Uri "$BASE/api/v1/projects/$PROJECT_ID/chats/$THREAD_ID/messages" -Headers $H -Body $body
    Pretty $response 50
} catch {
    Write-Host "Error: $($_.Exception.Message)"
}

Write-Host "`n--- GET /api/v1/projects/$PROJECT_ID/chats/$THREAD_ID/messages ---"
try {
    $response = Invoke-RestMethod -Method Get -Uri "$BASE/api/v1/projects/$PROJECT_ID/chats/$THREAD_ID/messages" -Headers $H
    Pretty $response 50
} catch {
    Write-Host "Error: $($_.Exception.Message)"
}
```

## cURL Examples

### Basic Message
```bash
curl -X POST "http://localhost:8000/api/v1/projects/proj_xxxxx/chats/thr_xxxxx/messages" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "How do I reduce p95 latency?",
    "mode": "instant"
  }'
```

### With Detail
```bash
curl -X POST "http://localhost:8000/api/v1/projects/proj_xxxxx/chats/thr_xxxxx/messages" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Deeply analyze p95 latency causes",
    "mode": "thinking",
    "detail": "high"
  }'
```

### With Attachments
```bash
curl -X POST "http://localhost:8000/api/v1/projects/proj_xxxxx/chats/thr_xxxxx/messages" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Review this architecture",
    "attachments": [
      {
        "object_key": "s3://bucket/diagram.png",
        "mime_type": "image/png",
        "file_name": "diagram.png",
        "file_size_bytes": 245120
      }
    ]
  }'
```

## Expected Behavior

1. **Chat History**: When you send multiple messages in the same thread, the LLM should reference previous messages
2. **Diagram Context**: If a project has a diagram version, it's automatically included in LLM requests
3. **Detail Levels**: Higher detail levels should produce more comprehensive responses
4. **Modes**: 
   - `instant` mode should be faster
   - `thinking` mode should be more thorough
5. **Storage**: All messages (user and assistant) are stored in the database and can be retrieved via GET messages endpoint

## Troubleshooting

### 401 Unauthorized
- Check that Firebase token is valid and included in Authorization header
- Verify Firebase credentials are configured

### 502 Bad Gateway
- Check that LLM service is running at `http://localhost:8081`
- Verify `LLM_API_KEY` matches the key expected by LLM service
- Check LLM service logs for errors

### 400 Bad Request
- Verify message field is not empty
- Check JSON format is valid
- Ensure required fields are present

### Empty Responses
- Check LLM service is responding correctly
- Verify API key is correct
- Check network connectivity between services

## Verification Checklist

- [ ] LLM service is running and accessible
- [ ] API key is configured correctly
- [ ] Database connection is working
- [ ] Firebase authentication is configured
- [ ] Can create a project
- [ ] Can create a chat thread
- [ ] Can send a message and receive LLM response
- [ ] Chat history is stored and retrieved correctly
- [ ] Multiple messages in same thread show conversation continuity
- [ ] Detail levels work as expected
- [ ] Attachments are handled correctly
- [ ] Diagram context is included when available
