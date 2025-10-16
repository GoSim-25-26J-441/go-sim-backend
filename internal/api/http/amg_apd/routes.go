package amg_apd

import "net/http"

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/amg/analyze", AnalyzeHandler) // POST body: YAML
}
