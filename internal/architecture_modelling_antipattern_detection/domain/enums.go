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
	APCycles         AntiPatternKind = "cycles"
	APGodService     AntiPatternKind = "god_service"
	APTightCoupling  AntiPatternKind = "tight_coupling"
	APSharedDBWrites AntiPatternKind = "shared_db_writes"
	APCrossDBRead    AntiPatternKind = "cross_db_read"
	APChattyCalls    AntiPatternKind = "chatty_calls"
)

type Severity string

const (
	SeverityLow    Severity = "LOW"
	SeverityMedium Severity = "MEDIUM"
	SeverityHigh   Severity = "HIGH"
)
