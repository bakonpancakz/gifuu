package tools

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// Generate a random string for use as a token
func GenerateRandomToken() string {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic("failed to generate enough random bytes")
	}
	return hex.EncodeToString(b)
}

// Generate a SHA256 from a string
func GenerateHash(str string) string {
	h := sha256.Sum256([]byte(str))
	return hex.EncodeToString(h[:])
}
