package service

import (
	"fmt"
	"os"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
	_ "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion/strategies"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/versioning"
)

type SuggestPreviewResult struct {
	Analysis     *Result                 `json:"analysis" yaml:"analysis"`
	Suggestions  []suggestion.Suggestion `json:"suggestions" yaml:"suggestions"`
}

type ApplySuggestionsResult struct {
	OriginalAnalysis    *Result                 `json:"original_analysis" yaml:"original_analysis"`
	OriginalSuggestions []suggestion.Suggestion  `json:"original_suggestions" yaml:"original_suggestions"`

	FixedYAML           string                  `json:"fixed_yaml" yaml:"fixed_yaml"`
	FixedVersion        *versioning.Version     `json:"fixed_version" yaml:"fixed_version"`
	FixedAnalysis       *Result                 `json:"fixed_analysis" yaml:"fixed_analysis"`
	AppliedFixes        []suggestion.Suggestion `json:"applied_fixes" yaml:"applied_fixes"`
}

// Frontend-friendly: preview suggestions from the CURRENT YAML content
func PreviewSuggestionsYAMLBytes(yamlBytes []byte, outBaseDir, title string) (*SuggestPreviewResult, error) {
	dotBin := os.Getenv("DOT_BIN")
	analysis, err := AnalyzeYAMLBytes(yamlBytes, outBaseDir, title, dotBin)
	if err != nil {
		return nil, err
	}

	sugs := suggestion.BuildSuggestions(analysis.Graph, analysis.Detections)
	return &SuggestPreviewResult{
		Analysis:    analysis,
		Suggestions: sugs,
	}, nil
}

func PreviewSuggestionsYAMLString(yamlText, outBaseDir, title string) (*SuggestPreviewResult, error) {
	return PreviewSuggestionsYAMLBytes([]byte(yamlText), outBaseDir, title)
}

func PreviewSuggestionsYAMLFile(path, outBaseDir, title string) (*SuggestPreviewResult, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return PreviewSuggestionsYAMLBytes(b, outBaseDir, title)
}

// Apply: auto-fix YAML (heuristics) -> write new YAML version -> re-analyze
func ApplySuggestionsYAMLBytes(jobID string, yamlBytes []byte, outBaseDir, title string) (*ApplySuggestionsResult, error) {
	dotBin := os.Getenv("DOT_BIN")

	// 1) Analyze current YAML
	origAnalysis, err := AnalyzeYAMLBytes(yamlBytes, outBaseDir, title, dotBin)
	if err != nil {
		return nil, err
	}
	origSugs := suggestion.BuildSuggestions(origAnalysis.Graph, origAnalysis.Detections)

	// 2) Apply fixes to spec
	fixed, applied, err := suggestion.ApplyFixesYAMLBytes(yamlBytes, origAnalysis.Graph, origAnalysis.Detections)
	if err != nil {
		return nil, err
	}
	if len(applied) == 0 {
		// No fixes applied; still return something predictable
		return &ApplySuggestionsResult{
			OriginalAnalysis:    origAnalysis,
			OriginalSuggestions: origSugs,
			FixedYAML:           string(yamlBytes),
			FixedVersion:        nil,
			FixedAnalysis:       origAnalysis,
			AppliedFixes:        []suggestion.Suggestion{},
		}, nil
	}

	// 3) Create a new version folder + write YAML there
	ver, err := versioning.CreateVersion(jobID, outBaseDir, "auto_fix", fixed)
	if err != nil {
		return nil, fmt.Errorf("versioning: %w", err)
	}

	// 4) Re-analyze using the fixed YAML into the version directory (no overwrites)
	fixedAnalysis, err := AnalyzeYAMLBytesToDir(fixed, ver.Dir, title, dotBin)
	if err != nil {
		return nil, err
	}

	return &ApplySuggestionsResult{
		OriginalAnalysis:    origAnalysis,
		OriginalSuggestions: origSugs,
		FixedYAML:           string(fixed),
		FixedVersion:        ver,
		FixedAnalysis:       fixedAnalysis,
		AppliedFixes:        applied,
	}, nil
}

func ApplySuggestionsYAMLString(jobID string, yamlText, outBaseDir, title string) (*ApplySuggestionsResult, error) {
	return ApplySuggestionsYAMLBytes(jobID, []byte(yamlText), outBaseDir, title)
}

func ApplySuggestionsYAMLFile(jobID string, path, outBaseDir, title string) (*ApplySuggestionsResult, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ApplySuggestionsYAMLBytes(jobID, b, outBaseDir, title)
}
