package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"gopkg.in/yaml.v3"
)

// SPEC TYPES
type DatabaseAccess struct {
	Name   string `yaml:"name"`
	Access string `yaml:"access"`
}

type Service struct {
	Name            string           `yaml:"name"`
	Lang            string           `yaml:"lang"`
	Team            string           `yaml:"team"`
	Responsibilities []string        `yaml:"responsibilities"`
	Replicas        int              `yaml:"replicas"`
	Databases       []DatabaseAccess `yaml:"databases"`
	EmitsTopics     []string         `yaml:"emits_topics"`
}

type Datastore struct {
	Name   string `yaml:"name"`
	Engine string `yaml:"engine"`
}

type Call struct {
	From     string `yaml:"from"`
	To       string `yaml:"to"`
	Sync     bool   `yaml:"sync"`
	Endpoint string `yaml:"endpoint"`
	Mode     string `yaml:"mode"`  
	Topic    string `yaml:"topic"` 
}

type NonFunctionalHints struct {
	TimeoutsMs          map[string]int `yaml:"timeouts_ms"`          
	FrequenciesPerMinute map[string]int `yaml:"frequencies_per_minute"` 
}

type Spec struct {
	Version            int                `yaml:"version"`
	System             string             `yaml:"system"`
	Metadata           map[string]any     `yaml:"metadata"`
	Services           []Service          `yaml:"services"`
	Datastores         []Datastore        `yaml:"datastores"`
	Calls              []Call             `yaml:"calls"`
	NonFunctionalHints NonFunctionalHints `yaml:"non_functional_hints"`
	ExpectIssues       []string           `yaml:"expect_issues"`
}

func parseSpec(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Spec
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

//IN-MEM GRAPH 

type CallEdge struct {
	From, To           string
	Sync               bool
	Endpoint, Mode     string
	Topic              string
	TimeoutMs          int
	FreqPerMin         int
}

type DBEdge struct {
	Service string
	DB      string
	Access  string 
}

type Graph struct {
	Services   map[string]struct{}
	Datastores map[string]struct{}
	Calls      []CallEdge
	Accesses   []DBEdge
}

func buildGraph(s *Spec) *Graph {
	g := &Graph{
		Services:   make(map[string]struct{}),
		Datastores: make(map[string]struct{}),
	}

	
	for _, d := range s.Datastores {
		g.Datastores[d.Name] = struct{}{}
	}

	// services + db accesses
	for _, svc := range s.Services {
		g.Services[svc.Name] = struct{}{}
		for _, db := range svc.Databases {
			
			g.Datastores[db.Name] = struct{}{}
			g.Accesses = append(g.Accesses, DBEdge{
				Service: svc.Name, DB: db.Name, Access: db.Access,
			})
		}
	}

	
	tout := map[string]int{}
	for k, v := range s.NonFunctionalHints.TimeoutsMs {
		tout[normalizeKey(k)] = v
	}
	freq := map[string]int{}
	for k, v := range s.NonFunctionalHints.FrequenciesPerMinute {
		freq[normalizeKey(k)] = v
	}

	
	for _, c := range s.Calls {
		key := normalizeKey(fmt.Sprintf("%s->%s", c.From, c.To))
		g.Calls = append(g.Calls, CallEdge{
			From: c.From, To: c.To, Sync: c.Sync,
			Endpoint: c.Endpoint, Mode: c.Mode, Topic: c.Topic,
			TimeoutMs:  tout[key],
			FreqPerMin: freq[key],
		})
		
		g.Services[c.From] = struct{}{}
		g.Services[c.To] = struct{}{}
	}

	return g
}

func normalizeKey(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func (g *Graph) validate(s *Spec) []string {
	var warns []string
	
	for _, e := range g.Calls {
		if e.From == e.To {
			warns = append(warns, "self-call at service "+e.From)
		}
	}
	
	declared := map[string]struct{}{}
	for _, d := range s.Datastores {
		declared[d.Name] = struct{}{}
	}
	for _, a := range g.Accesses {
		if _, ok := declared[a.DB]; !ok {
			warns = append(warns, "datastore '"+a.DB+"' referenced but not declared in .datastores")
		}
	}
	return warns
}

func (g *Graph) toDOT() string {
	var b strings.Builder
	b.WriteString("digraph G {\n  rankdir=LR;\n")
	b.WriteString("  node [style=rounded];\n")

	
	var svcNames []string
	for n := range g.Services {
		svcNames = append(svcNames, n)
	}
	sort.Strings(svcNames)
	for _, n := range svcNames {
		b.WriteString(fmt.Sprintf("  \"%s\" [shape=box];\n", n))
	}

	
	var dbNames []string
	for n := range g.Datastores {
		dbNames = append(dbNames, n)
	}
	sort.Strings(dbNames)
	for _, n := range dbNames {
		b.WriteString(fmt.Sprintf("  \"%s\" [shape=cylinder];\n", n))
	}

	
	for _, e := range g.Calls {
		lbl := ""
		if e.Endpoint != "" {
			lbl = e.Endpoint
		} else if e.Topic != "" {
			lbl = e.Topic
		}
		if e.Mode != "" {
			if lbl != "" {
				lbl += " "
			}
			lbl += "(" + e.Mode + ")"
		}
		if e.TimeoutMs > 0 || e.FreqPerMin > 0 {
			parts := []string{}
			if e.TimeoutMs > 0 {
				parts = append(parts, fmt.Sprintf("t=%dms", e.TimeoutMs))
			}
			if e.FreqPerMin > 0 {
				parts = append(parts, fmt.Sprintf("f=%d/min", e.FreqPerMin))
			}
			if lbl != "" {
				lbl += " "
			}
			lbl += "[" + strings.Join(parts, ", ") + "]"
		}
		style := "solid"
		if !e.Sync {
			style = "dashed"
		}
		if lbl != "" {
			b.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [label=\"%s\", style=%s];\n", e.From, e.To, lbl, style))
		} else {
			b.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [style=%s];\n", e.From, e.To, style))
		}
	}

	
	for _, a := range g.Accesses {
		lbl := "R"
		if strings.EqualFold(a.Access, "readwrite") {
			lbl = "RW"
		}
		b.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [style=dotted, label=\"%s\"];\n", a.Service, a.DB, lbl))
	}

	b.WriteString("}\n")
	return b.String()
}

// NEO4J LOADER

// Anti-pattern detection

type Issue struct {
	Type      string
	Severity  string
	Details   string
	Services  []string
	Datastore string
	Edge      *CallEdge 
}

// cycles via Tarjan SCC

func detectCycles(g *Graph) []Issue {
	n := 0
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var issues []Issue


	adj := map[string][]string{}
	for s := range g.Services { adj[s] = nil }
	for _, c := range g.Calls { adj[c.From] = append(adj[c.From], c.To) }

	var strongConnect func(v string)
	strongConnect = func(v string) {
		index[v] = n
		low[v] = n
		n++
		stack = append(stack, v); onStack[v] = true

		for _, w := range adj[v] {
			if _, seen := index[w]; !seen {
				strongConnect(w)
				if low[w] < low[v] { low[v] = low[w] }
			} else if onStack[w] {
				if index[w] < low[v] { low[v] = index[w] }
			}
		}

		if low[v] == index[v] {
			
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v { break }
			}
			if len(scc) >= 2 {
				issues = append(issues, Issue{
					Type:     "cycle",
					Severity: "high",
					Details:  fmt.Sprintf("cycle among: %v", scc),
					Services: scc,
				})
			}
		}
	}

	for s := range g.Services {
		if _, seen := index[s]; !seen {
			strongConnect(s)
		}
	}
	return issues
}

func detectGodServices(g *Graph, threshold int) []Issue {
	out := map[string]int{}
	for s := range g.Services { out[s] = 0 }
	for _, c := range g.Calls { out[c.From]++ }

	var issues []Issue
	for s, deg := range out {
		if deg >= threshold {
			issues = append(issues, Issue{
				Type:     "god-service",
				Severity: "medium",
				Details:  fmt.Sprintf("%s high fan-out: %d calls", s, deg),
				Services: []string{s},
			})
		}
	}
	return issues
}

func detectSharedDbWrites(g *Graph) []Issue {
	writers := map[string][]string{} // db -> services with RW
	for _, a := range g.Accesses {
		if strings.EqualFold(a.Access, "readwrite") {
			writers[a.DB] = append(writers[a.DB], a.Service)
		}
	}
	var issues []Issue
	for db, svcs := range writers {
		if len(svcs) >= 2 {
			issues = append(issues, Issue{
				Type:      "shared-db-writes",
				Severity:  "high",
				Details:   fmt.Sprintf("multiple writers to %s: %v", db, svcs),
				Services:  svcs,
				Datastore: db,
			})
		}
	}
	return issues
}

func detectCrossDbReads(g *Graph) []Issue {
	writers := map[string]map[string]bool{} // db -> set of writer svcs
	for _, a := range g.Accesses {
		if strings.EqualFold(a.Access, "readwrite") {
			if writers[a.DB] == nil { writers[a.DB] = map[string]bool{} }
			writers[a.DB][a.Service] = true
		}
	}
	var issues []Issue
	for _, a := range g.Accesses {
		if strings.EqualFold(a.Access, "readonly") {
			owners := writers[a.DB]
			if len(owners) > 0 && !owners[a.Service] {
				var ownerList []string
				for s := range owners { ownerList = append(ownerList, s) }
				issues = append(issues, Issue{
					Type:      "cross-db-read",
					Severity:  "medium",
					Details:   fmt.Sprintf("%s reads %s owned by %v", a.Service, a.DB, ownerList),
					Services:  []string{a.Service},
					Datastore: a.DB,
				})
			}
		}
	}
	return issues
}

func detectChattyCalls(g *Graph, freqThreshold int) []Issue {
	var issues []Issue
	for _, e := range g.Calls {
		e := e // make a copy so &e is stable for this iteration
		if strings.EqualFold(e.Mode, "per-item") || e.FreqPerMin >= freqThreshold {
			sev := "medium"
			if e.FreqPerMin >= freqThreshold*2 { sev = "high" }
			issues = append(issues, Issue{
				Type:     "chatty-call",
				Severity: sev,
				Details:  fmt.Sprintf("%s -> %s %s f=%d/min", e.From, e.To, e.Mode, e.FreqPerMin),
				Services: []string{e.From, e.To},
				Edge:     &e,
			})
		}
	}
	return issues
}


func detectRedundantResponsibilities(sp *Spec) []Issue {
	byResp := map[string]map[string]bool{} // resp -> set of services
	for _, svc := range sp.Services {
		for _, r := range svc.Responsibilities {
			rn := strings.ToLower(r)
			if byResp[rn] == nil { byResp[rn] = map[string]bool{} }
			byResp[rn][svc.Name] = true
		}
	}
	var issues []Issue
	for resp, svcsSet := range byResp {
		if len(svcsSet) >= 2 {
			var svcs []string
			for s := range svcsSet { svcs = append(svcs, s) }
			issues = append(issues, Issue{
				Type:     "redundant-responsibility",
				Severity: "low",
				Details:  fmt.Sprintf("responsibility '%s' duplicated across %v", resp, svcs),
				Services: svcs,
			})
		}
	}
	return issues
}

func detectAll(sp *Spec, g *Graph) []Issue {
	var all []Issue
	all = append(all, detectCycles(g)...)
	all = append(all, detectGodServices(g, 5)...)
	all = append(all, detectSharedDbWrites(g)...)
	all = append(all, detectCrossDbReads(g)...)
	all = append(all, detectChattyCalls(g, 300)...)
	all = append(all, detectRedundantResponsibilities(sp)...)
	return all
}



func ensureSchema(ctx context.Context, sess neo4j.SessionWithContext) error {
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		
		if _, err := tx.Run(ctx,
			"CREATE CONSTRAINT IF NOT EXISTS FOR (s:Service) REQUIRE s.name IS UNIQUE", nil); err != nil {
			return nil, fmt.Errorf("constraint service: %w", err)
		}
		if _, err := tx.Run(ctx,
			"CREATE CONSTRAINT IF NOT EXISTS FOR (d:Datastore) REQUIRE d.name IS UNIQUE", nil); err != nil {
			return nil, fmt.Errorf("constraint datastore: %w", err)
		}
		if _, err := tx.Run(ctx,
  			"CREATE CONSTRAINT IF NOT EXISTS FOR (i:Issue) REQUIRE i.key IS UNIQUE", nil); err != nil {
  			return nil, fmt.Errorf("constraint issue: %w", err)
		}
		return nil, nil
	})
	return err
}

func loadToNeo4j(ctx context.Context, drv neo4j.DriverWithContext, g *Graph) error {
	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)


	if err := ensureSchema(ctx, sess); err != nil {
		return fmt.Errorf("schema: %w", err)
	}


	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		
		svcs := make([]string, 0, len(g.Services))
		for name := range g.Services {
			svcs = append(svcs, name)
		}
		if _, err := tx.Run(ctx,
			"UNWIND $names AS n MERGE (:Service {name: n})",
			map[string]any{"names": svcs},
		); err != nil {
			return nil, fmt.Errorf("services: %w", err)
		}

	
		dbs := make([]string, 0, len(g.Datastores))
		for name := range g.Datastores {
			dbs = append(dbs, name)
		}
		if _, err := tx.Run(ctx,
			"UNWIND $names AS n MERGE (:Datastore {name: n})",
			map[string]any{"names": dbs},
		); err != nil {
			return nil, fmt.Errorf("datastores: %w", err)
		}

	
		calls := make([]map[string]any, 0, len(g.Calls))
		for _, e := range g.Calls {
			calls = append(calls, map[string]any{
				"From":       e.From,
				"To":         e.To,
				"Sync":       e.Sync,
				"Endpoint":   e.Endpoint,
				"Mode":       e.Mode,
				"Topic":      e.Topic,
				"TimeoutMs":  int64(e.TimeoutMs),
				"FreqPerMin": int64(e.FreqPerMin),
			})
		}
		if _, err := tx.Run(ctx, `
UNWIND $calls AS e
MATCH (a:Service {name: e.From}), (b:Service {name: e.To})
MERGE (a)-[r:CALLS]->(b)
SET r.sync = e.Sync,
    r.endpoint = e.Endpoint,
    r.mode = e.Mode,
    r.topic = e.Topic,
    r.timeout_ms = e.TimeoutMs,
    r.freq_per_min = e.FreqPerMin
`, map[string]any{"calls": calls}); err != nil {
			return nil, fmt.Errorf("CALLS: %w", err)
		}


		acc := make([]map[string]any, 0, len(g.Accesses))
		for _, a := range g.Accesses {
			acc = append(acc, map[string]any{
				"Svc":    a.Service,
				"DB":     a.DB,
				"Access": a.Access,
			})
		}
		if _, err := tx.Run(ctx, `
UNWIND $acc AS a
MATCH (s:Service {name: a.Svc}), (d:Datastore {name: a.DB})
MERGE (s)-[r:ACCESSES]->(d)
SET r.access = a.Access
`, map[string]any{"acc": acc}); err != nil {
			return nil, fmt.Errorf("ACCESSES: %w", err)
		}

		return nil, nil
	})
	return err
}

func loadIssuesToNeo4j(ctx context.Context, drv neo4j.DriverWithContext, issues []Issue) error {
	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)


	rows := make([]map[string]any, 0, len(issues))
	for _, is := range issues {
		key := is.Type + "|" + strings.Join(is.Services, ",") + "|" + is.Datastore
		row := map[string]any{
			"key":       key,
			"type":      is.Type,
			"severity":  is.Severity,
			"details":   is.Details,
			"services":  is.Services,
			"datastore": is.Datastore,
			"edgeFrom":  nil,
			"edgeTo":    nil,
			"endpoint":  nil,
			"mode":      nil,
			"freq":      nil,
			"timeout":   nil,
		}
		if is.Edge != nil {
			row["edgeFrom"] = is.Edge.From
			row["edgeTo"] = is.Edge.To
			if is.Edge.Endpoint != "" { row["endpoint"] = is.Edge.Endpoint }
			if is.Edge.Mode != ""     { row["mode"]     = is.Edge.Mode }
			if is.Edge.FreqPerMin > 0 { row["freq"]     = int64(is.Edge.FreqPerMin) }
			if is.Edge.TimeoutMs  > 0 { row["timeout"]  = int64(is.Edge.TimeoutMs) }
		}
		rows = append(rows, row)
	}

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
	
		if _, err := tx.Run(ctx, `
UNWIND $rows AS r
MERGE (i:Issue {key: r.key})
SET i.type = r.type,
    i.severity = r.severity,
    i.details = r.details,
    i.services = r.services,
    i.datastore = r.datastore
`, map[string]any{"rows": rows}); err != nil {
			return nil, fmt.Errorf("issue nodes: %w", err)
		}

	
		if _, err := tx.Run(ctx, `
UNWIND $rows AS r
UNWIND r.services AS sname
MATCH (s:Service {name: sname}), (i:Issue {key: r.key})
MERGE (s)-[:HAS_ISSUE]->(i)
`, map[string]any{"rows": rows}); err != nil {
			return nil, fmt.Errorf("issue svc links: %w", err)
		}

		
		if _, err := tx.Run(ctx, `
UNWIND $rows AS r
WITH r WHERE r.datastore IS NOT NULL AND r.datastore <> ""
MATCH (d:Datastore {name: r.datastore}), (i:Issue {key: r.key})
MERGE (i)-[:INVOLVES]->(d)
`, map[string]any{"rows": rows}); err != nil {
			return nil, fmt.Errorf("issue db links: %w", err)
		}

	
if _, err := tx.Run(ctx, `
UNWIND $rows AS r
WITH r WHERE r.edgeFrom IS NOT NULL AND r.edgeTo IS NOT NULL
MATCH (a:Service {name: r.edgeFrom}),
      (b:Service {name: r.edgeTo}),
      (i:Issue {key: r.key})
MERGE (i)-[:RELATES_FROM]->(a)
MERGE (i)-[:RELATES_TO]->(b)
`, map[string]any{"rows": rows}); err != nil {
    return nil, fmt.Errorf("issue call links: %w", err)
}

		return nil, nil
	})
	return err
}



// MAIN

func fatalf(msg string, a ...any) {
	fmt.Fprintf(os.Stderr, "error: "+msg+"\n", a...)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: worker <spec.yaml>")
	}
	specPath := os.Args[1]
	if _, err := os.Stat(specPath); err != nil {
		fatalf("cannot read spec file: %v", err)
	}

	// parse + build
	sp, err := parseSpec(specPath)
	if err != nil {
		fatalf("parse spec: %v", err)
	}
	g := buildGraph(sp)
	for _, w := range g.validate(sp) {
		fmt.Println("Warn:", w)
	}
	issues := detectAll(sp, g)
	fmt.Printf("Detected %d issues\n", len(issues))
	for _, is := range issues {
		fmt.Printf(" - [%s] %s\n", is.Type, is.Details)
	}

	// export DOT (+PNG if Graphviz installed)
	_ = os.MkdirAll("out", 0o755)
	dotPath := filepath.Join("out", "graph.dot")
	if err := os.WriteFile(dotPath, []byte(g.toDOT()), 0o644); err != nil {
		fatalf("write dot: %v", err)
	}
	if _, err := exec.LookPath("dot"); err == nil {
		_ = exec.Command("dot", "-Tpng", dotPath, "-o", filepath.Join("out", "graph.png")).Run()
	}
	fmt.Println("Wrote:", dotPath)

	// connect + load Neo4j
	uri, user, pass := os.Getenv("NEO4J_URI"), os.Getenv("NEO4J_USER"), os.Getenv("NEO4J_PASS")
	if uri == "" || user == "" || pass == "" {
		fatalf("NEO4J_URI/NEO4J_USER/NEO4J_PASS must be set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	drv, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, pass, ""))
	if err != nil {
		fatalf("neo4j driver: %v", err)
	}
	defer drv.Close(ctx)

	if err := loadToNeo4j(ctx, drv, g); err != nil {
		fatalf("load into Neo4j: %v", err)
	}
	if err := loadIssuesToNeo4j(ctx, drv, issues); err != nil {
	fatalf("load issues: %v", err)
	}
	fmt.Println("Loaded issues into Neo4j ✔")
}
