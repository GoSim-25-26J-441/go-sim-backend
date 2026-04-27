package spec

import "strings"

func cleanRef(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	up := strings.ToUpper(t)

	if strings.HasPrefix(up, "SERVICE:") {
		return strings.TrimSpace(t[len("SERVICE:"):])
	}
	if strings.HasPrefix(up, "DATABASE:") {
		return strings.TrimSpace(t[len("DATABASE:"):])
	}

	noSpaceUp := strings.ReplaceAll(up, " ", "")
	noSpaceRaw := strings.ReplaceAll(t, " ", "")
	if strings.HasPrefix(noSpaceUp, "SERVICE:") {
		return strings.TrimSpace(noSpaceRaw[len("SERVICE:"):])
	}
	if strings.HasPrefix(noSpaceUp, "DATABASE:") {
		return strings.TrimSpace(noSpaceRaw[len("DATABASE:"):])
	}

	return t
}

// canonicalServiceYAMLType normalizes services[].type for merge/sanitize (must stay in sync
// with ingest/mapper normalizeType).
func canonicalServiceYAMLType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "", "svc", "microservice", "micro-service", "ms", "service":
		return "service"
	case "db", "datastore", "data_store", "data-store", "database":
		return "database"
	case "api_gateway", "api-gateway", "gateway", "bff":
		return "api_gateway"
	case "client":
		return "client"
	case "user_actor", "user-actor", "user", "actor":
		return "user_actor"
	case "event_topic", "event-topic", "topic":
		return "event_topic"
	case "external_system", "external-system", "external":
		return "external_system"
	default:
		return "service"
	}
}

func Sanitize(root map[string]any) {
	if root == nil {
		return
	}

	if deps, ok := root["dependencies"].([]any); ok {
		for _, it := range deps {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if from, ok := m["from"].(string); ok {
				m["from"] = cleanRef(from)
			}
			if to, ok := m["to"].(string); ok {
				m["to"] = cleanRef(to)
			}
			if kind, ok := m["kind"].(string); ok {
				m["kind"] = strings.ToLower(strings.TrimSpace(kind))
			}
		}
	}

	if svcs, ok := root["services"].([]any); ok {
		for _, it := range svcs {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if name, ok := m["name"].(string); ok {
				m["name"] = cleanRef(name)
			}
			if typ, ok := m["type"].(string); ok {
				t := strings.TrimSpace(typ)
				if t == "" {
					m["type"] = "service"
				} else {
					m["type"] = canonicalServiceYAMLType(t)
				}
			} else {
				m["type"] = "service"
			}
		}
	}
}
