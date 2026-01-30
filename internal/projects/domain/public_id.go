package domain

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// NewPublicID generates a human-readable public ID for a project, e.g. "archfind-12345-6789".
func NewPublicID(prefix string) (string, error) {
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

