package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func ChatStream(c *gin.Context, upstreamURL, ollamaURL string) {
	msg := c.Query("message")
	if msg == "" {
		c.JSON(400, gin.H{"ok": false, "error": "missing message"})
		return
	}

	// Basic domain guard (reuse your existing helpers if you want)
	if !isArchitectureQuestion(msg) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", `{"ok":true,"answer":"Architecture topics only."}`)
		c.Writer.Flush()
		return
	}

	jobID := c.Param("id")
	appendChat(jobID, chatTurn{Role: "user", Text: msg, Ts: time.Now().Unix()})

	// Build compact context (same as your non-streaming path)
	ig, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/intermediate", upstreamURL, jobID), 10*time.Second)
	spec, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/export?format=json&download=false", upstreamURL, jobID), 10*time.Second)
	features := compactContext(ig, spec, msg)

	// Prepare Ollama streaming request
	payload := map[string]any{
		"model":  "llama3:instruct",
		"format": "json",
		"stream": true, // IMPORTANT
		"system": `You are a software architecture assistant. Return JSON: {"answer": string}. Keep it concise.`,
		"prompt": fmt.Sprintf("Context:\n%s\n\nQuestion:\n%s\n\nReturn only the JSON.", features, msg),
		"options": map[string]any{
			"temperature": 0.2, "num_ctx": 1024, "num_predict": 512,
		},
	}

	reqBody, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", ollamaURL+"/api/generate", bufio.NewReaderSize(bytes.NewReader(reqBody), 32*1024))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 0} // let stream run
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(502, gin.H{"ok": false, "error": "ollama: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(500, gin.H{"ok": false, "error": "streaming unsupported"})
		return
	}

	// Stream tokens as they arrive
	type chunk struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}
	var full string
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		var ch chunk
		if json.Unmarshal(sc.Bytes(), &ch) == nil {
			if ch.Response != "" {
				full += ch.Response
				// send partial to client UI
				fmt.Fprintf(c.Writer, "event: delta\ndata: %s\n\n", jsonEscape(ch.Response))
				flusher.Flush()
			}
			if ch.Done {
				break
			}
		}
	}
	// finalize: try to extract {"answer": "..."} else send raw text
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

// tiny helper
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	// b is quoted string, return as raw JSON string (no extra quotes)
	return string(b)
}
