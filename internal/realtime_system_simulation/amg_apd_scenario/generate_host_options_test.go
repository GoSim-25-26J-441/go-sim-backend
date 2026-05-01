package amg_apd_scenario

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const minimalAMG = `services:
  - id: gw
    type: api_gateway
dependencies: []
configs:
  slo:
    target_rps: 1
`

func TestGenerateScenarioYAML_emptyOptions_matchesDefaultHosts(t *testing.T) {
	a, err := GenerateScenarioYAML([]byte(minimalAMG))
	require.NoError(t, err)
	b, err := GenerateScenarioYAMLWithOptions([]byte(minimalAMG), GenerationOptions{})
	require.NoError(t, err)
	require.Equal(t, a, b)
	require.Contains(t, a, "host-3")
	require.Contains(t, a, "cores: 8")
}

func TestGenerateScenarioYAMLWithOptions_customHosts(t *testing.T) {
	opts := GenerationOptions{
		Hosts: HostDocsFromCounts(3, 4, 16),
	}
	yamlStr, err := GenerateScenarioYAMLWithOptions([]byte(minimalAMG), opts)
	require.NoError(t, err)
	require.Contains(t, yamlStr, "host-1")
	require.Contains(t, yamlStr, "host-3")
	require.Contains(t, yamlStr, "cores: 4")
	require.Contains(t, yamlStr, "memory_gb: 16")
}

func TestGenerateScenarioYAMLWithOptions_invalidHostListFallsBack(t *testing.T) {
	opts := GenerationOptions{Hosts: HostDocsFromCounts(0, 1, 1)}
	yamlStr, err := GenerateScenarioYAMLWithOptions([]byte(minimalAMG), opts)
	require.NoError(t, err)
	doc, _, err := GenerateFromAMGAPDYAMLWithOptions([]byte(minimalAMG), opts)
	require.NoError(t, err)
	require.Len(t, doc.Hosts, 3)
	require.Equal(t, 8, doc.Hosts[0].Cores)
	var parsed ScenarioDoc
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &parsed))
	require.Len(t, parsed.Hosts, 3)
}

func TestHostDocsFromCounts_deterministicIDs(t *testing.T) {
	h := HostDocsFromCounts(2, 8, 32)
	require.Len(t, h, 2)
	require.Equal(t, "host-1", h[0].ID)
	require.Equal(t, "host-2", h[1].ID)
}
