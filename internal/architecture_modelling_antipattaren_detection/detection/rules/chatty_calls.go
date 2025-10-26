package rules

import (
	"os"
	"strconv"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"
)

type chatty struct{}
func (c chatty) Name() string { return "chatty_calls" }

func (c chatty) Detect(g *domain.Graph) ([]domain.Detection, error) {
	thr := 300
	if v := os.Getenv("DETECT_CHATTY_RATE_PER_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil { thr = n }
	}
	var out []domain.Detection
	for i, e := range g.Edges {
		if e.Kind != domain.EdgeCalls { continue }
		perItem, _ := e.Attrs["per_item"].(bool)
		rpm, _ := e.Attrs["rate_per_min"].(int)
		if perItem || rpm >= thr {
			out = append(out, domain.Detection{
				Kind: domain.APChattyCalls, Severity: domain.SeverityMedium,
				Title: "Chatty call",
				Summary: "High-frequency or per-item chattiness between services",
				Nodes: []string{e.From, e.To}, Edges: []int{i},
				Evidence: domain.Attrs{"rate_per_min": rpm, "per_item": perItem},
			})
		}
	}
	return out, nil
}

func init(){ detection.Register(chatty{}) }
