package spec

func mergeMetadata(orig, fixed map[string]any) map[string]any {
	om, _ := orig["metadata"].(map[string]any)
	fm, _ := fixed["metadata"].(map[string]any)

	if om == nil && fm == nil {
		return nil
	}
	out := map[string]any{}
	for k, v := range om {
		out[k] = v
	}
	for k, v := range fm {
		out[k] = v
	}
	return out
}

func Merge(orig, fixed map[string]any) map[string]any {
	out := deepCopyMap(orig)

	if v, ok := fixed["dependencies"]; ok {
		out["dependencies"] = v
	}
	if v, ok := fixed["services"]; ok {
		out["services"] = v
	}

	if md := mergeMetadata(orig, fixed); md != nil {
		out["metadata"] = md
	}

	return out
}
