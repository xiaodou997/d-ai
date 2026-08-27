package main

import (
	"context"
	"testing"
)

func TestAssembleAIRuntimeRoleRequiresPlatform(t *testing.T) {
	if _, err := assembleAIRuntimeRole(context.Background(), nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("expected missing platform runtime role to fail")
	}
}
