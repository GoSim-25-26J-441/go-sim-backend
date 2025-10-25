package http

import (
	"net/http"
	"time"
)

func (h *Handler) client() *http.Client {
	return &http.Client{Timeout: 90 * time.Second}
}
