package amg_apd_version

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

func nodeKindToCanvasType(k domain.NodeKind) string {
	switch k {
	case domain.NodeService:
		return "service"
	case domain.NodeAPIGateway:
		return "gateway"
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

	baseByNodeID := make(map[string]canvasWireNode, len(base.Nodes))
	for _, n := range base.Nodes {
		id := strings.TrimSpace(n.ID)
		if id != "" {
			baseByNodeID[id] = n
		}
	}
	baseEdgeByPair := make(map[string]canvasWireEdge, len(base.Edges))
	for _, e := range base.Edges {
		k := edgePairKey(e.From, e.To)
		if _, ok := baseEdgeByPair[k]; !ok {
			baseEdgeByPair[k] = e
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

	nodeIDs := make(map[string]struct{}, len(analyzed.Nodes))
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
		if _, ok := nodeIDs[id]; !ok {
			analyzed.Nodes = append(analyzed.Nodes, bn)
			nodeIDs[id] = struct{}{}
		}
	}

	edgeSeen := make(map[string]struct{}, len(analyzed.Edges))
	for _, e := range analyzed.Edges {
		edgeSeen[edgePairKey(e.From, e.To)] = struct{}{}
	}

	for i := range analyzed.Edges {
		k := edgePairKey(analyzed.Edges[i].From, analyzed.Edges[i].To)
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
		k := edgePairKey(be.From, be.To)
		if _, ok := edgeSeen[k]; ok {
			continue
		}
		fromID := strings.TrimSpace(be.From)
		toID := strings.TrimSpace(be.To)
		if _, ok := nodeIDs[fromID]; !ok {
			continue
		}
		if _, ok := nodeIDs[toID]; !ok {
			continue
		}
		analyzed.Edges = append(analyzed.Edges, be)
		edgeSeen[k] = struct{}{}
	}

	return json.Marshal(analyzed)
}
