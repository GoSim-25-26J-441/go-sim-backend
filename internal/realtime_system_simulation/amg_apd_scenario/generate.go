// Package amg_apd_scenario converts AMG/APD diagram YAML into scenario-v2 draft YAML (local DTOs).
// Authoritative validation and preflight are performed by simulation-core over HTTP.
package amg_apd_scenario

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultRPS = 10.0

// firstStringField returns the first non-empty trimmed string among the given map keys.
// Non-string or nil values are ignored; fmt.Sprint is not used (avoids "<nil>" for missing fields).
func firstStringField(m map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

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
	isIngress bool   // gateway-shaped entry (api_gateway kind): single /ingress endpoint
	simKind   string // api_gateway, service, database, queue, topic
	roleField string // YAML role: ingress when client-facing heuristic applies
}

// GenerateFromAMGAPDYAML parses AMG/APD diagram YAML and produces a scenario draft.
// The returned warnings describe skipped dependencies (unknown endpoints); they are not errors.
func GenerateFromAMGAPDYAML(amgYAML []byte) (*ScenarioDoc, []string, error) {
	var root map[string]any
	if err := yaml.Unmarshal(amgYAML, &root); err != nil {
		return nil, nil, fmt.Errorf("parse AMG/APD YAML: %w", err)
	}

	var warnings []string

	servicesRaw, _ := root["services"].([]any)
	if len(servicesRaw) == 0 {
		return nil, nil, fmt.Errorf("AMG/APD YAML has no services")
	}

	byID := make(map[string]svcInfo)
	externalClients := make(map[string]struct{})
	order := make([]string, 0, len(servicesRaw))
	seen := make(map[string]struct{})
	for i, s := range servicesRaw {
		sm, ok := s.(map[string]any)
		if !ok {
			continue
		}
		rawID := firstStringField(sm, "id", "name")
		if rawID == "" {
			return nil, nil, fmt.Errorf("AMG/APD services[%d]: missing required id or name", i)
		}
		id := normalizeID(rawID)
		if _, dup := seen[id]; dup {
			return nil, nil, fmt.Errorf("duplicate AMG/APD service name/id %q", rawID)
		}
		seen[id] = struct{}{}
		typ := strings.ToLower(firstStringField(sm, "type", "kind"))
		role := strings.ToLower(firstStringField(sm, "role"))
		if isExternalClient(typ, role) {
			externalClients[id] = struct{}{}
			continue
		}
		isDB := strings.Contains(typ, "database") || strings.Contains(typ, "datastore") ||
			strings.Contains(typ, "postgres") || strings.Contains(typ, "mysql") ||
			strings.Contains(typ, "db")
		isQueue := typ == "queue" || strings.Contains(typ, "message_queue") || strings.Contains(typ, "sqs")
		isTopic := typ == "topic" || strings.Contains(typ, "kafka") || strings.Contains(typ, "pubsub") || typ == "event_stream"
		gwShape := ingressTypeOrRole(typ, role)
		nameIngress := ingressNameHeuristic(id) && !gwShape && typ == "service"
		simKind, roleField, isIngress := classifyService(typ, role, id, isDB, isQueue, isTopic, gwShape, nameIngress)
		byID[id] = svcInfo{
			id: id, rawType: typ, role: role, isDB: isDB, isIngress: isIngress,
			simKind: simKind, roleField: roleField,
		}
		order = append(order, id)
	}
	if len(byID) == 0 {
		return nil, nil, fmt.Errorf("no valid service ids in AMG/APD YAML")
	}

	for _, id := range order {
		info := byID[id]
		if (info.simKind == "queue" || info.simKind == "topic") && firstConsumerEndpoint(order, id, byID) == "" {
			return nil, warnings, fmt.Errorf("AMG/APD service %q (kind %s) needs at least one non-queue/topic/database application service for consumer_target", id, info.simKind)
		}
	}

	if ds, ok := root["datastores"].([]any); ok {
		for _, x := range ds {
			dm, ok := x.(map[string]any)
			if !ok {
				continue
			}
			rawID := firstStringField(dm, "id", "name")
			if rawID == "" {
				continue
			}
			id := normalizeID(rawID)
			if id == "" {
				continue
			}
			if _, exists := byID[id]; exists {
				continue
			}
			byID[id] = svcInfo{id: id, rawType: "database", isDB: true, simKind: "database"}
			order = append(order, id)
		}
	}

	if tops, ok := root["topics"].([]any); ok {
		for _, x := range tops {
			tm, ok := x.(map[string]any)
			if !ok {
				continue
			}
			rawID := firstStringField(tm, "id", "name")
			if rawID == "" {
				continue
			}
			id := normalizeID(rawID)
			if id == "" {
				continue
			}
			if _, exists := byID[id]; exists {
				continue
			}
			byID[id] = svcInfo{id: id, rawType: "topic", simKind: "topic"}
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
			from := normalizeID(firstStringField(dm, "from"))
			to := normalizeID(firstStringField(dm, "to"))
			if from == "" || to == "" {
				continue
			}
			if _, isClient := externalClients[from]; isClient {
				continue
			}
			_, fromOK := byID[from]
			_, toOK := byID[to]
			if !fromOK {
				warnings = append(warnings, fmt.Sprintf("skipped dependency %q -> %q: unknown from service", from, to))
				continue
			}
			if !toOK {
				warnings = append(warnings, fmt.Sprintf("skipped dependency %q -> %q: unknown to service", from, to))
				continue
			}
			sync := true
			if v, ok := dm["sync"].(bool); ok {
				sync = v
			}
			kind := strings.ToLower(firstStringField(dm, "kind"))
			if kind == "" {
				kind = "rest"
			}
			deps = append(deps, depEdge{from: from, to: to, sync: sync, kind: kind})
			incoming[to]++
		}
	}

	rps := parseTargetRPS(root)
	entry := pickEntrypoint(byID, incoming, order)
	if entry == "" {
		entry = order[0]
	}

	hosts := []HostDoc{
		{ID: "host-1", Cores: 8, MemoryGB: 32},
		{ID: "host-2", Cores: 8, MemoryGB: 32},
		{ID: "host-3", Cores: 8, MemoryGB: 32},
	}

	outgoing := make(map[string][]depEdge)
	for _, d := range deps {
		outgoing[d.from] = append(outgoing[d.from], d)
	}

	var simServices []ServiceDoc
	for _, id := range order {
		info := byID[id]
		model := "cpu"
		if info.simKind == "database" {
			model = "db_latency"
		}
		replicas := 2
		cpu := 1.0
		mem := 512.0
		if info.isIngress {
			cpu = 1.0
			mem = 768.0
		}
		if info.simKind == "database" {
			replicas = 1
			cpu = 0.5
			mem = 1024.0
		}

		var endpoints []EndpointDoc
		switch info.simKind {
		case "api_gateway":
			ds := buildDownstreamsForIngress(id, outgoing[id], byID)
			endpoints = []EndpointDoc{ingressEndpoint(ds)}
		case "queue":
			endpoints = queueEndpoints()
		case "topic":
			endpoints = topicEndpoints()
		case "database":
			endpoints = dbEndpoints(outgoing[id], byID)
		default:
			endpoints = crudEndpoints(outgoing[id], byID)
		}

		svc := ServiceDoc{
			ID:        id,
			Kind:      info.simKind,
			Role:      info.roleField,
			Replicas:  replicas,
			Model:     model,
			CPUCores:  cpu,
			MemoryMB:  mem,
			Scaling:   defaultScaling(info.simKind),
			Behavior:  behaviorForService(id, order, byID),
			Routing:   defaultServiceRouting(info.simKind),
			Endpoints: endpoints,
			Placement: nil,
		}
		simServices = append(simServices, svc)
	}

	workload := []WorkloadDoc{
		{
			From: "client",
			To:   fmt.Sprintf("%s:%s", entry, workloadPathForEntry(byID[entry])),
			Arrival: ArrivalDoc{
				Type:    "poisson",
				RateRPS: rps,
			},
		},
	}

	doc := &ScenarioDoc{
		Metadata: map[string]any{"schema_version": "0.2.0"},
		Hosts:    hosts,
		Services: simServices,
		Workload: workload,
	}
	if err := ValidateScenarioDraft(doc); err != nil {
		return nil, warnings, err
	}
	return doc, warnings, nil
}

func classifyService(typ, role, id string, isDB, isQueue, isTopic, gwShape, nameIngress bool) (simKind, roleField string, isIngress bool) {
	switch {
	case isQueue:
		return "queue", "", false
	case isTopic:
		return "topic", "", false
	case isDB:
		return "database", "datastore", false
	case gwShape:
		return "api_gateway", "ingress", true
	case nameIngress:
		return "service", "ingress", false
	default:
		return "service", "", false
	}
}

func isExternalClient(typ, role string) bool {
	return typ == "client" || typ == "external_client" || role == "client"
}

func defaultScaling(simKind string) *ScalingDoc {
	switch simKind {
	case "database":
		return &ScalingDoc{
			Horizontal:     false,
			VerticalCPU:    true,
			VerticalMemory: true,
		}
	default:
		return &ScalingDoc{
			Horizontal:     true,
			VerticalCPU:    true,
			VerticalMemory: true,
		}
	}
}

func defaultServiceRouting(simKind string) *RoutingDoc {
	if simKind == "api_gateway" {
		return &RoutingDoc{Strategy: "least_queue"}
	}
	return nil
}

func behaviorForService(id string, order []string, byID map[string]svcInfo) *BehaviorDoc {
	info := byID[id]
	ct := firstConsumerEndpoint(order, id, byID)
	switch info.simKind {
	case "queue":
		if ct == "" {
			return nil
		}
		return &BehaviorDoc{
			Queue: &QueueBehaviorDoc{
				Capacity:            5000,
				ConsumerConcurrency: 2,
				ConsumerTarget:      ct,
				DeliveryLatencyMs:   LatencyDoc{Mean: 1, Sigma: 0},
				AckTimeoutMs:        30000,
				MaxRedeliveries:     3,
				DropPolicy:          "reject",
			},
		}
	case "topic":
		if ct == "" {
			return nil
		}
		return &BehaviorDoc{
			Topic: &TopicBehaviorDoc{
				Partitions:        3,
				Capacity:          10000,
				DeliveryLatencyMs: LatencyDoc{Mean: 1, Sigma: 0},
				Subscribers: []TopicSubscriberDoc{
					{
						Name:            "default-subscriber",
						ConsumerTarget:  ct,
						ConsumerGroup:   fmt.Sprintf("%s-default", id),
						DropPolicy:      "reject",
						MaxRedeliveries: 3,
					},
				},
			},
		}
	default:
		return nil
	}
}

func firstConsumerEndpoint(order []string, selfID string, byID map[string]svcInfo) string {
	for _, oid := range order {
		if oid == selfID {
			continue
		}
		inf := byID[oid]
		if inf.simKind == "database" || inf.simKind == "queue" || inf.simKind == "topic" {
			continue
		}
		return oid + ":/read"
	}
	return ""
}

func ingressTypeOrRole(typ, role string) bool {
	if role == "ingress" || role == "bff" {
		return true
	}
	if typ == "api_gateway" || typ == "bff" || typ == "gateway" || typ == "web-ui" || typ == "web_ui" ||
		strings.Contains(typ, "gateway") {
		return true
	}
	return false
}

func ingressNameHeuristic(id string) bool {
	// Do not match generic "bff" substring (e.g. "customer-bff" is a service name, not an ingress hint).
	lower := strings.ToLower(id)
	return strings.Contains(lower, "gateway") || strings.Contains(lower, "web-ui") ||
		strings.Contains(lower, "frontend") || strings.Contains(lower, "client")
}

func parseTargetRPS(root map[string]any) float64 {
	rps := defaultRPS
	if cfg, ok := root["configs"].(map[string]any); ok {
		if slo, ok := cfg["slo"].(map[string]any); ok {
			if v, ok := slo["target_rps"]; ok {
				if f, ok := parsePositiveFloat(v); ok {
					rps = f
				}
			}
		}
	}
	return rps
}

func parsePositiveFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, x > 0
	case float32:
		return float64(x), x > 0
	case int:
		return float64(x), x > 0
	case int64:
		return float64(x), x > 0
	case uint64:
		return float64(x), x > 0
	}
	return 0, false
}

func normalizeID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func filterNonDB(order []string, byID map[string]svcInfo) []string {
	var out []string
	for _, id := range order {
		if byID[id].simKind != "database" {
			out = append(out, id)
		}
	}
	return out
}

func pickEntrypoint(byID map[string]svcInfo, incoming map[string]int, order []string) string {
	candidates := filterNonDB(order, byID)
	if len(candidates) == 0 {
		candidates = order
	}

	for _, id := range candidates {
		if strings.ToLower(strings.TrimSpace(byID[id].role)) == "ingress" {
			return id
		}
	}
	for _, id := range candidates {
		if byID[id].simKind == "api_gateway" {
			return id
		}
	}
	for _, id := range candidates {
		t := byID[id].rawType
		if t == "api_gateway" || t == "gateway" || t == "bff" || t == "web-ui" || t == "web_ui" ||
			strings.Contains(t, "gateway") {
			return id
		}
	}
	for _, id := range candidates {
		if strings.EqualFold(byID[id].roleField, "ingress") {
			return id
		}
	}
	for _, id := range candidates {
		if ingressNameHeuristic(id) {
			return id
		}
	}
	for _, id := range candidates {
		if incoming[id] == 0 {
			return id
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	if len(order) > 0 {
		return order[0]
	}
	return ""
}

func workloadPathForEntry(info svcInfo) string {
	switch info.simKind {
	case "api_gateway":
		return "/ingress"
	case "queue":
		return "/enqueue"
	case "topic":
		return "/events"
	default:
		return "/read"
	}
}

func netLatency() LatencyDoc {
	return LatencyDoc{Mean: 2, Sigma: 0.5}
}

func callLat() LatencyDoc {
	return LatencyDoc{Mean: 4, Sigma: 1}
}

func ingressEndpoint(down []DownstreamDoc) EndpointDoc {
	return EndpointDoc{
		Path:            "/ingress",
		MeanCPUMs:       5,
		CPUSigmaMs:      1.5,
		DefaultMemoryMB: 16,
		NetLatencyMs:    netLatency(),
		Downstream:      down,
	}
}

func queueEndpoints() []EndpointDoc {
	return []EndpointDoc{{
		Path:            "/enqueue",
		MeanCPUMs:       10,
		CPUSigmaMs:      2,
		DefaultMemoryMB: 20,
		NetLatencyMs:    netLatency(),
		Downstream:      nil,
	}}
}

func topicEndpoints() []EndpointDoc {
	return []EndpointDoc{{
		Path:            "/events",
		MeanCPUMs:       8,
		CPUSigmaMs:      2,
		DefaultMemoryMB: 20,
		NetLatencyMs:    netLatency(),
		Downstream:      nil,
	}}
}

func crudEndpoints(deps []depEdge, byID map[string]svcInfo) []EndpointDoc {
	paths := []string{"/create", "/read", "/update", "/delete"}
	var eps []EndpointDoc
	for _, p := range paths {
		var ds []DownstreamDoc
		if p == "/read" {
			ds = downstreamCalls(deps, byID)
		}
		eps = append(eps, EndpointDoc{
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

func dbEndpoints(deps []depEdge, byID map[string]svcInfo) []EndpointDoc {
	paths := []string{"/query", "/write"}
	var eps []EndpointDoc
	for _, p := range paths {
		var ds []DownstreamDoc
		if p == "/query" {
			ds = downstreamCalls(deps, byID)
		}
		eps = append(eps, EndpointDoc{
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

func downstreamCalls(deps []depEdge, byID map[string]svcInfo) []DownstreamDoc {
	var out []DownstreamDoc
	for _, d := range deps {
		tgt := byID[d.to]
		out = append(out, makeDownstreamCall(d, tgt))
	}
	return out
}

func makeDownstreamCall(d depEdge, tgt svcInfo) DownstreamDoc {
	mode := "sync"
	if !d.sync {
		mode = "async"
	}
	kind := downstreamKindForCall(d, tgt)
	return DownstreamDoc{
		To:            fmt.Sprintf("%s:%s", d.to, targetEndpointPath(tgt)),
		Mode:          mode,
		Kind:          kind,
		Probability:   1,
		CallCountMean: 1,
		CallLatencyMs: callLat(),
	}
}

func downstreamKindForCall(d depEdge, tgt svcInfo) string {
	in := strings.ToLower(strings.TrimSpace(d.kind))
	switch tgt.simKind {
	case "database":
		return "db"
	case "queue":
		return "queue"
	case "topic":
		return "topic"
	}
	switch in {
	case "", "rest":
		return "rest"
	case "grpc":
		return "grpc"
	case "db":
		return "db"
	case "queue":
		return "queue"
	case "topic":
		return "topic"
	case "external":
		return "external"
	default:
		return "rest"
	}
}

func targetEndpointPath(info svcInfo) string {
	switch info.simKind {
	case "database":
		return "/query"
	case "queue":
		return "/enqueue"
	case "topic":
		return "/events"
	default:
		return "/read"
	}
}

func buildDownstreamsForIngress(from string, deps []depEdge, byID map[string]svcInfo) []DownstreamDoc {
	var out []DownstreamDoc
	for _, d := range deps {
		if d.from != from {
			continue
		}
		tgt := byID[d.to]
		out = append(out, makeDownstreamCall(d, tgt))
	}
	return out
}

// GenerateScenarioYAML returns scenario-v2 draft YAML from AMG/APD input.
// Callers must validate the result with simulation-core HTTP POST /v1/scenarios:validate before persisting or running.
func GenerateScenarioYAML(amgYAML []byte) (string, error) {
	doc, _, err := GenerateFromAMGAPDYAML(amgYAML)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal scenario: %w", err)
	}
	s := string(data)
	if strings.Contains(s, "<nil>") {
		return "", fmt.Errorf("generated scenario YAML must not contain nil artifacts")
	}
	return s, nil
}
