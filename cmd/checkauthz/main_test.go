package main

import (
	"os"
	"strings"
	"testing"
)

func TestContractOperationsHaveAuthorizationRules(t *testing.T) {
	content, err := os.ReadFile("../../contracts/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ops, err := parseOperations(string(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) < 300 {
		t.Fatalf("parsed %d operations, expected the complete contract", len(ops))
	}
	for _, op := range ops {
		if _, ok := classify(op); !ok {
			t.Fatalf("operation is not classified: %s %s (%s)", op.Method, op.Path, op.ID)
		}
	}
}

func TestAuthorizationRulesCoverCriticalFamilies(t *testing.T) {
	tests := []struct {
		path   string
		policy string
	}{
		{"/api/auth/login", "public"},
		{"/api/v1/jwt-keys/rotate", "super_admin"},
		{"/api/v1/tenants/me/groups", "tenant_self"},
		{"/api/v1/users/me/api-keys", "customer_self"},
		{"/api/v1/payments/topup-orders", "tenant_or_customer"},
		{"/api/v1/admin/modules", "platform_admin"},
		{"/v1/tasks/{taskID}", "api_key_or_session"},
	}
	for _, tt := range tests {
		got, ok := classify(operation{Method: "GET", Path: tt.path, ID: "test"})
		if !ok || got.Policy != tt.policy {
			t.Errorf("classify(%q) = %#v/%v, want policy %q", tt.path, got, ok, tt.policy)
		}
	}
}

func TestRenderedAuthorizationMatrixHasNoTrailingWhitespace(t *testing.T) {
	rendered := renderMatrix([]byte("contract"), []struct {
		operation
		rule
	}{{operation: operation{Method: "GET", Path: "/health", ID: "health"}, rule: rule{Policy: "public", Ownership: "none"}}})
	for _, line := range strings.Split(rendered, "\n") {
		if strings.TrimRight(line, " \t") != line {
			t.Fatalf("rendered line has trailing whitespace: %q", line)
		}
	}
}
