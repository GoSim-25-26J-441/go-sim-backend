package amg_apd

import (
	"io"
	"net/http"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/graph/export"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/ingest/mapper"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/ingest/parser"
)

func GraphDOTHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	spec, err := parser.FromYAML(b)
	if err != nil {
		http.Error(w, "parse yaml: "+err.Error(), http.StatusBadRequest)
		return
	}
	g := mapper.ToGraph(spec)
	dot, err := export.ToDOT(g)
	if err != nil {
		http.Error(w, "to dot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/vnd.graphviz; charset=utf-8")
	w.Write(dot)
}
