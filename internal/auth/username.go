package auth

import "strings"

// NormalizeUsername returns the canonical stored form for a Portal username.
func NormalizeUsername(username string) string {
	return strings.TrimSpace(username)
}
