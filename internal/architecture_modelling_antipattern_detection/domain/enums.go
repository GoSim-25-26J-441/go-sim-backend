package domain

type NodeKind string

const (
	NodeService NodeKind = "SERVICE"
	NodeDB      NodeKind = "DATABASE"
)

type EdgeKind string

const (
	EdgeCalls  EdgeKind = "CALLS"
	EdgeReads  EdgeKind = "READS"
	EdgeWrites EdgeKind = "WRITES"
)

type AntiPatternKind string

const (
	APCycles             AntiPatternKind = "cycles"
	APGodService         AntiPatternKind = "god_service"
	APTightCoupling      AntiPatternKind = "tight_coupling"
	APSharedDatabase     AntiPatternKind = "shared_database"
	APSyncCallChain      AntiPatternKind = "sync_call_chain"
	APPingPongDependency AntiPatternKind = "ping_pong_dependency"
	APReverseDependency  AntiPatternKind = "reverse_dependency"
	APUIOrchestrator     AntiPatternKind = "ui_orchestrator"
)

type Severity string

const (
	SeverityLow    Severity = "LOW"
	SeverityMedium Severity = "MEDIUM"
	SeverityHigh   Severity = "HIGH"
)
