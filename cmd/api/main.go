package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/service"
	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/amg-apd/analyze", analyzeHandler).Methods("POST")
	port := os.Getenv("PORT")
	if port == "" { port = "8080" }
	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

type reqBody struct {
	Path   string `json:"path"`    // path to YAML file on disk
	OutDir string `json:"out_dir"` // e.g., "out"
	Title  string `json:"title"`
}

func analyzeHandler(w http.ResponseWriter, r *http.Request) {
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	res, err := service.AnalyzeYAML(req.Path, req.OutDir, req.Title, os.Getenv("DOT_BIN"))
	if err != nil {
		http.Error(w, err.Error(), 500); return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
