package amg_apd

import "net/http"

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/amg/analyze", AnalyzeHandler)
	mux.HandleFunc("/amg/graph.dot", GraphDOTHandler)
}
