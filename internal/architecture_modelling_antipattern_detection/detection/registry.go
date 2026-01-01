package detection

var registered []Detector

func Register(d Detector) { registered = append(registered, d) }
func All() []Detector     { return registered }
