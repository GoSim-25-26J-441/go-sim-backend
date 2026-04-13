package domain

type NodeKind string

const (
	NodeService        NodeKind = "SERVICE"
	NodeAPIGateway     NodeKind = "API_GATEWAY" // BFF / gateway – not counted as UI orchestrator source
	NodeDB             NodeKind = "DATABASE"
	NodeClient         NodeKind = "CLIENT"
	NodeUserActor      NodeKind = "USER_ACTOR"
	NodeEventTopic     NodeKind = "EVENT_TOPIC"
	NodeExternalSystem NodeKind = "EXTERNAL_SYSTEM"
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
