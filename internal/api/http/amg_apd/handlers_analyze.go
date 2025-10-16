package amg_apd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/service"
)

func AnalyzeHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := service.AnalyzeYAML(context.Background(), b)
	if err != nil {
		http.Error(w, "analyze: "+err.Error(), http.StatusBadRequest)
		return
	}

	var graph any
	_ = json.Unmarshal(res.GraphJSON, &graph)

	out := AnalyzeResponse{
		AnalysisID: "local-stub",
		Graph:      graph,
	}
	for _, f := range res.Findings {
		out.Findings = append(out.Findings, FindingDTO{
			Kind:     f.Kind,
			Severity: f.Severity,
			Summary:  f.Summary,
			Nodes:    f.Nodes,
			Meta:     f.Meta,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
