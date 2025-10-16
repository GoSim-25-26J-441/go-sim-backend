package detection

import (
	"os"
	"strconv"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/detection/rules"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"
)

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func RunAll(g domain.Graph, s domain.Spec) []Finding {
	findings := []Finding{}

	// 1) Cycles
	for _, f := range rules.DetectCycles(g) {
		findings = append(findings, Finding{Kind: f.Kind, Severity: f.Severity, Summary: f.Summary, Nodes: f.Nodes})
	}

	// 2) God service (rule)
	godThr := envInt("AMG_GOD_SERVICE_FANOUT", 6)
	for _, f := range rules.DetectGodService(g, godThr) {
		findings = append(findings, Finding{Kind: f.Kind, Severity: f.Severity, Summary: f.Summary, Nodes: f.Nodes, Meta: f.Meta})
	}

	// 3) Shared DB writes
	for _, f := range rules.DetectSharedDBWrites(s) {
		findings = append(findings, Finding{Kind: f.Kind, Severity: f.Severity, Summary: f.Summary, Nodes: f.Nodes, Meta: f.Meta})
	}

	// 4) Cross-DB reads
	for _, f := range rules.DetectCrossDBReads(s) {
		findings = append(findings, Finding{Kind: f.Kind, Severity: f.Severity, Summary: f.Summary, Nodes: f.Nodes, Meta: f.Meta})
	}

	// 5) Tight coupling
	for _, f := range rules.DetectTightCoupling(g) {
		findings = append(findings, Finding{Kind: f.Kind, Severity: f.Severity, Summary: f.Summary, Nodes: f.Nodes, Meta: f.Meta})
	}

	// 6) Chatty calls (rule)
	chatThr := envInt("AMG_CHATTY_CALLS_PER_MIN", 300)
	for _, f := range rules.DetectChattyCalls(s, chatThr) {
		findings = append(findings, Finding{Kind: f.Kind, Severity: f.Severity, Summary: f.Summary, Nodes: f.Nodes, Meta: f.Meta})
	}

	return findings
}
