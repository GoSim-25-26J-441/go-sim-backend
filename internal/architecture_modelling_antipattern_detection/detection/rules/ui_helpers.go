package rules

import "strings"

func isUIName(id string) bool {
	s := strings.ToLower(strings.TrimSpace(id))
	return strings.Contains(s, "web") ||
		strings.Contains(s, "ui") ||
		strings.Contains(s, "frontend") ||
		strings.Contains(s, "page") ||
		strings.Contains(s, "client") ||
		strings.Contains(s, "browser")
}
