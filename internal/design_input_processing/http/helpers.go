package http

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
)

// Types for chat functionality
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

// chatBaseDir returns the base directory for chat logs for a given user
func chatBaseDir(userID string) string {
	dir := os.Getenv("CHAT_LOG_DIR")
	if dir == "" {
		dir = filepath.FromSlash("internal/design_input_processing/data/chat_logs")
	}

	if strings.TrimSpace(userID) == "" {
		userID = "demo-user"
	}

	userID = strings.ReplaceAll(userID, "..", "_")
	userID = strings.ReplaceAll(userID, "/", "_")
	userID = strings.ReplaceAll(userID, "\\", "_")

	return filepath.Join(dir, userID)
}

// fetchJSON fetches a JSON response from a URL
func (h *Handler) fetchJSON(url string, to time.Duration) (map[string]any, error) {
	cl := &http.Client{Timeout: to}
	r, err := cl.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	var m map[string]any
	return m, json.NewDecoder(r.Body).Decode(&m)
}

// postJSON posts JSON and returns the response body
func (h *Handler) postJSON(url string, body any, to time.Duration) ([]byte, error) {
	b, _ := json.Marshal(body)
	rq, _ := http.NewRequest("POST", url, strings.NewReader(string(b)))
	rq.Header.Set("Content-Type", "application/json")
	cl := &http.Client{Timeout: to}
	rs, err := cl.Do(rq)
	if err != nil {
		return nil, err
	}
	defer rs.Body.Close()
	return io.ReadAll(rs.Body)
}

// listJobIDsForUser lists all job IDs for a given user (helper function)
func listJobIDsForUser(userID string) ([]string, error) {
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
		name := e.Name()
		if !strings.HasPrefix(name, "chat-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "chat-"), ".jsonl")
		jobIDs = append(jobIDs, id)
	}
	return jobIDs, nil
}

// intFromMap safely extracts an int from a map
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

// atoiSafe safely converts string to int
func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// loadSignalsFromHistory loads signals from chat history
func (h *Handler) loadSignalsFromHistory(jobID, userID string) map[string]int {
	agg := map[string]int{}

	baseDir := chatBaseDir(userID)
	fpath := filepath.Join(baseDir, fmt.Sprintf("chat-%s.jsonl", jobID))

	f, err := os.Open(fpath)
	if err != nil {
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

// extractSignals extracts signals from a message
func extractSignals(msg string) map[string]int {
	out := map[string]int{}
	l := strings.ToLower(msg)

	rpsMatches := rxRPS.FindAllStringSubmatch(l, -1)
	if len(rpsMatches) >= 1 {
		out["rps_peak"] = atoiSafe(rpsMatches[0][1])
	}
	if len(rpsMatches) >= 2 {
		out["rps_avg"] = atoiSafe(rpsMatches[1][1])
	}

	if m := rxP95ms.FindStringSubmatch(l); len(m) == 2 {
		out["latency_p95_ms"] = atoiSafe(m[1])
	}

	if m := rxPayload.FindStringSubmatch(l); len(m) == 3 {
		v := atoiSafe(m[1])
		unit := strings.ToLower(m[2])
		if unit == "mb" {
			v = v * 1024
		}
		out["payload_kb"] = v
	}

	if m := rxBurst.FindStringSubmatch(l); len(m) == 2 {
		out["burst_factor"] = atoiSafe(m[1])
	}

	if m := rxCPU.FindStringSubmatch(l); len(m) >= 2 {
		out["cpu_vcpu"] = atoiSafe(m[1])
	}

	return out
}

// Helper functions for chat functionality
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

func isDiagramQuestion(q string) bool {
	s := strings.ToLower(q)

	if strings.Contains(s, "diagram") ||
		strings.Contains(s, "picture") ||
		strings.Contains(s, "image") ||
		strings.Contains(s, "drawing") {
		return true
	}

	if strings.Contains(s, "what i upload") ||
		strings.Contains(s, "what i uploaded") ||
		strings.Contains(s, "in what i upload") ||
		strings.Contains(s, "in what i uploaded") {
		return true
	}

	if strings.Contains(s, "my services") ||
		strings.Contains(s, "my edges") ||
		strings.Contains(s, "services and edges") {
		return true
	}

	return false
}

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
		"rps @",
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

func compactContext(ig, spec map[string]any, msg string) string {
	services := tryArrayNames(spec, "services", "name")
	if len(services) == 0 {
		services = tryArrayNames(ig, "Nodes", "Label")
	}

	edges := tryDeps(spec)
	if len(edges) == 0 {
		edges = tryEdges(ig)
	}

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

func tryDeps(spec map[string]any) [][3]string {
	arr, _ := spec["dependencies"].([]any)
	out := make([][3]string, 0, len(arr))
	for _, v := range arr {
		if obj, ok := v.(map[string]any); ok {
			from, _ := obj["from"].(string)
			to, _ := obj["to"].(string)
			kind, _ := obj["kind"].(string)
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

func isSummaryQuestion(q string) bool {
	s := strings.ToLower(q)

	summaryPhrases := []string{
		"overall summary",
		"summary of the system",
		"summary of my system",
		"final idea",
		"overall idea",
		"what do you think",
		"your opinion",
		"tell me about my system",
		"explain my system",
		"explain my chat",
	}

	for _, p := range summaryPhrases {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
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

func ragAnswer(msg string) (string, []string) {
	results := diprag.Search(msg)
	log.Printf("[ragAnswer] query=%q results=%d", msg, len(results))

	if len(results) == 0 {
		return "", nil
	}

	lower := strings.ToLower(msg)

	allowMetaDocs := strings.Contains(lower, "sizing prompt") ||
		strings.Contains(lower, "questions should i ask") ||
		strings.Contains(lower, "what should i ask") ||
		strings.Contains(lower, "defaults") ||
		strings.Contains(lower, "guardrail")

	var b strings.Builder
	refs := make([]string, 0, 2)
	used := 0

	for i, r := range results {
		log.Printf("[ragAnswer] hit %d: id=%s snippet=%q", i, r.ID, r.Snippet)

		sn := strings.TrimSpace(r.Snippet)
		if sn == "" {
			continue
		}

		base := r.ID
		if slash := strings.LastIndexAny(base, `/\`); slash >= 0 {
			base = base[slash+1:]
		}

		if !allowMetaDocs && (base == "sizing_prompts.md" || base == "defaults_guardrails.md") {
			continue
		}

		b.WriteString(sn)
		if !strings.HasSuffix(sn, "\n") {
			b.WriteString("\n")
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

func readChat(userID, jobID string, limit int) []chatTurn {
	baseDir := chatBaseDir(userID)
	fpath := filepath.Join(baseDir, fmt.Sprintf("chat-%s.jsonl", jobID))

	f, err := os.Open(fpath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var turns []chatTurn
	dec := json.NewDecoder(f)
	for {
		var t chatTurn
		if err := dec.Decode(&t); err != nil {
			if err == io.EOF {
				break
			}
			return turns
		}
		turns = append(turns, t)
	}

	if limit > 0 && len(turns) > limit {
		turns = turns[len(turns)-limit:]
	}
	return turns
}

func clearChat(userID, jobID string) bool {
	baseDir := chatBaseDir(userID)
	fpath := filepath.Join(baseDir, fmt.Sprintf("chat-%s.jsonl", jobID))

	if err := os.Remove(fpath); err != nil && !os.IsNotExist(err) {
		return false
	}
	return true
}

func buildDOTFromSpec(spec map[string]any) string {
	services := tryArrayNames(spec, "services", "name")
	deps := tryDeps(spec)

	var b strings.Builder
	b.WriteString("digraph G {\n")
	b.WriteString("  rankdir=LR;\n")

	for _, s := range services {
		fmt.Fprintf(&b, "  \"%s\" [shape=box];\n", s)
	}

	for _, d := range deps {
		from, to, kind := d[0], d[1], d[2]
		if kind == "" {
			kind = "call"
		}
		label := strings.ToUpper(kind)
		fmt.Fprintf(&b, "  \"%s\" -> \"%s\" [label=\"%s\"];\n", from, to, label)
	}

	b.WriteString("}\n")
	return b.String()
}
