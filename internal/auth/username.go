package auth

import "strings"

// NormalizeUsername returns the canonical stored form for a Portal username.
func NormalizeUsername(username string) string {
	return strings.TrimSpace(username)
}

// NormalizeEndUsername keeps end users in an explicit global namespace.
func NormalizeEndUsername(username string) string {
	username = NormalizeUsername(username)
	if strings.HasPrefix(username, "u_") {
		return username
	}
	return "u_" + username
}
