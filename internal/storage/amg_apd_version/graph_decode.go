package amg_apd_version

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

func canvasTypeToNodeKind(t string) domain.NodeKind {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "db", "database":
		return domain.NodeDB
	case "topic":
		return domain.NodeEventTopic
	case "gateway":
		return domain.NodeAPIGateway
	case "external":
		return domain.NodeExternalSystem
	case "client":
		return domain.NodeClient
	case "user", "user_actor":
		return domain.NodeUserActor
	default:
		return domain.NodeService
	}
}

// decodeGraphJSONFlexible unmarshals AMG graph JSON (nodes object) or canvas diagram_json
// (nodes array with id/label/type) into domain.Graph.
func decodeGraphJSONFlexible(graphJSON []byte, g *domain.Graph) error {
	if len(graphJSON) == 0 {
		*g = *domain.NewGraph()
		return nil
	}

	var std domain.Graph
	stdErr := json.Unmarshal(graphJSON, &std)
	if stdErr == nil {
		if std.Nodes == nil {
			std.Nodes = map[string]*domain.Node{}
		}
		if std.Edges == nil {
			std.Edges = []*domain.Edge{}
		}
		*g = std
		return nil
	}

	var canvas struct {
		Nodes []struct {
			ID    string   `json:"id"`
			Label string   `json:"label"`
			Type  string   `json:"type"`
			X     *float64 `json:"x,omitempty"`
			Y     *float64 `json:"y,omitempty"`
		} `json:"nodes"`
		Edges []struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Protocol string `json:"protocol"`
			Sync     *bool  `json:"sync,omitempty"`
			Label    string `json:"label,omitempty"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(graphJSON, &canvas); err != nil {
		return fmt.Errorf("%w (canvas fallback: %v)", stdErr, err)
	}
	if len(canvas.Nodes) == 0 && len(canvas.Edges) == 0 {
		return stdErr
	}

	ng := domain.NewGraph()
	for _, n := range canvas.Nodes {
		id := strings.TrimSpace(n.ID)
		if id == "" {
			continue
		}
		ng.AddNode(&domain.Node{
			ID:    id,
			Name:  n.Label,
			Kind:  canvasTypeToNodeKind(n.Type),
			X:     n.X,
			Y:     n.Y,
			Attrs: nil,
		})
	}
	for _, e := range canvas.Edges {
		sync := true
		if e.Sync != nil {
			sync = *e.Sync
		}
		attrs := domain.Attrs{}
		if strings.TrimSpace(e.Protocol) != "" {
			attrs["canvas_protocol"] = e.Protocol
		}
		attrs["sync"] = sync
		if strings.TrimSpace(e.Label) != "" {
			attrs["label"] = e.Label
		}
		ng.AddEdge(&domain.Edge{
			From:  strings.TrimSpace(e.From),
			To:    strings.TrimSpace(e.To),
			Kind:  domain.EdgeCalls,
			Attrs: attrs,
		})
	}
	*g = *ng
	return nil
}
