package versioning

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/utils"
)

type Version struct {
	JobID     string    `json:"job_id" yaml:"job_id"`
	VersionID string    `json:"version_id" yaml:"version_id"`
	Label     string    `json:"label" yaml:"label"`
	Dir       string    `json:"dir" yaml:"dir"`
	YAMLPath  string    `json:"yaml_path" yaml:"yaml_path"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// CreateVersion writes a new YAML "snapshot" under:
//   <outBaseDir>/versions/<jobID>/<versionID>/architecture.yaml
//
// The same folder will also store analysis outputs (graph.dot, graph.svg, analysis.json, ...).
func CreateVersion(jobID, outBaseDir, label string, yamlBytes []byte) (*Version, error) {
	if outBaseDir == "" {
		outBaseDir = "out"
	}
	if jobID == "" {
		jobID = "adhoc"
	}
	if label == "" {
		label = "version"
	}

	vid := utils.NewID()
	dir := filepath.Join(outBaseDir, "versions", jobID, vid)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	yamlPath := filepath.Join(dir, "architecture.yaml")
	if err := os.WriteFile(yamlPath, yamlBytes, 0644); err != nil {
		return nil, err
	}

	v := &Version{
		JobID:     jobID,
		VersionID: vid,
		Label:     label,
		Dir:       dir,
		YAMLPath:  yamlPath,
		CreatedAt: time.Now(),
	}

	metaPath := filepath.Join(dir, "version.json")
	meta, _ := json.MarshalIndent(v, "", "  ")
	_ = os.WriteFile(metaPath, meta, 0644)

	return v, nil
}

func ReadVersion(outBaseDir, jobID, versionID string) (*Version, error) {
	if outBaseDir == "" {
		outBaseDir = "out"
	}
	if jobID == "" || versionID == "" {
		return nil, fmt.Errorf("jobID and versionID are required")
	}
	dir := filepath.Join(outBaseDir, "versions", jobID, versionID)
	metaPath := filepath.Join(dir, "version.json")
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var v Version
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
