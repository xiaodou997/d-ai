package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// OAuthAccountIdentity distinguishes provider workspaces as well as users.
// Email alone is deliberately insufficient: one login can own several accounts.
func OAuthAccountIdentity(provider string, email string, metadata map[string]any) string {
	value := func(keys ...string) string {
		for _, key := range keys {
			if v, ok := metadata[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	account := value("account_id", "chatgpt_account_id", "accountId")
	workspace := value("workspace_id", "organization_id", "project_id")
	user := value("account_user_id", "user_id")
	if account == "" && workspace == "" && user == "" {
		return ""
	}
	if user == "" {
		user = strings.ToLower(strings.TrimSpace(email))
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{provider, account, workspace, user}, "\x00")))
	return hex.EncodeToString(sum[:])
}
