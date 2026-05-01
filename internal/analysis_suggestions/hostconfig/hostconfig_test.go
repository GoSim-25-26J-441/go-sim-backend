package hostconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseScenarioHostConfig_Valid(t *testing.T) {
	raw := []byte(`{
  "design": {"preferred_vcpu": 4, "preferred_memory_gb": 16},
  "simulation": {"nodes": 3}
}`)
	cfg, ok := ParseScenarioHostConfig(raw)
	require.True(t, ok)
	require.Equal(t, 3, cfg.Nodes)
	require.Equal(t, 4, cfg.Cores)
	require.InDelta(t, 16.0, cfg.MemoryGB, 1e-9)
	require.Contains(t, CanonicalJSON(cfg), `"nodes":3`)
}

func TestParseScenarioHostConfig_InvalidOrMissing(t *testing.T) {
	for _, raw := range [][]byte{
		nil,
		[]byte(`{}`),
		[]byte(`{"design":{"preferred_vcpu":0,"preferred_memory_gb":16},"simulation":{"nodes":3}}`),
		[]byte(`{"design":{"preferred_vcpu":4,"preferred_memory_gb":0},"simulation":{"nodes":3}}`),
		[]byte(`{"design":{"preferred_vcpu":4,"preferred_memory_gb":16},"simulation":{"nodes":0}}`),
	} {
		_, ok := ParseScenarioHostConfig(raw)
		require.False(t, ok)
	}
}
