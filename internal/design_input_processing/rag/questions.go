package rag

import (
	"fmt"
	"os"
	"path/filepath"
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

// RequirementsAnswers represents user answers to the questionnaire
type RequirementsAnswers map[string]interface{}

var (
	questionsConfig *QuestionsConfig
	configPath      = filepath.Join("internal", "design_input_processing", "rag", "questions.yaml")
)

// LoadQuestions loads the questions configuration from the YAML file
func LoadQuestions() (*QuestionsConfig, error) {
	if questionsConfig != nil {
		return questionsConfig, nil
	}

	// Try to find the file relative to the current working directory
	// This works when running from the project root (go run ./cmd/api)
	var data []byte
	var err error
	
	// Try multiple possible paths
	possiblePaths := []string{
		configPath,
		filepath.Join(".", configPath),
		filepath.Join("..", configPath),
		filepath.Join("../..", configPath),
	}
	
	for _, path := range possiblePaths {
		if data, err = os.ReadFile(path); err == nil {
			break
		}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to read questions.yaml (tried: %v): %w", possiblePaths, err)
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

// BuildRequirementsSummary builds a compact summary string from user answers
// This summary is injected into the LLM context for the first message
func BuildRequirementsSummary(answers map[string]interface{}) string {
	if len(answers) == 0 {
		return ""
	}

	var parts []string
	for key, value := range answers {
		if value == nil || value == "" {
			continue
		}
		
		var valStr string
		switch v := value.(type) {
		case string:
			valStr = v
		case float64:
			valStr = fmt.Sprintf("%.0f", v)
		case int:
			valStr = fmt.Sprintf("%d", v)
		case int64:
			valStr = fmt.Sprintf("%d", v)
		default:
			valStr = fmt.Sprintf("%v", v)
		}
		
		if strings.TrimSpace(valStr) != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", key, valStr))
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "User requirements: " + strings.Join(parts, ", ")
}

// ReloadQuestions reloads the questions configuration from disk
// Useful for testing or when questions.yaml is updated
func ReloadQuestions() error {
	questionsConfig = nil
	_, err := LoadQuestions()
	return err
}
