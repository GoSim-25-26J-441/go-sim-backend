package http

type Handler struct {
	UpstreamURL string
	OllamaURL   string
}

func New(upstreamURL, ollamaURL string) *Handler {
	return &Handler{UpstreamURL: upstreamURL, OllamaURL: ollamaURL}
}
