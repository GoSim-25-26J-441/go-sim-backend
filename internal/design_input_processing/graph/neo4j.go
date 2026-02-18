package graph

import "context"

type Neo4jClient interface {
	Run(ctx context.Context, cypher string, params map[string]any) error
}

func PushToNeo4j(ctx context.Context, db Neo4jClient, g *Graph) error {
	// TODO: use a real driver.
	return nil
}
