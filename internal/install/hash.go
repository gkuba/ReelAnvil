package install

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// fileHash returns "" when the file cannot be read.
func fileHash(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return hashBytes(b)
}
