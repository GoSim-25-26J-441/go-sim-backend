package rules

import (
	"slices"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"
)

func DetectCrossDBReads(s domain.Spec) []Finding {
	// who "owns" a DB? Treat writers as owners
	dbOwners := map[string][]string{}
	for svc, spec := range s.Services {
		for _, db := range spec.Writes {
			dbOwners[db] = append(dbOwners[db], svc)
		}
	}
	var out []Finding
	for svc, spec := range s.Services {
		for _, db := range spec.Reads {
			owners := dbOwners[db]
			// if there are owners and this svc is not the sole owner → cross-owned read
			if len(owners) > 0 && !slices.Contains(owners, svc) {
				out = append(out, Finding{
					Kind:     "cross_db_read",
					Severity: "medium",
					Summary:  "service reads a database owned by another service",
					Nodes:    append([]string{svc}, owners...),
					Meta:     map[string]any{"db": db, "owners": owners, "reader": svc},
				})
			}
		}
	}
	return out
}
