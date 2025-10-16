package amg_apd

type AnalyzeResponse struct {
	AnalysisID string       `json:"analysis_id"`
	Graph      any          `json:"graph"`
	Findings   []FindingDTO `json:"findings"`
}

type FindingDTO struct {
	Kind     string         `json:"kind"`
	Severity string         `json:"severity"`
	Summary  string         `json:"summary"`
	Nodes    []string       `json:"nodes,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
}
