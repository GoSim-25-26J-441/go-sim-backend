package strategies

import (
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

// cleanRef removes leaked graph-ID prefixes like SERVICE:foo / DATABASE:bar
// and trims whitespace.
func cleanRef(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}

	up := strings.ToUpper(t)

	if strings.HasPrefix(up, "SERVICE:") {
		return strings.TrimSpace(t[len("SERVICE:"):])
	}
	if strings.HasPrefix(up, "DATABASE:") {
		return strings.TrimSpace(t[len("DATABASE:"):])
	}

	// tolerate "SERVICE : foo" style
	noSpaceUp := strings.ReplaceAll(up, " ", "")
	noSpaceRaw := strings.ReplaceAll(t, " ", "")
	if strings.HasPrefix(noSpaceUp, "SERVICE:") {
		return strings.TrimSpace(noSpaceRaw[len("SERVICE:"):])
	}
	if strings.HasPrefix(noSpaceUp, "DATABASE:") {
		return strings.TrimSpace(noSpaceRaw[len("DATABASE:"):])
	}

	return t
}

func eqRef(a, b string) bool {
	return strings.EqualFold(cleanRef(a), cleanRef(b))
}

func findDepIndex(spec *parser.YSpec, from, to string) int {
	if spec == nil {
		return -1
	}
	f := cleanRef(from)
	t := cleanRef(to)

	for i := range spec.Dependencies {
		if eqRef(spec.Dependencies[i].From, f) && eqRef(spec.Dependencies[i].To, t) {
			return i
		}
	}
	return -1
}

func removeDependencyOnce(spec *parser.YSpec, from, to string) (bool, string) {
	f := cleanRef(from)
	t := cleanRef(to)

	i := findDepIndex(spec, f, t)
	if i < 0 {
		return false, ""
	}
	spec.Dependencies = append(spec.Dependencies[:i], spec.Dependencies[i+1:]...)
	return true, fmt.Sprintf("Removed dependency: %s → %s", f, t)
}

func setDependencySync(spec *parser.YSpec, from, to string, sync bool) (bool, string) {
	f := cleanRef(from)
	t := cleanRef(to)

	i := findDepIndex(spec, f, t)
	if i < 0 {
		return false, ""
	}
	before := spec.Dependencies[i].Sync
	spec.Dependencies[i].Sync = sync
	if before == sync {
		return false, ""
	}
	return true, fmt.Sprintf("Updated sync on %s → %s: %v → %v", f, t, before, sync)
}

func retargetDependency(spec *parser.YSpec, from, oldTo, newTo string) (bool, string) {
	f := cleanRef(from)
	o := cleanRef(oldTo)
	n := cleanRef(newTo)

	i := findDepIndex(spec, f, o)
	if i < 0 {
		return false, ""
	}
	spec.Dependencies[i].To = n
	return true, fmt.Sprintf("Retargeted dependency: %s → %s (was %s)", f, n, o)
}

func addDependencyIfMissing(spec *parser.YSpec, dep parser.YDependency) (bool, string) {
	if spec == nil {
		return false, ""
	}

	dep.From = cleanRef(dep.From)
	dep.To = cleanRef(dep.To)
	dep.Kind = strings.ToLower(strings.TrimSpace(dep.Kind))

	if dep.From == "" || dep.To == "" {
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
	n := cleanRef(name)
	for i := range spec.Services {
		if strings.EqualFold(cleanRef(spec.Services[i].Name), n) {
			return &spec.Services[i]
		}
	}
	return nil
}

func ensureService(spec *parser.YSpec, name string) *parser.YService {
	n := cleanRef(name)
	if s := findService(spec, n); s != nil {
		if s.Type == "" {
			s.Type = "service"
		}
		return s
	}
	spec.Services = append(spec.Services, parser.YService{
		Name: n,
		Type: "service",
	})
	return &spec.Services[len(spec.Services)-1]
}

func ensureDatabase(spec *parser.YSpec, name string) *parser.YService {
	n := cleanRef(name)
	if s := findService(spec, n); s != nil {
		// do not overwrite explicit type, but default to database if empty
		if s.Type == "" {
			s.Type = "database"
		}
		return s
	}
	spec.Services = append(spec.Services, parser.YService{
		Name: n,
		Type: "database",
	})
	return &spec.Services[len(spec.Services)-1]
}

func uniqueServiceName(spec *parser.YSpec, base string) string {
	base = cleanRef(base)
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
	s := strings.ToLower(strings.TrimSpace(cleanRef(name)))
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
