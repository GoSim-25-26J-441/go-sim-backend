package suggestion

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

func join(xs []string, sep string) string {
	if len(xs) == 0 {
		return ""
	}
	var b strings.Builder
	for i := range xs {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(xs[i])
	}
	return b.String()
}

func marshalSpec(spec *parser.YSpec) ([]byte, error) {
	return yaml.Marshal(spec)
}
