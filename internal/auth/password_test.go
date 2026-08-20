package auth

import (
	"errors"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		username string
		wantErr  bool
	}{
		{name: "valid three classes", password: "Correct-Horse-47", username: "alice"},
		{name: "valid unicode", password: "复杂Passphrase-47", username: "alice"},
		{name: "too short", password: "Short-47", username: "alice", wantErr: true},
		{name: "too few classes", password: "alllowercasepassword", username: "alice", wantErr: true},
		{name: "contains username", password: "Alice-Secure-47", username: "alice", wantErr: true},
		{name: "bcrypt byte limit", password: "Correct-Horse-47-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", username: "alice", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password, tt.username)
			if tt.wantErr != errors.Is(err, ErrWeakPassword) {
				t.Fatalf("ValidatePassword() error = %v, want weak=%t", err, tt.wantErr)
			}
		})
	}
}
