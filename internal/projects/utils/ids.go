package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
)

// NewID generates a new hex-based ID with a prefix (used for chats, messages, attachments).
// Format: "prefix_hexstring" (e.g., "thr_a1b2c3d4e5f6...")
func NewID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b)), nil
}

// NewTextID generates a new human-readable numeric ID with a prefix (used for projects, diagrams).
// Format: "prefix-12345-6789" (e.g., "archfind-12345-6789")
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
