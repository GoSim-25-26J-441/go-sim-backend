package strategies

import (
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

func findDepIndex(spec *parser.YSpec, from, to string) int {
	if spec == nil {
		return -1
	}
	for i := range spec.Dependencies {
		if strings.EqualFold(spec.Dependencies[i].From, from) &&
			strings.EqualFold(spec.Dependencies[i].To, to) {
			return i
		}
	}
	return -1
}

func removeDependencyOnce(spec *parser.YSpec, from, to string) (bool, string) {
	i := findDepIndex(spec, from, to)
	if i < 0 {
		return false, ""
	}
	spec.Dependencies = append(spec.Dependencies[:i], spec.Dependencies[i+1:]...)
	return true, fmt.Sprintf("Removed dependency: %s → %s", from, to)
}

func setDependencySync(spec *parser.YSpec, from, to string, sync bool) (bool, string) {
	i := findDepIndex(spec, from, to)
	if i < 0 {
		return false, ""
	}
	before := spec.Dependencies[i].Sync
	spec.Dependencies[i].Sync = sync
	if before == sync {
		return false, ""
	}
	return true, fmt.Sprintf("Updated sync on %s → %s: %v → %v", from, to, before, sync)
}

func retargetDependency(spec *parser.YSpec, from, oldTo, newTo string) (bool, string) {
	i := findDepIndex(spec, from, oldTo)
	if i < 0 {
		return false, ""
	}
	spec.Dependencies[i].To = newTo
	return true, fmt.Sprintf("Retargeted dependency: %s → %s (was %s)", from, newTo, oldTo)
}

func addDependencyIfMissing(spec *parser.YSpec, dep parser.YDependency) (bool, string) {
	if spec == nil {
		return false, ""
	}
	if findDepIndex(spec, dep.From, dep.To) >= 0 {
		return false, ""
	}
	spec.Dependencies = append(spec.Dependencies, dep)
	return true, fmt.Sprintf("Added dependency: %s → %s", dep.From, dep.To)
}

func findService(spec *parser.YSpec, name string) *parser.YService {
	if spec == nil {
		return nil
	}
	for i := range spec.Services {
		if strings.EqualFold(spec.Services[i].Name, name) {
			return &spec.Services[i]
		}
	}
	return nil
}

func ensureService(spec *parser.YSpec, name string) *parser.YService {
	if s := findService(spec, name); s != nil {
		if s.Type == "" {
			s.Type = "service"
		}
		return s
	}
	spec.Services = append(spec.Services, parser.YService{
		Name: name,
		Type: "service",
	})
	return &spec.Services[len(spec.Services)-1]
}

func uniqueServiceName(spec *parser.YSpec, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "service"
	}
	try := base
	for i := 0; i < 50; i++ {
		if findService(spec, try) == nil {
			return try
		}
		try = fmt.Sprintf("%s-%d", base, i+1)
	}
	return fmt.Sprintf("%s-%d", base, 999)
}

func isDatabaseLikeName(name string) bool {
	s := strings.ToLower(strings.TrimSpace(name))
	return s == "database" || strings.Contains(s, "database") || strings.HasSuffix(s, "-db") || strings.Contains(s, " db")
}

func joinNice(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	if len(xs) == 1 {
		return xs[0]
	}
	if len(xs) == 2 {
		return xs[0] + " and " + xs[1]
	}
	var b strings.Builder
	for i := range xs {
		if i > 0 {
			if i == len(xs)-1 {
				b.WriteString(", and ")
			} else {
				b.WriteString(", ")
			}
		}
		b.WriteString(xs[i])
	}
	return b.String()
}
