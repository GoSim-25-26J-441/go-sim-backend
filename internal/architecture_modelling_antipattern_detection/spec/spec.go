package spec

import "gopkg.in/yaml.v3"

func UnmarshalMap(b []byte) (map[string]any, error) {
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func MarshalMap(m map[string]any) ([]byte, error) {
	return yaml.Marshal(m)
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = deepCopy(vv)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, vv := range t {
			out = append(out, deepCopy(vv))
		}
		return out
	default:
		return v
	}
}

func deepCopyMap(m map[string]any) map[string]any {
	return deepCopy(m).(map[string]any)
}
