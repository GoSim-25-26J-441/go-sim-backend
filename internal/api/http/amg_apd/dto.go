package amg_apd

type SuggestionPreviewRequest struct {
	YAML   string `json:"yaml"`
	Title  string `json:"title,omitempty"`
	OutDir string `json:"out_dir,omitempty"`
}

type SuggestionApplyRequest struct {
	JobID  string `json:"job_id,omitempty"`
	YAML   string `json:"yaml"`
	Title  string `json:"title,omitempty"`
	OutDir string `json:"out_dir,omitempty"`
}
