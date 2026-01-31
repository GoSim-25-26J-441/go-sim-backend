package http

import (
	"net/http"
	"time"
)

func httpClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}
