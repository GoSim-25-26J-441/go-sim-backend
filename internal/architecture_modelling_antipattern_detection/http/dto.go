package http

// Suggestion preview request (read-only)
type SuggestionPreviewRequest struct {
	YAML   string `json:"yaml"`              // required
	Title  string `json:"title,omitempty"`   // optional
	OutDir string `json:"out_dir,omitempty"` // optional base dir (default "/app/out")
}

// Apply suggestions request (writes a new version + re-analyzes)
type SuggestionApplyRequest struct {
	JobID  string `json:"job_id,omitempty"`  // optional (default "adhoc")
	YAML   string `json:"yaml"`              // required
	Title  string `json:"title,omitempty"`   // optional
	OutDir string `json:"out_dir,omitempty"` // optional base dir (default "/app/out")
}
