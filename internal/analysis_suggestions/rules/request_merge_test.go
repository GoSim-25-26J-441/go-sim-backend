package rules

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergePersistedRequestEnvelope_preservesStoredSimulationNodesAndDesign(t *testing.T) {
	stored := []byte(`{"design":{"preferred_vcpu":4,"preferred_memory_gb":16},"simulation":{"nodes":10},"candidates":[]}`)
	incomingDesign := DesignInput{PreferredVCPU: 99, PreferredMemoryGB: 99}
	incomingSim := SimulationInput{Nodes: 3}

	out := mergePersistedRequestEnvelope(stored, incomingDesign, incomingSim, []Candidate{{ID: "c1"}})

	require.Equal(t, 10, out.Simulation.Nodes)
	require.Equal(t, 3, out.Simulation.CandidateNodes)
	require.Equal(t, 4, out.Design.PreferredVCPU)
	require.Equal(t, 16.0, out.Design.PreferredMemoryGB)
	require.Len(t, out.Candidates, 1)
	require.Equal(t, "c1", out.Candidates[0].ID)
}

func TestMergePersistedRequestEnvelope_whenStoredNodesZero_usesIncoming(t *testing.T) {
	stored := []byte(`{"design":{"preferred_vcpu":4},"simulation":{"nodes":0},"candidates":[]}`)
	incomingDesign := DesignInput{PreferredVCPU: 8}
	incomingSim := SimulationInput{Nodes: 5}

	out := mergePersistedRequestEnvelope(stored, incomingDesign, incomingSim, nil)

	require.Equal(t, 5, out.Simulation.Nodes)
	require.Equal(t, 5, out.Simulation.CandidateNodes)
	require.Equal(t, 8, out.Design.PreferredVCPU)
}

func TestMergePersistedRequestEnvelope_invalidStoredJSON_fallsBackToIncoming(t *testing.T) {
	incomingDesign := DesignInput{PreferredVCPU: 2}
	incomingSim := SimulationInput{Nodes: 7}
	out := mergePersistedRequestEnvelope([]byte(`not-json`), incomingDesign, incomingSim, []Candidate{})
	require.Equal(t, 7, out.Simulation.Nodes)
	require.Equal(t, 7, out.Simulation.CandidateNodes)
	require.Equal(t, 2, out.Design.PreferredVCPU)
}

func TestMergePersistedRequestEnvelope_roundTripJSON_candidateNodesAdditive(t *testing.T) {
	stored := []byte(`{"design":{"preferred_vcpu":1},"simulation":{"nodes":10},"candidates":[]}`)
	out := mergePersistedRequestEnvelope(stored, DesignInput{}, SimulationInput{Nodes: 3}, []Candidate{})
	b, err := json.Marshal(out)
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &raw))
	var sim map[string]int
	require.NoError(t, json.Unmarshal(raw["simulation"], &sim))
	require.Equal(t, 10, sim["nodes"])
	require.Equal(t, 3, sim["candidate_nodes"])
}
