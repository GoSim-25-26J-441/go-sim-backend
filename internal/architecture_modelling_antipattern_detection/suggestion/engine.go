package suggestion

import (
	"sort"
	"strconv"
	"strings"

	specpkg "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/spec"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/mapper"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

type Suggestion struct {
	ID      string                 `json:"id" yaml:"id"`
	Kind    domain.AntiPatternKind `json:"kind" yaml:"kind"`
	Title   string                 `json:"title" yaml:"title"`
	Bullets []string               `json:"bullets" yaml:"bullets"`

	AutoFixApplied bool     `json:"auto_fix_applied" yaml:"auto_fix_applied"`
	AutoFixNotes   []string `json:"auto_fix_notes,omitempty" yaml:"auto_fix_notes,omitempty"`
}

// DetectionKey returns a stable unique key for a detection (kind|nodes).
func DetectionKey(d domain.Detection) string {
	nodesKey := append([]string{}, d.Nodes...)
	sort.Strings(nodesKey)
	return string(d.Kind) + "|" + join(nodesKey, ",")
}

type Strategy interface {
	Kind() domain.AntiPatternKind
	Suggest(g *domain.Graph, det domain.Detection) Suggestion
	Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (changed bool, notes []string)
}

var strategies []Strategy

func Register(s Strategy) { strategies = append(strategies, s) }

func findStrategy(kind domain.AntiPatternKind) Strategy {
	for _, s := range strategies {
		if s.Kind() == kind {
			return s
		}
	}
	return nil
}

func severityWeight(s domain.Severity) int {
	switch s {
	case domain.SeverityHigh:
		return 3
	case domain.SeverityMedium:
		return 2
	default:
		return 1
	}
}

func BuildSuggestions(g *domain.Graph, dets []domain.Detection) []Suggestion {
	tmp := make([]domain.Detection, 0, len(dets))
	tmp = append(tmp, dets...)
	sort.SliceStable(tmp, func(i, j int) bool {
		wi := severityWeight(tmp[i].Severity)
		wj := severityWeight(tmp[j].Severity)
		if wi != wj {
			return wi > wj
		}
		return string(tmp[i].Kind) < string(tmp[j].Kind)
	})

	out := make([]Suggestion, 0, len(tmp))
	seen := map[string]bool{}
	for _, d := range tmp {
		key := DetectionKey(d)
		if seen[key] {
			continue
		}
		seen[key] = true

		s := findStrategy(d.Kind)
		if s == nil {
			out = append(out, Suggestion{
				ID:      key,
				Kind:    d.Kind,
				Title:   d.Title,
				Bullets: []string{"No suggestion strategy found for this anti-pattern yet."},
			})
			continue
		}
		sug := s.Suggest(g, d)
		sug.ID = key
		out = append(out, sug)
	}
	return out
}

// OrderedDetectionKeys returns detection keys in the same order as BuildSuggestions (for index-based selection).
func OrderedDetectionKeys(dets []domain.Detection) []string {
	tmp := make([]domain.Detection, 0, len(dets))
	tmp = append(tmp, dets...)
	sort.SliceStable(tmp, func(i, j int) bool {
		wi := severityWeight(tmp[i].Severity)
		wj := severityWeight(tmp[j].Severity)
		if wi != wj {
			return wi > wj
		}
		return string(tmp[i].Kind) < string(tmp[j].Kind)
	})
	var keys []string
	seen := map[string]bool{}
	for _, d := range tmp {
		key := DetectionKey(d)
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

// ResolveSelectedIDs converts selectedIDs (which may include "idx:N" format) to a set of detection keys.
func ResolveSelectedIDs(selectedIDs []string, orderedKeys []string) map[string]bool {
	out := make(map[string]bool)
	if selectedIDs == nil {
		return out
	}
	for _, id := range selectedIDs {
		if strings.HasPrefix(id, "idx:") {
			n, err := strconv.Atoi(strings.TrimPrefix(id, "idx:"))
			if err == nil && n >= 0 && n < len(orderedKeys) {
				out[orderedKeys[n]] = true
			}
		} else {
			out[id] = true
		}
	}
	return out
}

// ApplyFixesYAMLBytes applies fixes for all detections (legacy).
func ApplyFixesYAMLBytes(yamlBytes []byte, g *domain.Graph, dets []domain.Detection) ([]byte, []Suggestion, error) {
	return ApplyFixesYAMLBytesFiltered(yamlBytes, g, dets, nil)
}

// ApplyFixesYAMLBytesFiltered applies fixes only for detections whose DetectionKey is in selectedIDs.
// When selectedIDs is nil or empty, no fixes are applied (user must explicitly select suggestions).
func ApplyFixesYAMLBytesFiltered(yamlBytes []byte, g *domain.Graph, dets []domain.Detection, selectedIDs map[string]bool) ([]byte, []Suggestion, error) {
	origRaw, err := specpkg.UnmarshalMap(yamlBytes)
	if err != nil {
		return nil, nil, err
	}

	specStruct, err := parser.ParseYAMLBytes(yamlBytes)
	if err != nil {
		return nil, nil, err
	}
	mapper.NormalizeYAMLSpecInPlace(specStruct)

	tmp := make([]domain.Detection, 0, len(dets))
	tmp = append(tmp, dets...)
	sort.SliceStable(tmp, func(i, j int) bool {
		return severityWeight(tmp[i].Severity) > severityWeight(tmp[j].Severity)
	})

	var applied []Suggestion
	for _, d := range tmp {
		// If selection filter is provided, only apply selected detections
		if selectedIDs != nil {
			if len(selectedIDs) == 0 {
				continue
			}
			if !selectedIDs[DetectionKey(d)] {
				continue
			}
		}
		s := findStrategy(d.Kind)
		if s == nil {
			continue
		}
		changed, notes := s.Apply(specStruct, g, d)
		if changed {
			ss := s.Suggest(g, d)
			ss.AutoFixApplied = true
			ss.AutoFixNotes = notes
			applied = append(applied, ss)
		}
	}

	fixedBytes, err := marshalSpec(specStruct)
	if err != nil {
		return nil, nil, err
	}
	fixedRaw, err := specpkg.UnmarshalMap(fixedBytes)
	if err != nil {
		return nil, nil, err
	}

	specpkg.Sanitize(fixedRaw)

	merged := specpkg.Merge(origRaw, fixedRaw)

	out, err := specpkg.MarshalMap(merged)
	if err != nil {
		return nil, nil, err
	}

	return out, applied, nil
}
