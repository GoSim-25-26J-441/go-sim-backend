package detection

import "sort"

var registered = map[string]Detector{}

func Register(d Detector) {
	if d == nil {
		return
	}
	registered[d.Name()] = d
}

func All() []Detector {
	out := make([]Detector, 0, len(registered))
	for _, d := range registered {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
