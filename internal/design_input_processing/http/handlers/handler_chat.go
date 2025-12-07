package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	diprag "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/rag"
	"github.com/gin-gonic/gin"
)

type missing struct {
	Key string `json:"key"`
	Q   string `json:"question"`
}

type chatReq struct {
	Mode    string `json:"mode"`
	Message string `json:"message"`
	Stream  bool   `json:"stream"`
}

type chatTurn struct {
	Role   string   `json:"role"`
	Text   string   `json:"text"`
	Ts     int64    `json:"ts"`
	Source string   `json:"source,omitempty"`
	Refs   []string `json:"refs,omitempty"`
	UserID string   `json:"user_id,omitempty"`
}

var allowKeywords = []string{
	"microservice", "service", "api", "grpc", "rest", "gateway",
	"throughput", "rps", "latency", "p95", "p99",
	"queue", "kafka", "topic", "db", "replica", "cache",
	"cpu", "memory", "autoscale", "deployment", "container",
	"circuit", "retry", "timeout", "load balancer", "rate limit",
	"uml", "plantuml", "draw.io", "diagram",
}

func Chat(c *gin.Context, upstreamURL, ollamaURL string) {
	// 1) Parse JSON (robust)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"ok": false, "error": "read body: " + err.Error()})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))

	var req chatReq
	if err := json.Unmarshal(raw, &req); err != nil || strings.TrimSpace(req.Message) == "" {
		c.JSON(400, gin.H{"ok": false, "error": "invalid body", "raw": string(raw)})
		return
	}

	jobID := c.Param("id")
	strict := strings.EqualFold(req.Mode, "design-guardrails")

	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if strings.TrimSpace(userID) == "" {
		userID = "demo-user"
	}

	// 2) Optional strict guardrails
	if strict && !onTopic(req.Message) {
		guardMsg := "Let’s keep it on software architecture (microservices, APIs, sizing, latency, etc.)."
		appendChat(jobID, chatTurn{
			Role:   "assistant",
			Text:   guardMsg,
			Ts:     time.Now().Unix(),
			Source: "guardrails",
			UserID: userID,
		})
		c.JSON(200, gin.H{
			"ok":     true,
			"answer": guardMsg,
			"source": "guardrails",
		})
		return
	}

	// Log the user message
	appendChat(jobID, chatTurn{
		Role:   "user",
		Text:   req.Message,
		Ts:     time.Now().Unix(),
		Source: "user",
		UserID: userID,
	})

	// 3) Try RAG first
	if ans, refs := ragAnswer(req.Message); ans != "" && !looksLikeTitle(ans) {
		appendChat(jobID, chatTurn{
			Role:   "assistant",
			Text:   ans,
			Ts:     time.Now().Unix(),
			Source: "rag",
			Refs:   refs,
			UserID: userID,
		})
		c.JSON(200, gin.H{
			"ok":     true,
			"answer": ans,
			"source": "rag",
			"refs":   refs,
		})
		return
	}

	// 4) Fetch compact context
	ig, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/intermediate", upstreamURL, jobID), 10*time.Second)
	spec, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/export?format=json&download=false", upstreamURL, jobID), 10*time.Second)
	features := compactContext(ig, spec, req.Message)
	log.Printf("[chat] job=%s features:\n%s", jobID, features)

	// 5) Call local Ollama
	body := map[string]any{
		"model":  "llama3:instruct",
		"format": "json",
		"stream": false,
		"system": `You are a software architecture assistant.

You are given:
- Services and their names.
- Dependencies in the form "from -> to (protocol)".
- Gaps such as RPS and protocol certainty.

Rules:
- Always respect the protocol from the context (if it says gRPC, do NOT call it REST).
- Do NOT invent extra services or components that are not in the context.
- If there is only one or two services, describe it as a very small microservice-like setup that is effectively
  monolithic for now, and explicitly say that it does NOT really benefit from microservice-style decomposition yet.
- You may suggest how it could be evolved into a better microservice architecture (e.g., more domain-focused services,
  clearer boundaries, separate datastores).
- Be concise, stay on topic.

Return JSON: {"answer": string}.`,
		"prompt": fmt.Sprintf("Context:\n%s\n\nQuestion:\n%s\n\nReturn only the JSON.", features, req.Message),
		"options": map[string]any{
			"temperature": 0.0,
			"num_ctx":     1024,
			"num_predict": 512,
		},
	}

	respBytes, err := postJSON(ollamaURL+"/api/generate", body, 60*time.Second)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "ollama: " + err.Error()})
		return
	}

	// 6) Decode Ollama response
	var gen struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBytes, &gen); err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "decode: " + err.Error()})
		return
	}

	answerText := ""
	var out struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(gen.Response), &out); err == nil && out.Answer != "" {
		answerText = out.Answer
	} else {
		answerText = gen.Response
	}

	// Fix protocol wording if needed
	answerText = enforceProtocol(answerText, ig, spec)

	appendChat(jobID, chatTurn{
		Role:   "assistant",
		Text:   answerText,
		Ts:     time.Now().Unix(),
		Source: "llm",
		UserID: userID,
	})

	c.JSON(200, gin.H{
		"ok":     true,
		"answer": answerText,
		"source": "llm",
	})
}

func isArchitectureQuestion(s string) bool {
	keys := []string{"service", "api", "grpc", "rest", "queue", "topic", "database", "latency", "rps", "throughput", "cache", "retry", "circuit"}
	for _, k := range keys {
		if containsCI(s, k) {
			return true
		}
	}
	return false
}

func containsCI(hay, needle string) bool {
	return len(hay) >= len(needle) && bytes.Contains(bytes.ToLower([]byte(hay)), bytes.ToLower([]byte(needle)))
}

func fetchJSON(url string, to time.Duration) (map[string]any, error) {
	cl := &http.Client{Timeout: to}
	r, err := cl.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	var m map[string]any
	return m, json.NewDecoder(r.Body).Decode(&m)
}

func postJSON(url string, body any, to time.Duration) ([]byte, error) {
	b, _ := json.Marshal(body)
	rq, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	rq.Header.Set("Content-Type", "application/json")
	cl := &http.Client{Timeout: to}
	rs, err := cl.Do(rq)
	if err != nil {
		return nil, err
	}
	defer rs.Body.Close()
	return io.ReadAll(rs.Body)
}

func compactContext(ig, spec map[string]any, msg string) string {
	// 1) Prefer spec.services; fall back to IG nodes
	services := tryArrayNames(spec, "services", "name")
	if len(services) == 0 {
		services = tryArrayNames(ig, "Nodes", "Label")
	}

	// 2) Prefer spec.dependencies; fall back to IG edges
	edges := tryDeps(spec)
	if len(edges) == 0 {
		edges = tryEdges(ig)
	}

	// 3) Gaps from spec (e.g., RPS, protocol certainty, etc.)
	gaps := tryGaps(spec)

	var b strings.Builder

	b.WriteString("Services:\n")
	for _, s := range services {
		fmt.Fprintf(&b, "- %s\n", s)
	}

	b.WriteString("\nDependencies:\n")
	if len(edges) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, e := range edges {
			from, to, proto := e[0], e[1], strings.ToLower(e[2])
			if proto == "" {
				proto = "unknown"
			}
			fmt.Fprintf(&b, "- %s -> %s (%s)\n", from, to, proto)
		}
	}

	b.WriteString("\nGaps:\n")
	if len(gaps) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, g := range gaps {
			fmt.Fprintf(&b, "- %s = %s\n", g[0], g[1])
		}
	}

	b.WriteString("\nUser question:\n")
	b.WriteString(msg)

	return b.String()
}

func enforceProtocol(answer string, ig, spec map[string]any) string {
	if answer == "" {
		return answer
	}

	// Collect protocols from spec.dependencies first, then IG edges
	edges := tryDeps(spec)
	if len(edges) == 0 {
		edges = tryEdges(ig)
	}

	protos := map[string]bool{}
	for _, e := range edges {
		p := strings.ToLower(e[2]) // e[2] is protocol
		if p == "" {
			continue
		}
		protos[p] = true
	}
	if len(protos) != 1 {
		// Multiple or none – don't try to "fix" text
		return answer
	}

	var only string
	for p := range protos {
		only = p
	}

	lower := strings.ToLower(answer)

	switch only {
	case "grpc":
		if strings.Contains(lower, "rest") && !strings.Contains(lower, "grpc") {
			answer = strings.ReplaceAll(answer, "REST", "gRPC")
			answer = strings.ReplaceAll(answer, "Rest", "gRPC")
			answer = strings.ReplaceAll(answer, "rest", "gRPC")
		}
	case "rest":
		if strings.Contains(lower, "grpc") && !strings.Contains(lower, "rest") {
			answer = strings.ReplaceAll(answer, "gRPC", "REST")
			answer = strings.ReplaceAll(answer, "grpc", "REST")
		}
	}

	return answer
}

func tryArrayNames(m map[string]any, key, sub string) []string {
	arr, _ := m[key].([]any)
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if obj, ok := v.(map[string]any); ok {
			if s, _ := obj[sub].(string); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func appendChat(jobID string, turn chatTurn) {
	baseDir := chatBaseDir(turn.UserID)

	_ = os.MkdirAll(baseDir, 0o755)

	fpath := filepath.Join(baseDir, fmt.Sprintf("chat-%s.jsonl", jobID))
	f, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	b, _ := json.Marshal(turn)
	_, _ = f.Write(append(b, '\n'))
}

func ListJobsForUser(userID string) ([]string, error) {
	dir := chatBaseDir(userID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var jobIDs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name() // chat-<job>.jsonl
		if !strings.HasPrefix(name, "chat-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "chat-"), ".jsonl")
		jobIDs = append(jobIDs, id)
	}
	return jobIDs, nil
}

func intFromMap(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func ragAnswer(msg string) (string, []string) {
	results := diprag.Search(msg)
	if len(results) == 0 {
		return "", nil
	}

	var b strings.Builder
	refs := make([]string, 0, 2)
	for i, r := range results {
		if i > 1 {
			break
		}
		b.WriteString(r.Snippet)
		if !strings.HasSuffix(r.Snippet, "\n") {
			b.WriteString("\n")
		}
		base := r.ID
		if slash := strings.LastIndexAny(base, `/\`); slash >= 0 {
			base = base[slash+1:]
		}
		refs = append(refs, base)
	}
	return strings.TrimSpace(b.String()), refs
}

func onTopic(msg string) bool {
	m := strings.ToLower(msg)
	hits := 0
	for _, kw := range allowKeywords {
		if strings.Contains(m, kw) {
			hits++
		}
	}
	return hits >= 1
}

func looksLikeTitle(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}

	// Consider only the first line
	lines := strings.SplitN(s, "\n", 2)
	first := strings.TrimSpace(lines[0])

	// Very short, no sentence punctuation → looks like a heading such as "Sizing prompts..."
	if len(first) < 25 && !strings.ContainsAny(first, ".!?") {
		return true
	}

	return false
}

func tryGaps(m map[string]any) [][2]string {
	arr, _ := m["gaps"].([]any)
	out := make([][2]string, 0, len(arr))
	for _, v := range arr {
		if obj, ok := v.(map[string]any); ok {
			k, _ := obj["key"].(string)
			val, _ := obj["value"].(string)
			if k != "" && val != "" {
				out = append(out, [2]string{k, val})
			}
		}
	}
	return out
}

// Each triple is: [from, to, protocol]
func tryDeps(spec map[string]any) [][3]string {
	arr, _ := spec["dependencies"].([]any)
	out := make([][3]string, 0, len(arr))
	for _, v := range arr {
		if obj, ok := v.(map[string]any); ok {
			from, _ := obj["from"].(string)
			to, _ := obj["to"].(string)
			kind, _ := obj["kind"].(string) // "grpc", "rest", "pubsub", etc.
			out = append(out, [3]string{from, to, kind})
		}
	}
	return out
}

func tryEdges(m map[string]any) [][3]string {
	arr, _ := m["Edges"].([]any)
	out := make([][3]string, 0, len(arr))
	for _, v := range arr {
		if obj, ok := v.(map[string]any); ok {
			from, _ := obj["From"].(string)
			to, _ := obj["To"].(string)
			proto, _ := obj["Protocol"].(string)
			out = append(out, [3]string{from, to, proto})
		}
	}
	return out
}
