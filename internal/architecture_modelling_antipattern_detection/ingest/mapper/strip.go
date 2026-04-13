package mapper

import "strings"

var graphIDPrefixes = []string{
	"SERVICE:",
	"DATABASE:",
	"API_GATEWAY:",
	"CLIENT:",
	"USER_ACTOR:",
	"EVENT_TOPIC:",
	"EXTERNAL_SYSTEM:",
}

// StripNodeNameRef removes graph node ID prefixes (e.g. SERVICE:, DATABASE:) case-insensitively.
// Tolerates spaces like "SERVICE : foo" via a second pass without spaces.
func StripNodeNameRef(s string) string {
	t := strings.TrimSpace(s)
	for t != "" {
		next, ok := stripOneGraphPrefix(t)
		if !ok {
			break
		}
		t = next
	}
	return t
}

func stripOneGraphPrefix(t string) (string, bool) {
	t = strings.TrimSpace(t)
	if t == "" {
		return t, false
	}
	up := strings.ToUpper(t)
	for _, p := range graphIDPrefixes {
		if strings.HasPrefix(up, p) && len(t) >= len(p) {
			return strings.TrimSpace(t[len(p):]), true
		}
	}
	noSpaceUp := strings.ReplaceAll(up, " ", "")
	noSpaceT := strings.ReplaceAll(t, " ", "")
	for _, p := range graphIDPrefixes {
		if strings.HasPrefix(noSpaceUp, p) && len(noSpaceT) >= len(p) {
			return strings.TrimSpace(noSpaceT[len(p):]), true
		}
	}
	return t, false
}
