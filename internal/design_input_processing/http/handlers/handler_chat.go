package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
}

var rxRPS = regexp.MustCompile(`(?i)\b(\d+)\s*rps\b`)
var rxP95ms = regexp.MustCompile(`(?i)\bp95\b.*?(\d+)\s*ms`)
var rxCPU = regexp.MustCompile(`(?i)\b(\d+)\s*(v?cpu|cores?)\b`)
var rxPayload = regexp.MustCompile(`(?i)(\d+)\s*(kb|mb)\b`)
var rxBurst = regexp.MustCompile(`(?i)\b(\d+)[x×]\b`)

var allowKeywords = []string{
	"microservice", "service", "api", "grpc", "rest", "gateway",
	"throughput", "rps", "latency", "p95", "p99",
	"queue", "kafka", "topic", "db", "replica", "cache",
	"cpu", "memory", "autoscale", "deployment", "container",
	"circuit", "retry", "timeout", "load balancer", "rate limit",
	"uml", "plantuml", "draw.io", "diagram",
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

	jobID := c.Param("id")
	strict := strings.EqualFold(req.Mode, "design-guardrails")

	if strict && !onTopic(req.Message) {
		guardMsg := "Let’s keep it on software architecture (microservices, APIs, sizing, latency, etc.)."
		appendChat(jobID, chatTurn{
			Role:   "assistant",
			Text:   guardMsg,
			Ts:     time.Now().Unix(),
			Source: "guardrails",
		})
		c.JSON(200, gin.H{
			"ok":     true,
			"answer": guardMsg,
			"source": "guardrails",
		})
		return
	}

	// 2) Domain guard
	if !isArchitectureQuestion(req.Message) {
		c.JSON(200, gin.H{"ok": true, "answer": "This assistant focuses on software architecture topics."})
		return
	}

	appendChat(jobID, chatTurn{Role: "user", Text: req.Message, Ts: time.Now().Unix()})

	if ans, refs := ragAnswer(req.Message); ans != "" {
		if miss := findMissingSignals(req.Message); len(miss) > 0 {
			var b strings.Builder
			b.WriteString("To size this accurately, I need:\n")
			for i, it := range miss {
				if i >= 3 {
					break
				} // keep it short (top 3)
				b.WriteString("- ")
				b.WriteString(it.Q)
				b.WriteString("\n")
			}
			ask := strings.TrimSpace(b.String())
			appendChat(jobID, chatTurn{
				Role:   "assistant",
				Text:   ask,
				Ts:     time.Now().Unix(),
				Source: "rag",
				Refs:   refs,
			})
			c.JSON(200, gin.H{
				"ok":      true,
				"answer":  ask,
				"source":  "rag",
				"refs":    refs,
				"missing": miss,
			})
			return
		}

		appendChat(jobID, chatTurn{
			Role:   "assistant",
			Text:   ans,
			Ts:     time.Now().Unix(),
			Source: "rag",
			Refs:   refs,
		})
		c.JSON(200, gin.H{
			"ok":     true,
			"answer": ans,
			"source": "rag",
			"refs":   refs,
		})
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

	appendChat(jobID, chatTurn{
		Role:   "assistant",
		Text:   answerText,
		Ts:     time.Now().Unix(),
		Source: "llm",
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

func ragRefs(msg string) []string {
	results := diprag.Search(msg)
	refs := make([]string, 0, len(results))
	for i, r := range results {
		if i > 1 {
			break
		}
		base := r.ID
		if slash := strings.LastIndexAny(base, `/\`); slash >= 0 {
			base = base[slash+1:]
		}
		refs = append(refs, base)
	}
	return refs
}

func findMissingSignals(msg string) []missing {
	l := strings.ToLower(msg)
	m := []missing{}
	if rxRPS.FindStringSubmatch(l) == nil {
		m = append(m, missing{Key: "rps_peak", Q: "What is peak vs average RPS (traffic pattern)?"})
	}
	if rxP95ms.FindStringSubmatch(l) == nil {
		m = append(m, missing{Key: "latency_p95", Q: "What p95 latency target are you aiming for (ms)?"})
	}
	if rxPayload.FindStringSubmatch(l) == nil {
		m = append(m, missing{Key: "payload", Q: "Typical payload size (KB) and read/write split?"})
	}
	if rxBurst.FindStringSubmatch(l) == nil {
		m = append(m, missing{Key: "burst", Q: "Expected burst (e.g., 2x average) and duration?"})
	}
	if rxCPU.FindStringSubmatch(l) == nil {
		m = append(m, missing{Key: "current_cpu", Q: "Current/target CPU/RAM footprint per instance?"})
	}
	return m
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
