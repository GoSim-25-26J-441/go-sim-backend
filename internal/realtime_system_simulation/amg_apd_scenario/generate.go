package amg_apd_scenario

import (
	"fmt"
	"strings"

	simconfig "github.com/GoSim-25-26J-441/simulation-core/pkg/config"
	"gopkg.in/yaml.v3"
)

const defaultRPS = 10.0

type depEdge struct {
	from, to string
	sync     bool
	kind     string
}

type svcInfo struct {
	id        string
	rawType   string
	role      string
	isDB      bool
	isIngress bool
}

// GenerateFromAMGAPDYAML parses AMG/APD diagram YAML and produces a simulation-core Scenario.
func GenerateFromAMGAPDYAML(amgYAML []byte) (*simconfig.Scenario, error) {
	var root map[string]any
	if err := yaml.Unmarshal(amgYAML, &root); err != nil {
		return nil, fmt.Errorf("parse AMG/APD YAML: %w", err)
	}

	servicesRaw, _ := root["services"].([]any)
	if len(servicesRaw) == 0 {
		return nil, fmt.Errorf("AMG/APD YAML has no services")
	}

	byID := make(map[string]svcInfo)
	order := make([]string, 0, len(servicesRaw))
	for _, s := range servicesRaw {
		sm, ok := s.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(fmt.Sprint(sm["id"]))
		if id == "" {
			continue
		}
		id = normalizeID(id)
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(sm["type"])))
		role := strings.ToLower(strings.TrimSpace(fmt.Sprint(sm["role"])))
		isDB := strings.Contains(typ, "database") || strings.Contains(typ, "datastore") || strings.Contains(typ, "db") || strings.Contains(typ, "postgres") || strings.Contains(typ, "mysql")
		isIngress := typ == "api_gateway" || typ == "bff" || typ == "gateway" || typ == "web-ui" || typ == "web_ui" ||
			strings.Contains(typ, "gateway") || role == "ingress" || role == "bff"
		byID[id] = svcInfo{id: id, rawType: typ, role: role, isDB: isDB, isIngress: isIngress}
		order = append(order, id)
	}
	if len(byID) == 0 {
		return nil, fmt.Errorf("no valid service ids in AMG/APD YAML")
	}

	// Datastores as DB services (register before dependencies so edges can target them).
	if ds, ok := root["datastores"].([]any); ok {
		for _, x := range ds {
			dm, ok := x.(map[string]any)
			if !ok {
				continue
			}
			id := normalizeID(strings.TrimSpace(fmt.Sprint(dm["id"])))
			if id == "" {
				continue
			}
			if _, exists := byID[id]; exists {
				continue
			}
			byID[id] = svcInfo{id: id, rawType: "database", isDB: true}
			order = append(order, id)
		}
	}

	incoming := make(map[string]int)
	for id := range byID {
		incoming[id] = 0
	}
	var deps []depEdge
	if dr, ok := root["dependencies"].([]any); ok {
		for _, d := range dr {
			dm, ok := d.(map[string]any)
			if !ok {
				continue
			}
			from := normalizeID(strings.TrimSpace(fmt.Sprint(dm["from"])))
			to := normalizeID(strings.TrimSpace(fmt.Sprint(dm["to"])))
			if from == "" || to == "" {
				continue
			}
			if _, ok := byID[from]; !ok {
				continue
			}
			if _, ok := byID[to]; !ok {
				continue
			}
			sync := true
			if v, ok := dm["sync"].(bool); ok {
				sync = v
			}
			kind := strings.ToLower(strings.TrimSpace(fmt.Sprint(dm["kind"])))
			if kind == "" {
				kind = "rest"
			}
			deps = append(deps, depEdge{from: from, to: to, sync: sync, kind: kind})
			incoming[to]++
		}
	}

	rps := defaultRPS
	if cfg, ok := root["configs"].(map[string]any); ok {
		if slo, ok := cfg["slo"].(map[string]any); ok {
			if v, ok := slo["target_rps"].(float64); ok && v > 0 {
				rps = v
			} else if v, ok := slo["target_rps"].(int); ok && v > 0 {
				rps = float64(v)
			}
		}
	}

	entry := pickEntrypoint(byID, incoming, order)
	if entry == "" {
		entry = order[0]
	}

	hosts := []simconfig.Host{
		{ID: "host-1", Cores: 8, MemoryGB: 32},
		{ID: "host-2", Cores: 8, MemoryGB: 32},
		{ID: "host-3", Cores: 8, MemoryGB: 32},
	}

	outgoing := make(map[string][]depEdge)
	for _, d := range deps {
		outgoing[d.from] = append(outgoing[d.from], d)
	}

	var simServices []simconfig.Service
	for _, id := range order {
		info := byID[id]
		model := "cpu"
		if info.isDB {
			model = "db_latency"
		}
		replicas := 2
		cpu := 1.0
		mem := 512.0
		if info.isIngress {
			cpu = 1.0
			mem = 768.0
		}
		if info.isDB {
			replicas = 1
			cpu = 0.5
			mem = 1024.0
		}

		var endpoints []simconfig.Endpoint
		if info.isIngress {
			ds := buildDownstreamsForIngress(id, outgoing[id], byID)
			endpoints = []simconfig.Endpoint{ingressEndpoint(ds)}
		} else if info.isDB {
			endpoints = dbEndpoints(outgoing[id], byID)
		} else {
			endpoints = crudEndpoints(outgoing[id], byID)
		}

		simServices = append(simServices, simconfig.Service{
			ID:        id,
			Replicas:  replicas,
			Model:     model,
			CPUCores:  cpu,
			MemoryMB:  mem,
			Endpoints: endpoints,
		})
	}

	workload := []simconfig.WorkloadPattern{
		{
			From: "client",
			To:   fmt.Sprintf("%s:%s", entry, workloadPathForEntry(byID[entry])),
			Arrival: simconfig.ArrivalSpec{
				Type:    "poisson",
				RateRPS: rps,
			},
		},
	}

	return &simconfig.Scenario{
		Hosts:    hosts,
		Services: simServices,
		Workload: workload,
	}, nil
}

func normalizeID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func pickEntrypoint(byID map[string]svcInfo, incoming map[string]int, order []string) string {
	for _, id := range order {
		info := byID[id]
		if info.isIngress && incoming[id] == 0 {
			return id
		}
	}
	for _, id := range order {
		if byID[id].isIngress {
			return id
		}
	}
	for _, id := range order {
		if incoming[id] == 0 {
			return id
		}
	}
	return ""
}

func workloadPathForEntry(info svcInfo) string {
	if info.isIngress {
		return "/ingress"
	}
	return "/read"
}

func netLatency() simconfig.LatencySpec {
	return simconfig.LatencySpec{Mean: 2, Sigma: 0.5}
}

func callLat() simconfig.LatencySpec {
	return simconfig.LatencySpec{Mean: 4, Sigma: 1}
}

func ingressEndpoint(down []simconfig.DownstreamCall) simconfig.Endpoint {
	return simconfig.Endpoint{
		Path:            "/ingress",
		MeanCPUMs:       5,
		CPUSigmaMs:      1.5,
		DefaultMemoryMB: 16,
		NetLatencyMs:    netLatency(),
		Downstream:      down,
	}
}

func crudEndpoints(deps []depEdge, byID map[string]svcInfo) []simconfig.Endpoint {
	paths := []string{"/create", "/read", "/update", "/delete"}
	var eps []simconfig.Endpoint
	for _, p := range paths {
		var ds []simconfig.DownstreamCall
		if p == "/read" {
			ds = downstreamCalls(deps, byID)
		}
		eps = append(eps, simconfig.Endpoint{
			Path:            p,
			MeanCPUMs:       8,
			CPUSigmaMs:      2,
			DefaultMemoryMB: 20,
			NetLatencyMs:    netLatency(),
			Downstream:      ds,
		})
	}
	return eps
}

func dbEndpoints(deps []depEdge, byID map[string]svcInfo) []simconfig.Endpoint {
	paths := []string{"/query", "/write"}
	var eps []simconfig.Endpoint
	for _, p := range paths {
		var ds []simconfig.DownstreamCall
		if p == "/query" {
			ds = downstreamCalls(deps, byID)
		}
		eps = append(eps, simconfig.Endpoint{
			Path:            p,
			MeanCPUMs:       6,
			CPUSigmaMs:      2,
			DefaultMemoryMB: 32,
			NetLatencyMs:    netLatency(),
			Downstream:      ds,
		})
	}
	return eps
}

func downstreamCalls(deps []depEdge, byID map[string]svcInfo) []simconfig.DownstreamCall {
	var out []simconfig.DownstreamCall
	for _, d := range deps {
		targetPath := targetEndpointPath(byID[d.to])
		out = append(out, simconfig.DownstreamCall{
			To:            fmt.Sprintf("%s:%s", d.to, targetPath),
			CallCountMean: 1,
			CallLatencyMs: callLat(),
		})
	}
	return out
}

func targetEndpointPath(info svcInfo) string {
	if info.isDB {
		return "/query"
	}
	return "/read"
}

func buildDownstreamsForIngress(from string, deps []depEdge, byID map[string]svcInfo) []simconfig.DownstreamCall {
	var out []simconfig.DownstreamCall
	for _, d := range deps {
		if d.from != from {
			continue
		}
		tpath := targetEndpointPath(byID[d.to])
		out = append(out, simconfig.DownstreamCall{
			To:            fmt.Sprintf("%s:%s", d.to, tpath),
			CallCountMean: 1,
			CallLatencyMs: callLat(),
		})
	}
	return out
}

// GenerateScenarioYAML returns validated simulation-core scenario YAML bytes.
func GenerateScenarioYAML(amgYAML []byte) (string, error) {
	sc, err := GenerateFromAMGAPDYAML(amgYAML)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(sc)
	if err != nil {
		return "", fmt.Errorf("marshal scenario: %w", err)
	}
	if _, err := simconfig.ParseScenarioYAML(data); err != nil {
		return "", fmt.Errorf("invalid generated scenario: %w", err)
	}
	return string(data), nil
}
