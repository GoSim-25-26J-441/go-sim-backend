package http

type Handler struct {
	UpstreamURL string
}

func New(upstreamURL string) *Handler {
	return &Handler{UpstreamURL: upstreamURL}
}
