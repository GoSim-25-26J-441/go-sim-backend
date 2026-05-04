package amg_apd_version

import (
	"encoding/json"
	"testing"
)

func TestMergeCanvasPreserveFromBase_DBAndEdges(t *testing.T) {
	base := `{"edges":[{"id":"edge-import-0","to":"API_GATEWAY:gateway-1","from":"CLIENT:client-1","sync":true,"protocol":"REST"},{"id":"edge-import-2","to":"SERVICE:service-2","from":"API_GATEWAY:gateway-1","sync":true,"protocol":"REST"},{"id":"edge-db","to":"node-db","from":"SERVICE:service-1","sync":true,"label":"service-1 → db-1","protocol":"REST"}],"nodes":[{"id":"CLIENT:client-1","type":"client","label":"client-1","x":296,"y":481},{"id":"SERVICE:service-1","type":"service","label":"service-1","x":791,"y":162},{"id":"SERVICE:service-2","type":"service","label":"service-2","x":800,"y":476},{"id":"API_GATEWAY:gateway-1","type":"gateway","label":"gateway-1","x":534,"y":314},{"id":"node-db","type":"db","label":"db-1","x":1059,"y":288}]}`
	analyzed := `{"edges":[{"id":"edge-0","to":"API_GATEWAY:gateway-1","from":"CLIENT:client-1","sync":true,"label":"client-1 → gateway-1","protocol":"REST"},{"id":"edge-1","to":"SERVICE:service-1","from":"API_GATEWAY:gateway-1","sync":true,"label":"gateway-1 → service-1","protocol":"REST"},{"id":"edge-2","to":"SERVICE:service-2","from":"API_GATEWAY:gateway-1","sync":true,"label":"gateway-1 → service-2","protocol":"REST"}],"nodes":[{"id":"SERVICE:service-2","type":"service","label":"service-2","x":100,"y":100},{"id":"API_GATEWAY:gateway-1","type":"gateway","label":"gateway-1","x":572,"y":361},{"id":"CLIENT:client-1","type":"client","label":"client-1","x":254,"y":517},{"id":"SERVICE:service-1","type":"service","label":"service-1","x":640,"y":100}]}`
	out, err := mergeCanvasPreserveFromBase([]byte(analyzed), []byte(base))
	if err != nil {
		t.Fatal(err)
	}
	var doc canvasWireDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Nodes) != 5 {
		t.Fatalf("nodes: want 5 got %d", len(doc.Nodes))
	}
	if len(doc.Edges) != 4 {
		t.Fatalf("edges: want 4 got %d", len(doc.Edges))
	}
	seenImport := false
	seenDB := false
	for _, e := range doc.Edges {
		if e.ID == "edge-import-0" {
			seenImport = true
		}
		if e.To == "node-db" && e.From == "SERVICE:service-1" {
			seenDB = true
		}
	}
	if !seenImport {
		t.Fatal("expected edge-import-0 id from base")
	}
	if !seenDB {
		t.Fatal("expected service-1 → db edge from base")
	}
	var dbNode *canvasWireNode
	for i := range doc.Nodes {
		if doc.Nodes[i].ID == "node-db" {
			dbNode = &doc.Nodes[i]
			break
		}
	}
	if dbNode == nil || dbNode.Label != "db-1" {
		t.Fatal("expected db node from base")
	}
}
