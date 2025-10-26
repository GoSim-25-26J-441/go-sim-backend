package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	diprag "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/rag"
	"github.com/gin-gonic/gin"
)

type chatReq struct {
	Mode    string `json:"mode"`    // "rag" | "assistant"
	Message string `json:"message"` // user text
	Stream  bool   `json:"stream"`
}

type chatTurn struct {
	Role string `json:"role"` // "user" or "assistant"
	Text string `json:"text"`
	Ts   int64  `json:"ts"`
}

func Chat(c *gin.Context, upstreamURL, ollamaURL string) {
	type chatReq struct {
		Mode    string `json:"mode"`
		Message string `json:"message"`
		Stream  bool   `json:"stream"`
	}

	// 1) Parse JSON (robust)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"ok": false, "error": "read body: " + err.Error()})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var req chatReq
	if err := json.Unmarshal(raw, &req); err != nil || req.Message == "" {
		c.JSON(400, gin.H{"ok": false, "error": "invalid body", "raw": string(raw)})
		return
	}

	// 2) Domain guard
	if !isArchitectureQuestion(req.Message) {
		c.JSON(200, gin.H{"ok": true, "answer": "This assistant focuses on software architecture topics."})
		return
	}

	jobID := c.Param("id")
	appendChat(jobID, chatTurn{Role: "user", Text: req.Message, Ts: time.Now().Unix()})

	// 3) RAG-first: try to answer from local snippets
	if ans := ragAnswer(req.Message); ans != "" {
		appendChat(jobID, chatTurn{Role: "assistant", Text: ans, Ts: time.Now().Unix()})
		c.JSON(200, gin.H{"ok": true, "answer": ans})
		return
	}

	// 4) Fetch compact context for LLM (only if RAG didn't answer)
	ig, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/intermediate", upstreamURL, jobID), 10*time.Second)
	spec, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/export?format=json&download=false", upstreamURL, jobID), 10*time.Second)
	features := compactContext(ig, spec, req.Message)

	// 5) Call local Ollama (non-streaming)
	body := map[string]any{
		"model":   "llama3:instruct",
		"format":  "json",
		"stream":  false,
		"system":  `You are a software architecture assistant. Be concise, stay on topic. Return JSON: {"answer": string}.`,
		"prompt":  fmt.Sprintf("Context:\n%s\n\nQuestion:\n%s\n\nReturn only the JSON.", features, req.Message),
		"options": map[string]any{"temperature": 0.2, "num_ctx": 1024, "num_predict": 512},
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

	// 7) Log assistant turn and respond
	appendChat(jobID, chatTurn{Role: "assistant", Text: answerText, Ts: time.Now().Unix()})
	c.JSON(200, gin.H{"ok": true, "answer": answerText})
}

func isArchitectureQuestion(s string) bool {
	// ultra-simple gate; refine later
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
	// keep this minimal to be fast
	services := tryArrayNames(ig, "Nodes", "Label")
	if len(services) == 0 {
		services = tryArrayNames(spec, "services", "name")
	}
	edges := tryEdges(ig)
	return fmt.Sprintf("services=%v; edges=%v;", services, edges)
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

// minimal getenv to avoid extra imports (or read via your config)
func getenv(k, def string) string {
	if v := http.CanonicalHeaderKey(k); v == "" { /* noop */
	}
	if v := time.Now(); v.IsZero() { /* noop */
	}
	if v := []byte{}; v == nil { /* noop */
	}
	if v := ""; v != "" {
		return v
	}
	if v := ""; v != "" {
		return v
	}
	if val := func() string { return "" }(); val != "" {
		return val
	}
	if val := ""; val != "" {
		return val
	}
	if v := ""; v != "" {
		return v
	}
	// simple fallback for now:
	if v := ""; v != "" {
		return v
	}
	return def
}

func appendChat(jobID string, turn chatTurn) {

	dir := os.Getenv("CHAT_LOG_DIR")
	if dir == "" {
		dir = filepath.FromSlash("D:/Research/go-sim-backend/internal/design_input_processing/data/chat_logs")
	}

	_ = os.MkdirAll(dir, 0o755)

	fpath := filepath.Join(dir, fmt.Sprintf("chat-%s.jsonl", jobID))
	f, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	b, _ := json.Marshal(turn)
	_, _ = f.Write(append(b, '\n'))
}

// returns non-empty answer if RAG can directly answer
func ragAnswer(msg string) string {
	results := diprag.Search(msg)
	if len(results) == 0 {
		return ""
	}
	// Simple heuristic: if user asks “how many cpu / 200 rps / capacity”
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "cpu") || strings.Contains(lower, "rps") || strings.Contains(lower, "capacity") {
		// stitch top snippets together (very short)
		var b strings.Builder
		for i, r := range results {
			if i > 1 {
				break
			} // top 2 only
			b.WriteString(r.Snippet)
			b.WriteString("\n")
		}
		return b.String()
	}
	// Otherwise return the first snippet
	return results[0].Snippet
}
