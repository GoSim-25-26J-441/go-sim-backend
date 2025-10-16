package parser

import (
	"errors"

	"gopkg.in/yaml.v3"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"
)

func FromYAML(b []byte) (domain.Spec, error) {
	var s domain.Spec
	if err := yaml.Unmarshal(b, &s); err != nil {
		return domain.Spec{}, err
	}
	if len(s.Services) == 0 {
		return domain.Spec{}, errors.New("no services found in YAML")
	}
	return s, nil
}
