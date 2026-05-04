package amg_apd_version

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

func nodeKindToCanvasType(k domain.NodeKind) string {
	switch k {
	case domain.NodeService:
		return "service"
	case domain.NodeAPIGateway:
		// Canonical wire type aligned with spec_summary / YAML (see mapper.CanonicalServiceTypeForYAML).
		return "api_gateway"
	case domain.NodeDB:
		return "db"
	case domain.NodeClient:
		return "client"
	case domain.NodeUserActor:
		return "user_actor"
	case domain.NodeEventTopic:
		return "topic"
	case domain.NodeExternalSystem:
		return "external"
	default:
		return "service"
	}
}

func edgeProtocol(e *domain.Edge) string {
	if e == nil || e.Attrs == nil {
		return "REST"
	}
	if v, ok := e.Attrs["dep_kind"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.ToUpper(strings.TrimSpace(v))
	}
	if v, ok := e.Attrs["canvas_protocol"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return "REST"
}

func edgeSync(e *domain.Edge) bool {
	if e == nil || e.Attrs == nil {
		return true
	}
	if v, ok := e.Attrs["sync"].(bool); ok {
		return v
	}
	return true
}

func edgeLabel(e *domain.Edge, g *domain.Graph) string {
	if e != nil && e.Attrs != nil {
		if v, ok := e.Attrs["label"].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	fromName := e.From
	toName := e.To
	if n, ok := g.Nodes[e.From]; ok && n != nil && n.Name != "" {
		fromName = n.Name
	}
	if n, ok := g.Nodes[e.To]; ok && n != nil && n.Name != "" {
		toName = n.Name
	}
	return fmt.Sprintf("%s \u2192 %s", fromName, toName)
}

func isEmptyJSONContainer(b []byte) bool {
	s := strings.TrimSpace(string(b))
	return s == "" || s == "null" || s == "[]"
}

type canvasWireNode struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Type  string   `json:"type"`
	X     *float64 `json:"x,omitempty"`
	Y     *float64 `json:"y,omitempty"`
}

type canvasWireEdge struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	To       string `json:"to"`
	Protocol string `json:"protocol,omitempty"`
	Sync     bool   `json:"sync"`
	Label    string `json:"label,omitempty"`
}

type canvasWireDoc struct {
	Nodes      []canvasWireNode `json:"nodes"`
	Edges      []canvasWireEdge `json:"edges"`
	Detections json.RawMessage  `json:"detections,omitempty"`
}

// buildCanvasDiagramJSON returns a canvas_json-style payload ({nodes, edges} plus optional detections)
// for diagram_versions.diagram_json so UIGP and generateSpecSummaryFromDiagram see top-level nodes/edges.
func buildCanvasDiagramJSON(graphJSON, detectionsJSON []byte) ([]byte, error) {
	var g domain.Graph
	if err := decodeGraphJSONFlexible(graphJSON, &g); err != nil {
		return nil, err
	}
	if g.Nodes == nil {
		g.Nodes = map[string]*domain.Node{}
	}

	nodes := make([]canvasWireNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		if n == nil {
			continue
		}
		nodes = append(nodes, canvasWireNode{
			ID:    n.ID,
			Label: n.Name,
			Type:  nodeKindToCanvasType(n.Kind),
			X:     n.X,
			Y:     n.Y,
		})
	}
	edges := make([]canvasWireEdge, 0, len(g.Edges))
	for i, e := range g.Edges {
		if e == nil {
			continue
		}
		edges = append(edges, canvasWireEdge{
			ID:       fmt.Sprintf("edge-%d", i),
			From:     e.From,
			To:       e.To,
			Protocol: edgeProtocol(e),
			Sync:     edgeSync(e),
			Label:    edgeLabel(e, &g),
		})
	}

	doc := canvasWireDoc{Nodes: nodes, Edges: edges}
	if len(detectionsJSON) > 0 && !isEmptyJSONContainer(detectionsJSON) {
		doc.Detections = detectionsJSON
	}
	return json.Marshal(doc)
}

func edgePairKey(from, to string) string {
	return strings.TrimSpace(from) + "\x00" + strings.TrimSpace(to)
}

func normalizeCanvasToken(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// canvasWireMatchKeys builds index keys so a temp editor id (e.g. node-…-…) can match
// a canonical diagram id (e.g. SERVICE:service-1) with the same type + label or id suffix.
func canvasWireMatchKeys(n canvasWireNode) []string {
	t := normalizeCanvasToken(n.Type)
	l := normalizeCanvasToken(n.Label)
	keys := make([]string, 0, 3)
	if l != "" {
		keys = append(keys, t+"\x00"+l)
	}
	id := strings.TrimSpace(n.ID)
	if idx := strings.LastIndex(id, ":"); idx >= 0 {
		suf := normalizeCanvasToken(id[idx+1:])
		if suf != "" {
			keys = append(keys, t+"\x00"+suf)
		}
	}
	return keys
}

// buildBaseNodeAliasToAnalyzed maps base-only node ids (often editor temp ids) to an
// analyzed node id when type+label (or type+id suffix) match — prevents merged diagrams
// from keeping two copies of the same logical node and parallel duplicate edges.
func buildBaseNodeAliasToAnalyzed(analyzed, base canvasWireDoc) map[string]string {
	out := make(map[string]string)
	idx := make(map[string]string)
	for _, n := range analyzed.Nodes {
		aid := strings.TrimSpace(n.ID)
		if aid == "" {
			continue
		}
		for _, k := range canvasWireMatchKeys(n) {
			if _, ok := idx[k]; !ok {
				idx[k] = aid
			}
		}
	}
	for _, bn := range base.Nodes {
		bid := strings.TrimSpace(bn.ID)
		if bid == "" {
			continue
		}
		for _, k := range canvasWireMatchKeys(bn) {
			if aid, ok := idx[k]; ok && aid != "" && aid != bid {
				out[bid] = aid
				break
			}
		}
	}
	return out
}

func resolveWireEndpoint(id string, alias map[string]string) string {
	id = strings.TrimSpace(id)
	if a, ok := alias[id]; ok && a != "" {
		return a
	}
	return id
}

// allocateUniqueWireEdgeID returns an id of the form edge-N not present in existing.
func allocateUniqueWireEdgeID(existing []canvasWireEdge) string {
	used := make(map[string]struct{})
	maxN := -1
	for _, e := range existing {
		id := strings.TrimSpace(e.ID)
		if id != "" {
			used[id] = struct{}{}
		}
		if !strings.HasPrefix(id, "edge-") {
			continue
		}
		rest := strings.TrimPrefix(id, "edge-")
		if n, err := strconv.Atoi(rest); err == nil && n > maxN {
			maxN = n
		}
	}
	for i := maxN + 1; ; i++ {
		cand := fmt.Sprintf("edge-%d", i)
		if _, taken := used[cand]; !taken {
			return cand
		}
	}
}

func dedupeWireEdgesByEndpointPair(doc *canvasWireDoc) {
	if len(doc.Edges) <= 1 {
		return
	}
	seen := make(map[string]struct{})
	out := make([]canvasWireEdge, 0, len(doc.Edges))
	for _, e := range doc.Edges {
		from := strings.TrimSpace(e.From)
		to := strings.TrimSpace(e.To)
		if from == "" || to == "" {
			continue
		}
		k := edgePairKey(from, to)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, e)
	}
	doc.Edges = out
}

func ensureUniqueWireEdgeIDs(edges []canvasWireEdge) {
	used := make(map[string]struct{})
	for i := range edges {
		id := strings.TrimSpace(edges[i].ID)
		for {
			if id != "" {
				if _, dup := used[id]; !dup {
					break
				}
			}
			for n := 0; ; n++ {
				cand := fmt.Sprintf("edge-%d", n)
				if _, taken := used[cand]; !taken {
					id = cand
					break
				}
			}
		}
		used[id] = struct{}{}
		edges[i].ID = id
	}
}

// mergeCanvasPreserveFromBase overlays the last saved canvas onto freshly analyzed canvas JSON:
// - keeps x/y (and node records) from base when node ids match;
// - restores edge id/label from base when from→to matches;
// - re-appends nodes and edges that exist in base but are missing from the analyzer graph (e.g. DB + READS edges).
func mergeCanvasPreserveFromBase(analyzedJSON, baseJSON []byte) ([]byte, error) {
	if len(strings.TrimSpace(string(baseJSON))) == 0 {
		return analyzedJSON, nil
	}
	var analyzed canvasWireDoc
	if err := json.Unmarshal(analyzedJSON, &analyzed); err != nil {
		return nil, err
	}
	var base canvasWireDoc
	if err := json.Unmarshal(baseJSON, &base); err != nil {
		return analyzedJSON, nil
	}
	if len(base.Nodes) == 0 || len(base.Edges) == 0 {
		return analyzedJSON, nil
	}

	alias := buildBaseNodeAliasToAnalyzed(analyzed, base)

	baseByNodeID := make(map[string]canvasWireNode, len(base.Nodes))
	for _, n := range base.Nodes {
		id := strings.TrimSpace(n.ID)
		if id != "" {
			baseByNodeID[id] = n
		}
	}
	baseEdgePairCap := len(base.Edges)
	if baseEdgePairCap > math.MaxInt/2 {
		return nil, errors.New("edge count too large")
	}
	baseEdgeByPair := make(map[string]canvasWireEdge, baseEdgePairCap*2)
	for _, e := range base.Edges {
		fr := resolveWireEndpoint(e.From, alias)
		tr := resolveWireEndpoint(e.To, alias)
		kRes := edgePairKey(fr, tr)
		kRaw := edgePairKey(e.From, e.To)
		if _, ok := baseEdgeByPair[kRaw]; !ok {
			baseEdgeByPair[kRaw] = e
		}
		if _, ok := baseEdgeByPair[kRes]; !ok {
			baseEdgeByPair[kRes] = e
		}
	}

	for i := range analyzed.Nodes {
		id := strings.TrimSpace(analyzed.Nodes[i].ID)
		if b, ok := baseByNodeID[id]; ok {
			if b.X != nil {
				analyzed.Nodes[i].X = b.X
			}
			if b.Y != nil {
				analyzed.Nodes[i].Y = b.Y
			}
		}
	}

	analyzedNodeCount := len(analyzed.Nodes)
	baseNodeCount := len(base.Nodes)
	if baseNodeCount > math.MaxInt-analyzedNodeCount {
		return nil, errors.New("node count too large")
	}
	nodeIDs := make(map[string]struct{}, analyzedNodeCount+baseNodeCount)
	for _, n := range analyzed.Nodes {
		if id := strings.TrimSpace(n.ID); id != "" {
			nodeIDs[id] = struct{}{}
		}
	}
	for _, bn := range base.Nodes {
		id := strings.TrimSpace(bn.ID)
		if id == "" {
			continue
		}
		if canon := alias[id]; canon != "" {
			if _, ok := nodeIDs[canon]; ok {
				continue
			}
		}
		if _, ok := nodeIDs[id]; !ok {
			analyzed.Nodes = append(analyzed.Nodes, bn)
			nodeIDs[id] = struct{}{}
		}
	}

	analyzedEdgeCount := len(analyzed.Edges)
	baseEdgeCount := len(base.Edges)
	if baseEdgeCount > math.MaxInt-analyzedEdgeCount {
		return nil, errors.New("edge count too large")
	}
	edgeSeen := make(map[string]struct{}, analyzedEdgeCount+baseEdgeCount)
	for _, e := range analyzed.Edges {
		fr := resolveWireEndpoint(e.From, alias)
		tr := resolveWireEndpoint(e.To, alias)
		edgeSeen[edgePairKey(fr, tr)] = struct{}{}
	}

	for i := range analyzed.Edges {
		fr := resolveWireEndpoint(analyzed.Edges[i].From, alias)
		tr := resolveWireEndpoint(analyzed.Edges[i].To, alias)
		k := edgePairKey(fr, tr)
		if be, ok := baseEdgeByPair[k]; ok {
			if strings.TrimSpace(be.ID) != "" {
				analyzed.Edges[i].ID = be.ID
			}
			if strings.TrimSpace(be.Label) != "" {
				analyzed.Edges[i].Label = be.Label
			}
		}
	}

	for _, be := range base.Edges {
		fromR := resolveWireEndpoint(be.From, alias)
		toR := resolveWireEndpoint(be.To, alias)
		k := edgePairKey(fromR, toR)
		if _, ok := edgeSeen[k]; ok {
			continue
		}
		if _, ok := nodeIDs[fromR]; !ok {
			continue
		}
		if _, ok := nodeIDs[toR]; !ok {
			continue
		}
		newBe := be
		newBe.From = fromR
		newBe.To = toR
		newBe.ID = allocateUniqueWireEdgeID(analyzed.Edges)
		analyzed.Edges = append(analyzed.Edges, newBe)
		edgeSeen[k] = struct{}{}
	}

	dedupeWireEdgesByEndpointPair(&analyzed)
	ensureUniqueWireEdgeIDs(analyzed.Edges)

	return json.Marshal(analyzed)
}
