package domain

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

func NewTextID(prefix string) (string, error) {
	a, err := randInt(10000, 99999)
	if err != nil {
		return "", err
	}
	b, err := randInt(1000, 9999)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%05d-%04d", prefix, a, b), nil
}

func randInt(min, max int64) (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(max-min+1))
	if err != nil {
		return 0, err
	}
	return min + n.Int64(), nil
}

func StringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
