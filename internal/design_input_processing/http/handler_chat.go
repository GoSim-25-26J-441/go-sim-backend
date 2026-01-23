package http

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) chat(c *gin.Context) {
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

	historySignals := h.loadSignalsFromHistory(jobID, userID)
	thisSignals := extractSignals(req.Message)
	signals := mergeSignals(historySignals, thisSignals)

	if strict && !onTopic(req.Message) {
		guardMsg := "Let's keep it on software architecture (microservices, APIs, sizing, latency, etc.)."
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

	appendChat(jobID, chatTurn{
		Role:   "user",
		Text:   req.Message,
		Ts:     time.Now().Unix(),
		Source: "user",
		UserID: userID,
	})

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
	isSum := isSummaryQuestion(req.Message)
	ragCandidate := isRAGCandidate(req.Message)

	missing := findMissingSignals(signals)
	if isSizing && len(missing) > 0 {
		var b strings.Builder
		b.WriteString("To size this accurately, I need:\n")
		for i, m := range missing {
			if i >= 3 {
				break
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

	skipRAG := isSizing && len(missing) == 0

	tryRAG := ragCandidate &&
		!skipRAG &&
		!req.ForceLLM &&
		!isDef &&
		!isJob &&
		!isDiag &&
		!isSum

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

	ig, _ := h.fetchJSON(fmt.Sprintf("%s/jobs/%s/intermediate", h.UpstreamURL, jobID), 10*time.Second)
	spec, _ := h.fetchJSON(fmt.Sprintf("%s/jobs/%s/export?format=json&download=false", h.UpstreamURL, jobID), 10*time.Second)
	features := compactContext(ig, spec, req.Message)
	log.Printf("[chat] job=%s features:\n%s", jobID, features)

	signalsJSON, _ := json.Marshal(signals)

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

	respBytes, err := h.postJSON(h.OllamaURL+"/api/generate", body, 3*time.Minute)

	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "ollama: " + err.Error()})
		return
	}

	var gen struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBytes, &gen); err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "decode: " + err.Error()})
		return
	}

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

func (h *Handler) chatStream(c *gin.Context) {
	msg := c.Query("message")
	if msg == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing message"})
		return
	}

	if !isArchitectureQuestion(msg) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Header("Access-Control-Allow-Origin", "*")

		fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", `{"ok":true,"answer":"Architecture topics only."}`)
		if f, ok := c.Writer.(http.Flusher); ok {
			f.Flush()
		}
		return
	}

	jobID := c.Param("id")
	appendChat(jobID, chatTurn{Role: "user", Text: msg, Ts: time.Now().Unix()})

	ig, _ := h.fetchJSON(fmt.Sprintf("%s/jobs/%s/intermediate", h.UpstreamURL, jobID), 10*time.Second)
	spec, _ := h.fetchJSON(fmt.Sprintf("%s/jobs/%s/export?format=json&download=false", h.UpstreamURL, jobID), 10*time.Second)
	features := compactContext(ig, spec, msg)

	payload := map[string]any{
		"model":  "llama3:instruct",
		"format": "json",
		"stream": true,
		"system": `You are a software architecture assistant. Return JSON: {"answer": string}. Keep it concise.`,
		"prompt": fmt.Sprintf("Context:\n%s\n\nQuestion:\n%s\n\nReturn only the JSON.", features, msg),
		"options": map[string]any{
			"temperature": 0.2, "num_ctx": 1024, "num_predict": 512,
		},
	}
	reqBody, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", h.OllamaURL+"/api/generate", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": "ollama: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Access-Control-Allow-Origin", "*")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "streaming unsupported"})
		return
	}

	ctx := c.Request.Context()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fmt.Fprint(c.Writer, ": keep-alive\n\n")
				flusher.Flush()
			}
		}
	}()

	type chunk struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}

	var full string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for sc.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		var ch chunk
		if json.Unmarshal(sc.Bytes(), &ch) == nil {
			if ch.Response != "" {
				full += ch.Response
				jsonStr, _ := json.Marshal(ch.Response)
				fmt.Fprintf(c.Writer, "event: delta\ndata: %s\n\n", string(jsonStr))
				flusher.Flush()
			}
			if ch.Done {
				break
			}
		}
	}

	answer := full
	var out struct {
		Answer string `json:"answer"`
	}
	if json.Unmarshal([]byte(full), &out) == nil && out.Answer != "" {
		answer = out.Answer
	}

	appendChat(jobID, chatTurn{
		Role:   "assistant",
		Text:   answer,
		Ts:     time.Now().Unix(),
		Source: "llm",
	})

	fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", `{"ok":true}`)
	flusher.Flush()
}

func (h *Handler) graphviz(c *gin.Context) {
	jobID := c.Param("id")

	spec, err := h.fetchJSON(
		fmt.Sprintf("%s/jobs/%s/export?format=json&download=false", h.UpstreamURL, jobID),
		10*time.Second,
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": "export: " + err.Error()})
		return
	}

	dot := buildDOTFromSpec(spec)

	c.Header("Content-Type", "text/vnd.graphviz; charset=utf-8")
	c.String(http.StatusOK, dot)
}
