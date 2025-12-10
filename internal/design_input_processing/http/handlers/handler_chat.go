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
	"regexp"
	"strconv"
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
	Message  string `json:"message"`
	Mode     string `json:"mode,omitempty"`
	ForceLLM bool   `json:"force_llm,omitempty"` // override to bypass RAG
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

// Signal regexes
var rxRPS = regexp.MustCompile(`(?i)\b(\d+)\s*rps\b`)
var rxP95ms = regexp.MustCompile(`(?i)\bp95\b.*?(\d+)\s*ms`)
var rxCPU = regexp.MustCompile(`(?i)\b(\d+)\s*(v?cpu|cores?)\b`)
var rxPayload = regexp.MustCompile(`(?i)(\d+)\s*(kb|mb)\b`)
var rxBurst = regexp.MustCompile(`(?i)\b(\d+)[x×]\b`)

func looksLikeDefinition(q string) bool {
	s := strings.ToLower(strings.TrimSpace(q))
	defPhrases := []string{
		"what is ", "what's ", "define ", "definition of ",
		"explain ", "explain about ", "difference between",
		"pros and cons", "advantages and disadvantages",
	}
	for _, p := range defPhrases {
		if strings.HasPrefix(s, p) || strings.Contains(s, " "+p) {
			return true
		}
	}
	return false
}

// Questions clearly about the uploaded diagram / image
func isDiagramQuestion(q string) bool {
	s := strings.ToLower(q)

	// direct mentions of diagrams/images
	if strings.Contains(s, "diagram") ||
		strings.Contains(s, "picture") ||
		strings.Contains(s, "image") ||
		strings.Contains(s, "drawing") {
		return true
	}

	// user talking about the uploaded artifact
	if strings.Contains(s, "what i upload") ||
		strings.Contains(s, "what i uploaded") ||
		strings.Contains(s, "in what i upload") ||
		strings.Contains(s, "in what i uploaded") {
		return true
	}

	// common phrasing
	if strings.Contains(s, "my services") ||
		strings.Contains(s, "my edges") ||
		strings.Contains(s, "services and edges") {
		return true
	}

	return false
}

// Questions where RAG snippets (throughput formula / sizing cheat sheet) are useful
func isRAGCandidate(q string) bool {
	s := strings.ToLower(q)

	ragKeywords := []string{
		"throughput", "through put",
		"concurrency",
		"throughput formula", "sizing formula",
		"back-of-the-envelope", "back of the envelope",
		"capacity planning",
		"cpu sizing", "cpu core", "cpu cores", "vcpu",
		"headroom",
		"autoscale", "auto scale",
		"rate limit", "rate limiting",
		"timeouts", "retries",
		"rps @", // e.g. "200 rps @ 150ms"
		"p95", "p99",
		"slo", "sla",
		"sizing guide",
	}

	for _, kw := range ragKeywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// --- Main Chat handler ---

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

	// Resolve user id (for per-user chat logs)
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
	}
	if strings.TrimSpace(userID) == "" {
		userID = "demo-user"
	}

	// --- Aggregate signals from history + this message ---
	historySignals := loadSignalsFromHistory(jobID, userID)
	thisSignals := extractSignals(req.Message)
	signals := mergeSignals(historySignals, thisSignals)

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
			"ok":      true,
			"answer":  guardMsg,
			"source":  "guardrails",
			"signals": signals,
		})
		return
	}

	// 3) Log the user message
	appendChat(jobID, chatTurn{
		Role:   "user",
		Text:   req.Message,
		Ts:     time.Now().Unix(),
		Source: "user",
		UserID: userID,
	})

	// ---- Intent detection ----
	lowerMsg := strings.ToLower(req.Message)
	isSizing := strings.Contains(lowerMsg, "size") ||
		strings.Contains(lowerMsg, "sizing") ||
		strings.Contains(lowerMsg, "capacity") ||
		strings.Contains(lowerMsg, "scale") ||
		strings.Contains(lowerMsg, "rps") ||
		strings.Contains(lowerMsg, "latency")

	isDef := looksLikeDefinition(req.Message)
	isJob := isJobSpecific(req.Message)
	isDiag := isDiagramQuestion(req.Message)

	// If it's a sizing-style question and we DON'T yet have all signals → ask for them
	missing := findMissingSignals(signals)
	if isSizing && len(missing) > 0 {
		var b strings.Builder
		b.WriteString("To size this accurately, I need:\n")
		for i, m := range missing {
			if i >= 3 {
				break // keep prompt short
			}
			b.WriteString("- ")
			b.WriteString(m.Q)
			b.WriteString("\n")
		}
		ask := strings.TrimSpace(b.String())

		appendChat(jobID, chatTurn{
			Role:   "assistant",
			Text:   ask,
			Ts:     time.Now().Unix(),
			Source: "sizing-prompts",
			UserID: userID,
		})

		c.JSON(200, gin.H{
			"ok":      true,
			"answer":  ask,
			"source":  "sizing-prompts",
			"missing": missing,
			"signals": signals,
		})
		return
	}

	// If it's a sizing Q and we ALREADY have all signals → skip RAG, go straight to LLM
	skipRAG := isSizing && len(missing) == 0

	// Decide whether to try RAG at all
	tryRAG := !skipRAG && !req.ForceLLM && !isDef && !isJob && !isDiag

	// 4) Try RAG first (unless disabled)
	if tryRAG {
		if ans, refs := ragAnswer(req.Message); ans != "" {
			appendChat(jobID, chatTurn{
				Role:   "assistant",
				Text:   ans,
				Ts:     time.Now().Unix(),
				Source: "rag",
				Refs:   refs,
				UserID: userID,
			})
			c.JSON(200, gin.H{
				"ok":      true,
				"answer":  ans,
				"source":  "rag",
				"refs":    refs,
				"signals": signals,
			})
			return
		}
	}

	// 5) Fetch compact context for LLM
	ig, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/intermediate", upstreamURL, jobID), 10*time.Second)
	spec, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/export?format=json&download=false", upstreamURL, jobID), 10*time.Second)
	features := compactContext(ig, spec, req.Message)
	log.Printf("[chat] job=%s features:\n%s", jobID, features)

	signalsJSON, _ := json.Marshal(signals)

	// 6) Call local Ollama (non-streaming)
	body := map[string]any{
		"model":  "llama3:instruct",
		"stream": false,
		"system": `You are a software architecture assistant.

You are given:
- Services and their names.
- Dependencies in the form "from -> to (protocol)".
- Gaps such as RPS and protocol certainty.

Rules:
- Always respect the protocol from the context (if it says gRPC, do NOT call it REST).
- Do NOT invent extra services or components that are not in the context.
- If there is only one or two services, describe it as a very small microservice-like setup
  that is effectively monolithic for now, and explicitly say that it does NOT really
  benefit from microservice-style decomposition yet.
- You may suggest how it could be evolved into a better microservice architecture
  (e.g., more domain-focused services, clearer boundaries, separate datastores).
- Be concise, stay on topic.
- When you are given numeric sizing signals such as rps_peak, rps_avg, latency_p95_ms,
  payload_kb, burst_factor, cpu_vcpu, you MUST produce a concrete sizing recommendation
  (e.g., instances, vCPU per instance, rough RAM per service) instead of saying that
  you need more information.

Answer in plain English, not JSON.`,
		"prompt": fmt.Sprintf(
			"Context:\n%s\n\nSignals:\n%s\n\nQuestion:\n%s\n\nAnswer in plain English.",
			features,
			string(signalsJSON),
			req.Message,
		),
		"options": map[string]any{
			"temperature": 0.0,
			"num_ctx":     1024,
			"num_predict": 512,
		},
	}

	respBytes, err := postJSON(ollamaURL+"/api/generate", body, 3*time.Minute)

	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "ollama: " + err.Error()})
		return
	}

	// 7) Decode Ollama response
	var gen struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBytes, &gen); err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "decode: " + err.Error()})
		return
	}

	// Fix protocol wording if needed
	answerText := strings.TrimSpace(gen.Response)

	appendChat(jobID, chatTurn{
		Role:   "assistant",
		Text:   answerText,
		Ts:     time.Now().Unix(),
		Source: "llm",
		UserID: userID,
	})

	c.JSON(200, gin.H{
		"ok":      true,
		"answer":  answerText,
		"source":  "llm",
		"signals": signals,
	})
}

// --- Helpers / utilities ---

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
	// chatBaseDir(userID) is defined elsewhere in the same package
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

// intFromMap is used by summary code in the same package.
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
	log.Printf("[ragAnswer] query=%q results=%d", msg, len(results))

	if len(results) == 0 {
		return "", nil
	}

	var b strings.Builder
	refs := make([]string, 0, 2)
	used := 0

	for i, r := range results {
		log.Printf("[ragAnswer] hit %d: id=%s snippet=%q", i, r.ID, r.Snippet)

		sn := strings.TrimSpace(r.Snippet)
		if sn == "" {
			continue
		}

		b.WriteString(sn)
		if !strings.HasSuffix(sn, "\n") {
			b.WriteString("\n")
		}

		base := r.ID
		if slash := strings.LastIndexAny(base, `/\`); slash >= 0 {
			base = base[slash+1:]
		}
		refs = append(refs, base)

		used++
		if used >= 2 {
			break
		}
	}

	ans := strings.TrimSpace(b.String())
	if ans == "" {
		return "", refs
	}
	return ans, refs
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

// --- Signals extraction helpers ---

func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func extractSignals(msg string) map[string]int {
	out := map[string]int{}
	l := strings.ToLower(msg)

	// RPS: if we see one number → treat as peak; if two → [0]=peak, [1]=avg
	rpsMatches := rxRPS.FindAllStringSubmatch(l, -1)
	if len(rpsMatches) >= 1 {
		out["rps_peak"] = atoiSafe(rpsMatches[0][1])
	}
	if len(rpsMatches) >= 2 {
		out["rps_avg"] = atoiSafe(rpsMatches[1][1])
	}

	// p95 latency (ms)
	if m := rxP95ms.FindStringSubmatch(l); len(m) == 2 {
		out["latency_p95_ms"] = atoiSafe(m[1])
	}

	// payload size → normalize to KB
	if m := rxPayload.FindStringSubmatch(l); len(m) == 3 {
		v := atoiSafe(m[1])
		unit := strings.ToLower(m[2])
		if unit == "mb" {
			v = v * 1024
		}
		out["payload_kb"] = v
	}

	// burst factor (e.g. "2x")
	if m := rxBurst.FindStringSubmatch(l); len(m) == 2 {
		out["burst_factor"] = atoiSafe(m[1])
	}

	// CPU (vCPU or cores)
	if m := rxCPU.FindStringSubmatch(l); len(m) >= 2 {
		out["cpu_vcpu"] = atoiSafe(m[1])
	}

	return out
}

// merge history + current signals (current overrides history if non-zero)
func mergeSignals(history, current map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range history {
		out[k] = v
	}
	for k, v := range current {
		if v != 0 {
			out[k] = v
		}
	}
	return out
}

// load signals from previous user turns for this job
func loadSignalsFromHistory(jobID, userID string) map[string]int {
	agg := map[string]int{}

	baseDir := chatBaseDir(userID)
	fpath := filepath.Join(baseDir, fmt.Sprintf("chat-%s.jsonl", jobID))

	f, err := os.Open(fpath)
	if err != nil {
		// no history yet, that's fine
		return agg
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for {
		var t chatTurn
		if err := dec.Decode(&t); err != nil {
			if err == io.EOF {
				break
			}
			// on decode error, just return what we have so far
			return agg
		}
		if t.Role != "user" {
			continue
		}
		s := extractSignals(t.Text)
		for k, v := range s {
			if v != 0 {
				agg[k] = v
			}
		}
	}
	return agg
}

// Build list of missing signals (for follow-up questions), based on aggregated signals
func findMissingSignals(signals map[string]int) []missing {
	m := []missing{}

	if signals["rps_peak"] == 0 {
		m = append(m, missing{Key: "rps_peak", Q: "What is peak vs average RPS (traffic pattern)?"})
	}
	if signals["latency_p95_ms"] == 0 {
		m = append(m, missing{Key: "latency_p95_ms", Q: "What p95 latency target are you aiming for (ms)?"})
	}
	if signals["payload_kb"] == 0 {
		m = append(m, missing{Key: "payload_kb", Q: "Typical payload size (KB) and read/write split?"})
	}
	if signals["burst_factor"] == 0 {
		m = append(m, missing{Key: "burst_factor", Q: "Expected burst (e.g., 2x average) and duration?"})
	}
	if signals["cpu_vcpu"] == 0 {
		m = append(m, missing{Key: "cpu_vcpu", Q: "Current/target CPU/RAM footprint per instance?"})
	}
	return m
}

func isJobSpecific(msg string) bool {
	m := strings.ToLower(msg)
	patterns := []string{
		"my architecture",
		"this architecture",
		"my diagram",
		"this diagram",
		"what i upload",
		"services and edges",
		"in what i upload",
	}
	for _, p := range patterns {
		if strings.Contains(m, p) {
			return true
		}
	}
	return false
}
