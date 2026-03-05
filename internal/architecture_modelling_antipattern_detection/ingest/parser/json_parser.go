package parser

import (
	"encoding/json"
	"os"
)

func ParseJSON(path string) (*YSpec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseJSONBytes(b)
}

func ParseJSONBytes(b []byte) (*YSpec, error) {
	var s YSpec
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func ParseJSONString(s string) (*YSpec, error) {
	return ParseJSONBytes([]byte(s))
}
