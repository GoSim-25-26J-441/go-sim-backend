package rules

import "github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"

func DetectSharedDBWrites(s domain.Spec) []Finding {
	dbOwners := map[string][]string{} // db -> services that write
	for svc, spec := range s.Services {
		for _, db := range spec.Writes {
			dbOwners[db] = append(dbOwners[db], svc)
		}
	}
	var out []Finding
	for db, owners := range dbOwners {
		if len(owners) > 1 {
			out = append(out, Finding{
				Kind:     "shared_db_writes",
				Severity: "high",
				Summary:  "multiple services write to the same database",
				Nodes:    owners,
				Meta:     map[string]any{"db": db, "writers": owners},
			})
		}
	}
	return out
}
