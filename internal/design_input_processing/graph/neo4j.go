package graph

import "context"

// pseudo API, integrate with your existing Neo4j driver
type Neo4jClient interface {
	Run(ctx context.Context, cypher string, params map[string]any) error
}

func PushToNeo4j(ctx context.Context, db Neo4jClient, g *Graph) error {
	// TODO: use your real driver.
	// 1) MERGE (:Service {id: ..., name: ..., kind: ...})
	// 2) MERGE (:Service {id: from})-[:CALLS {protocol: ..., rps: ...}]->(:Service {id: to})
	return nil
}
