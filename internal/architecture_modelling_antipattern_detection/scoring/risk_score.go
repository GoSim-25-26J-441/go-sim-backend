package scoring

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"


func ScoreDetection(d domain.Detection) int {
	base := severityWeight(d.Severity)
	kind := kindWeight(d.Kind)
	size := 0
	if len(d.Nodes) > 0 {
		size = min(5, len(d.Nodes)-1)
	}
	return base + kind + size
}

func severityWeight(s domain.Severity) int {
	switch s {
	case domain.SeverityHigh:
		return 60
	case domain.SeverityMedium:
		return 35
	case domain.SeverityLow:
		return 15
	default:
		return 20
	}
}

func kindWeight(k domain.AntiPatternKind) int {
	switch k {
	case domain.APCycles:
		return 25
	case domain.APGodService:
		return 20
	case domain.APTightCoupling:
		return 22
	case domain.APSharedDatabase:
		return 18
	case domain.APSyncCallChain:
		return 16
	case domain.APPingPongDependency:
		return 14
	case domain.APReverseDependency:
		return 20
	case domain.APUIOrchestrator:
		return 17
	default:
		return 10
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
