// Command checkauthz builds and validates the backend authorization matrix
// from the generated OpenAPI contract. A new operation must be classified by
// an explicit route-family rule before the matrix can be regenerated.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	contractPath = "contracts/openapi.yaml"
	matrixPath   = "docs/AUTHORIZATION_MATRIX.md"
)

type operation struct {
	Method string
	Path   string
	ID     string
}

type rule struct {
	Policy    string
	Ownership string
	Reason    string
}

func main() {
	data, err := os.ReadFile(contractPath)
	if err != nil {
		fail("read contract", err)
	}
	ops, err := parseOperations(string(data))
	if err != nil {
		fail("parse contract", err)
	}
	if len(ops) == 0 {
		fail("parse contract", fmt.Errorf("no operations found"))
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Path != ops[j].Path {
			return ops[i].Path < ops[j].Path
		}
		return ops[i].Method < ops[j].Method
	})
	seen := make(map[string]struct{}, len(ops))
	rows := make([]struct {
		operation
		rule
	}, 0, len(ops))
	for _, op := range ops {
		if _, ok := seen[op.ID]; ok {
			fail("validate contract", fmt.Errorf("duplicate operationId %q", op.ID))
		}
		seen[op.ID] = struct{}{}
		classification, ok := classify(op)
		if !ok {
			fail("classify operation", fmt.Errorf("no authorization rule for %s %s (%s)", op.Method, op.Path, op.ID))
		}
		rows = append(rows, struct {
			operation
			rule
		}{operation: op, rule: classification})
	}
	if err := os.WriteFile(matrixPath, []byte(renderMatrix(data, rows)), 0o644); err != nil {
		fail("write matrix", err)
	}
	fmt.Printf("authorization matrix: %d/%d operations classified; wrote %s\n", len(rows), len(ops), matrixPath)
}

func parseOperations(content string) ([]operation, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	inPaths := false
	path := ""
	method := ""
	var operations []operation
	for scanner.Scan() {
		line := scanner.Text()
		if line == "paths:" {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}
		if len(line) > 3 && strings.HasPrefix(line, "  /") && strings.HasSuffix(line, ":") {
			path = strings.TrimSuffix(strings.TrimSpace(line), ":")
			method = ""
			continue
		}
		if path != "" && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(line, ":") {
			candidate := strings.TrimSuffix(strings.TrimSpace(line), ":")
			switch candidate {
			case "get", "post", "put", "patch", "delete", "head", "options":
				method = candidate
			}
			continue
		}
		if path != "" && method != "" && strings.HasPrefix(line, "      operationId:") {
			id := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "operationId:"))
			if id == "" {
				return nil, fmt.Errorf("empty operationId for %s %s", method, path)
			}
			operations = append(operations, operation{Method: strings.ToUpper(method), Path: path, ID: id})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return operations, nil
}

func classify(op operation) (rule, bool) {
	p, id := op.Path, op.ID
	switch {
	case p == "/.well-known/jwks.json" || p == "/public/jwks.json" || p == "/api/v1/info":
		return rule{"public", "none", "public metadata or signing keys"}, true
	case p == "/api/auth/login" || p == "/api/auth/refresh" || p == "/api/auth/activate" || p == "/api/auth/password-policy":
		return rule{"public", "none", "anonymous authentication bootstrap"}, true
	case strings.HasPrefix(p, "/api/v1/public/"):
		return rule{"public", "none", "public invitation flow"}, true
	case strings.HasPrefix(p, "/v1/tasks"):
		return rule{"api_key_or_session", "resource", "AI gateway task credential"}, true
	case p == "/api/auth/mfa/enroll" || p == "/api/auth/mfa/confirm":
		return rule{"platform_admin", "actor.user", "administrator MFA management"}, true
	case strings.HasPrefix(p, "/api/auth/"):
		return rule{"authenticated", "actor.user", "authenticated account/session operation"}, true
	case strings.HasPrefix(p, "/api/v1/users/me/") || strings.HasPrefix(p, "/api/v1/user-") || p == "/api/v1/customer/portal-brand":
		return rule{"customer_self", "actor.user", "terminal-user self-service"}, true
	case strings.HasPrefix(p, "/api/v1/tenant/") || strings.HasPrefix(p, "/api/v1/tenants/me/") || p == "/api/v1/tenants/me" || p == "/api/v1/invitations" || strings.HasPrefix(p, "/api/v1/invitations/") || p == "/api/v1/tenant-api-keys":
		return rule{"tenant_self", "actor.tenant", "tenant self-service"}, true
	case strings.HasPrefix(p, "/api/v1/tenants/analytics/"):
		return rule{"tenant_self", "actor.tenant", "tenant analytics"}, true
	case strings.HasPrefix(p, "/api/v1/payments/"):
		return rule{"tenant_or_customer", "actor.tenant + actor.user", "scoped online top-up"}, true
	case p == "/api/v1/account/balance" || p == "/api/v1/account/recharge-records" || p == "/api/v1/account/stats" || strings.HasPrefix(p, "/api/v1/announcements") || strings.HasPrefix(p, "/api/v1/notifications"):
		return rule{"authenticated", "actor.tenant/user", "authenticated self or inbox projection"}, true
	case p == "/api/v1/recharges" || strings.HasPrefix(p, "/api/v1/recharges/") || p == "/api/v1/users" || strings.HasPrefix(p, "/api/v1/users/"):
		return rule{"platform_or_tenant", "actor.tenant/resource", "management action with tenant ownership"}, true
	case p == "/api/v1/jwt-keys" || strings.HasPrefix(p, "/api/v1/jwt-keys/"):
		return rule{"super_admin", "global", "signing-key administration"}, true
	case strings.HasPrefix(p, "/api/v1/admin/") || strings.HasPrefix(p, "/api/v1/ai/") || strings.HasPrefix(p, "/api/v1/analytics/") || strings.HasPrefix(p, "/api/v1/dashboard/") || strings.HasPrefix(p, "/api/v1/audit-logs") || strings.HasPrefix(p, "/api/v1/auth-audit-logs") || strings.HasPrefix(p, "/api/v1/system/") || strings.HasPrefix(p, "/api/v1/credential-pools") || strings.HasPrefix(p, "/api/v1/upstream-accounts") || strings.HasPrefix(p, "/api/v1/price-books") || strings.HasPrefix(p, "/api/v1/limit-policies") || strings.HasPrefix(p, "/api/v1/risk-control") || strings.HasPrefix(p, "/api/v1/oauth-pool-health") || strings.HasPrefix(p, "/api/v1/model-capability") || strings.HasPrefix(p, "/api/v1/route-weights") || strings.HasPrefix(p, "/api/v1/usage-") || p == "/api/v1/usage-logs" || strings.HasPrefix(p, "/api/v1/usage-logs/") || p == "/api/v1/usage-ranking/users" || p == "/api/v1/tenant-users" || strings.HasPrefix(p, "/api/v1/tenant-users/") || p == "/api/v1/system-admins" || strings.HasPrefix(p, "/api/v1/system-admins/") || strings.HasPrefix(p, "/api/v1/tenants/{tenantID}/") || strings.HasPrefix(p, "/api/v1/tenants/{id}") || p == "/api/v1/tenants":
		return rule{"platform_admin", "global/resource", "platform administration"}, true
	case id != "":
		return rule{}, false
	default:
		return rule{}, false
	}
}

func renderMatrix(contract []byte, rows []struct {
	operation
	rule
}) string {
	hash := sha256.Sum256(contract)
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Policy]++
	}
	policies := make([]string, 0, len(counts))
	for policy := range counts {
		policies = append(policies, policy)
	}
	sort.Strings(policies)
	var b strings.Builder
	b.WriteString("<!-- generated by `go run ./cmd/checkauthz`; do not edit manually. -->\n")
	b.WriteString("# OpenAPI capability authorization matrix\n\n")
	fmt.Fprintf(&b, "Source: `contracts/openapi.yaml`\n\nContract SHA-256: `%s`\n\nCoverage: **%d/%d operations (100%%)**\n\n", hex.EncodeToString(hash[:]), len(rows), len(rows))
	b.WriteString("The matrix is a review artifact and a generation gate. Middleware and application services remain the enforcement points; `ownership` describes the second authorization check required after capability admission.\n\n")
	b.WriteString("## Policy summary\n\n| Policy | Operations | Required capability/auth | Ownership |\n| --- | ---: | --- | --- |\n")
	for _, policy := range policies {
		var sample rule
		for _, row := range rows {
			if row.Policy == policy {
				sample = row.rule
				break
			}
		}
		fmt.Fprintf(&b, "| `%s` | %d | `%s` | `%s` |\n", policy, counts[policy], sample.Policy, sample.Ownership)
	}
	b.WriteString("\n## Operation matrix\n\n| Method | Path | Operation ID | Policy | Ownership |\n| --- | --- | --- | --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%s` | `%s` |\n", row.Method, row.Path, row.ID, row.Policy, row.Ownership)
	}
	return b.String()
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "checkauthz: %s: %v\n", action, err)
	os.Exit(1)
}
