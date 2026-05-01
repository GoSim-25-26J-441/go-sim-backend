package http

import (
	"encoding/json"
	"testing"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
	"github.com/stretchr/testify/require"
)

func TestMarshalDesignRequestEnvelope_storesSimulationNodes(t *testing.T) {
	body := CreateDesignRequestBody{
		Design:     json.RawMessage(`{"preferred_vcpu":4,"preferred_memory_gb":16}`),
		Simulation: &rules.SimulationInput{Nodes: 3},
	}
	b, err := marshalDesignRequestEnvelope(body)
	require.NoError(t, err)
	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &env))
	var sim map[string]int
	require.NoError(t, json.Unmarshal(env["simulation"], &sim))
	require.Equal(t, 3, sim["nodes"])
}

func TestMarshalDesignRequestEnvelope_omittedSimulationBackwardCompatible(t *testing.T) {
	body := CreateDesignRequestBody{
		Design: json.RawMessage(`{"preferred_vcpu":4}`),
	}
	b, err := marshalDesignRequestEnvelope(body)
	require.NoError(t, err)
	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &env))
	var sim map[string]int
	require.NoError(t, json.Unmarshal(env["simulation"], &sim))
	require.Equal(t, 0, sim["nodes"])
}
