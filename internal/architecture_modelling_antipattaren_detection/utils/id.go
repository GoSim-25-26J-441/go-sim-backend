package utils

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID returns a short random hex string (16 chars).
func NewID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
