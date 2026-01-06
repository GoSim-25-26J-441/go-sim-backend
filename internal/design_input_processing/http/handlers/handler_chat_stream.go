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

	ig, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/intermediate", upstreamURL, jobID), 10*time.Second)
	spec, _ := fetchJSON(fmt.Sprintf("%s/jobs/%s/export?format=json&download=false", upstreamURL, jobID), 10*time.Second)
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

	req, _ := http.NewRequest("POST", ollamaURL+"/api/generate", bytes.NewReader(reqBody))
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
				fmt.Fprintf(c.Writer, "event: delta\ndata: %s\n\n", jsonString(ch.Response))
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

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
