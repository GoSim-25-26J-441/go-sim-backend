package rag

import (
	"embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Question represents a single question in the questionnaire
type Question struct {
	ID          string   `yaml:"id"`
	Label       string   `yaml:"label"`
	Type        string   `yaml:"type"` // "text", "number", "select"
	Options     []string `yaml:"options,omitempty"`
	Placeholder string   `yaml:"placeholder,omitempty"`
}

// QuestionsConfig represents the questions configuration file
type QuestionsConfig struct {
	Enabled   bool       `yaml:"enabled"`
	Questions []Question `yaml:"questions"`
}

// DesignInput represents the design structure from the client
type DesignInput map[string]interface{}

//go:embed questions.yaml
var questionFiles embed.FS

var (
	questionsConfig *QuestionsConfig
)

// LoadQuestions loads the questions configuration from the embedded YAML file.
func LoadQuestions() (*QuestionsConfig, error) {
	if questionsConfig != nil {
		return questionsConfig, nil
	}

	data, err := questionFiles.ReadFile("questions.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded questions.yaml: %w", err)
	}

	var config QuestionsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse questions.yaml: %w", err)
	}

	questionsConfig = &config
	return questionsConfig, nil
}

// GetQuestions returns the questions configuration
func GetQuestions() (*QuestionsConfig, error) {
	return LoadQuestions()
}

// BuildDesignSummary builds a compact summary string from the design object
// Handles nested structure: design.workload.concurrent_users
// Example: design { preferred_vcpu: 4, preferred_memory_gb: 8, workload: { concurrent_users: 1000 }, budget: 2000 }
func BuildDesignSummary(design map[string]interface{}) string {
	if len(design) == 0 {
		return ""
	}

	var parts []string

	// Helper to append key=value
	appendPart := func(key, valStr string) {
		if strings.TrimSpace(valStr) != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", key, valStr))
		}
	}

	// Helper to format value
	formatVal := func(v interface{}) string {
		if v == nil {
			return ""
		}
		switch x := v.(type) {
		case string:
			return x
		case float64:
			return fmt.Sprintf("%.0f", x)
		case int:
			return fmt.Sprintf("%d", x)
		case int64:
			return fmt.Sprintf("%d", x)
		default:
			return fmt.Sprintf("%v", v)
		}
	}

	for key, value := range design {
		if value == nil {
			continue
		}

		// Handle nested workload object
		if key == "workload" {
			if workload, ok := value.(map[string]interface{}); ok {
				if cu, has := workload["concurrent_users"]; has && cu != nil {
					appendPart("workload.concurrent_users", formatVal(cu))
				}
			}
			continue
		}

		// Top-level fields: preferred_vcpu, preferred_memory_gb, budget
		appendPart(key, formatVal(value))
	}

	if len(parts) == 0 {
		return ""
	}

	return "Design: " + strings.Join(parts, ", ")
}

// ReloadQuestions clears the cached config so the next LoadQuestions reads from the embedded file again.
// Useful for testing.
func ReloadQuestions() error {
	questionsConfig = nil
	_, err := LoadQuestions()
	return err
}
