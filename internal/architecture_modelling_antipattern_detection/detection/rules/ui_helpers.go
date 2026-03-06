package rules

import (
	"strings"
)

// isBFFOrGatewayName returns true for node names/ids that represent a BFF or API gateway.
// Such nodes must not be treated as "UI" for the UI orchestrator rule.
func isBFFOrGatewayName(id string) bool {
	s := strings.ToLower(strings.TrimSpace(id))
	// Strip common prefixes for the check
	s = strings.TrimPrefix(s, "service:")
	if strings.Contains(s, "bff") ||
		strings.Contains(s, "backend-for-frontend") ||
		strings.Contains(s, "api-gateway") ||
		strings.Contains(s, "api_gateway") ||
		strings.HasSuffix(s, "gateway") ||
		s == "gateway" {
		return true
	}
	return false
}

func isUIName(id string) bool {
	s := strings.ToLower(strings.TrimSpace(id))
	// Do not treat BFF/gateway nodes as UI (they are the fix, not the orchestrator)
	if isBFFOrGatewayName(id) {
		return false
	}
	return strings.Contains(s, "web") ||
		strings.Contains(s, "ui") ||
		strings.Contains(s, "frontend") ||
		strings.Contains(s, "page") ||
		strings.Contains(s, "client") ||
		strings.Contains(s, "browser")
}
