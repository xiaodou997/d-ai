package claude

import (
	"crypto/rand"
	"encoding/hex"
)

// randomHex returns n hex characters (n/2 random bytes).
func randomHex(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
