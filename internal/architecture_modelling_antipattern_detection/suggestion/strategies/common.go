package strategies

import (
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection/rules"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/mapper"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

// cleanRef removes leaked graph-ID prefixes like SERVICE:foo / DATABASE:bar
// and trims whitespace.
func cleanRef(s string) string {
	return mapper.StripNodeNameRef(s)
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

func findServiceIndexByRef(spec *parser.YSpec, name string) int {
	if spec == nil {
		return -1
	}
	n := cleanRef(name)
	for i := range spec.Services {
		if eqRef(spec.Services[i].Name, n) {
			return i
		}
	}
	return -1
}

// removeLegacyCall removes one services[].calls entry (legacy YAML without top-level dependencies).
func removeLegacyCall(spec *parser.YSpec, from, to string) (bool, string) {
	fi := findServiceIndexByRef(spec, from)
	if fi < 0 {
		return false, ""
	}
	t := cleanRef(to)
	svc := &spec.Services[fi]
	for i, c := range svc.Calls {
		if eqRef(c.To, t) {
			svc.Calls = append(svc.Calls[:i], svc.Calls[i+1:]...)
			return true, fmt.Sprintf("Removed call %s → %s", cleanRef(svc.Name), t)
		}
	}
	return false, ""
}

// addLegacyCall appends a call on the given service if not already present.
func addLegacyCall(spec *parser.YSpec, from, to string) (bool, string) {
	fromC := cleanRef(from)
	toC := cleanRef(to)
	ensureService(spec, fromC)
	fi := findServiceIndexByRef(spec, fromC)
	if fi < 0 {
		return false, ""
	}
	svc := &spec.Services[fi]
	for _, c := range svc.Calls {
		if eqRef(c.To, toC) {
			return false, ""
		}
	}
	svc.Calls = append(svc.Calls, parser.YCall{To: toC})
	return true, fmt.Sprintf("Added call %s → %s", fromC, toC)
}

// flipDependencyDirection removes from→to and adds to→from (new-style dependencies), preserving kind/sync when possible.
func flipDependencyDirection(spec *parser.YSpec, from, to string) (bool, []string) {
	f := cleanRef(from)
	t := cleanRef(to)
	i := findDepIndex(spec, f, t)
	if i < 0 {
		return false, nil
	}
	dep := spec.Dependencies[i]
	kind := strings.ToLower(strings.TrimSpace(dep.Kind))
	if kind == "" {
		kind = "rest"
	}
	sync := dep.Sync
	spec.Dependencies = append(spec.Dependencies[:i], spec.Dependencies[i+1:]...)
	var notes []string
	notes = append(notes, fmt.Sprintf("Removed dependency: %s → %s", f, t))
	ok, n := addDependencyIfMissing(spec, parser.YDependency{
		From: t,
		To:   f,
		Kind: kind,
		Sync: sync,
	})
	if ok {
		notes = append(notes, n)
	} else if findDepIndex(spec, t, f) >= 0 {
		notes = append(notes, fmt.Sprintf("Dependency %s → %s already existed", t, f))
	}
	return true, notes
}

// flipLegacyCallDirection removes from→to call and adds to→from under legacy services[].calls.
func flipLegacyCallDirection(spec *parser.YSpec, from, to string) (bool, []string) {
	ok, n1 := removeLegacyCall(spec, from, to)
	if !ok {
		return false, nil
	}
	notes := []string{n1}
	ok2, n2 := addLegacyCall(spec, to, from)
	if ok2 {
		notes = append(notes, n2)
	} else {
		notes = append(notes, fmt.Sprintf("Call %s → %s already present (no duplicate added)", cleanRef(to), cleanRef(from)))
	}
	return true, notes
}

// removeDependencyOrLegacyCall removes a CALLS edge from new-style dependencies or legacy services[].calls.
func removeDependencyOrLegacyCall(spec *parser.YSpec, from, to string) (bool, string) {
	if ok, n := removeDependencyOnce(spec, from, to); ok {
		return true, n
	}
	return removeLegacyCall(spec, from, to)
}

// pingPongRemovalSequence lists dependency removals to try (deduped). Prefers dropping backend→UI, else B→A, then A→B.
func pingPongRemovalSequence(a, b string) [][2]string {
	var out [][2]string
	add := func(from, to string) {
		f, t := cleanRef(from), cleanRef(to)
		for _, e := range out {
			if cleanRef(e[0]) == f && cleanRef(e[1]) == t {
				return
			}
		}
		out = append(out, [2]string{from, to})
	}

	uiA, uiB := rules.IsUIServiceID(a), rules.IsUIServiceID(b)
	if !uiA && uiB {
		add(a, b)
	}
	if uiA && !uiB {
		add(b, a)
	}
	add(b, a)
	add(a, b)
	return out
}

// pingPongPreviewRemoveLeg returns "top" if removal matches top-row direction (topFrom → topTo).
func pingPongPreviewRemoveLeg(topFrom, topTo, remFrom, remTo string) string {
	if eqRef(remFrom, topFrom) && eqRef(remTo, topTo) {
		return "top"
	}
	return "bottom"
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

// ensureAPIGateway adds or finds a service and sets its type to api_gateway (BFF/gateway).
func ensureAPIGateway(spec *parser.YSpec, name string) *parser.YService {
	n := cleanRef(name)
	if s := findService(spec, n); s != nil {
		if s.Type == "" {
			s.Type = "api_gateway"
		}
		return s
	}
	spec.Services = append(spec.Services, parser.YService{
		Name: n,
		Type: "api_gateway",
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

// removeService removes a service/database node from spec.Services by name.
func removeService(spec *parser.YSpec, name string) bool {
	if spec == nil {
		return false
	}
	n := cleanRef(name)
	for i := range spec.Services {
		if eqRef(spec.Services[i].Name, n) {
			spec.Services = append(spec.Services[:i], spec.Services[i+1:]...)
			return true
		}
	}
	return false
}

// removeDatabase removes a database node from spec.Databases by name.
func removeDatabase(spec *parser.YSpec, name string) bool {
	if spec == nil || len(spec.Databases) == 0 {
		return false
	}
	n := cleanRef(name)
	for i := range spec.Databases {
		if eqRef(spec.Databases[i].Name, n) {
			spec.Databases = append(spec.Databases[:i], spec.Databases[i+1:]...)
			return true
		}
	}
	return false
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
