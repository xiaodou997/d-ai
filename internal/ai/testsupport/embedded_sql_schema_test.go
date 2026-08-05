package testsupport

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var sqlAuditStart = regexp.MustCompile(`(?is)^\s*(?:--[^\n]*\n\s*)*(select|insert|update|delete|with)\b`)

func TestEmbeddedSQLMatchesCanonicalSchema(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := OpenAsyncTaskTestPool(ctx, AsyncTaskPoolOptions{MaxConns: 1})
	if err != nil {
		t.Skipf("embedded SQL schema audit database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	// V2 cutover SQL intentionally targets a table that is absent from fresh
	// V3 schemas. Recreate only its retired shape inside this isolated audit
	// schema so the compatibility command's SQL still receives PREPARE-level
	// validation without restoring the table to canonical production DDL.
	if _, err := pool.Exec(ctx, `
		CREATE TABLE ai_user_credit_ledger (
		  owner_type TEXT NOT NULL,
		  tenant_id TEXT NOT NULL,
		  user_id TEXT NOT NULL DEFAULT '',
		  pending_tenant_micro BIGINT NOT NULL DEFAULT 0,
		  pending_user_micro BIGINT NOT NULL DEFAULT 0,
		  settled_tenant_micro BIGINT NOT NULL DEFAULT 0,
		  settled_user_micro BIGINT NOT NULL DEFAULT 0,
		  settle_window_id TEXT,
		  settle_window_tenant_micro BIGINT NOT NULL DEFAULT 0,
		  settle_window_user_micro BIGINT NOT NULL DEFAULT 0,
		  settle_window_opened_at TIMESTAMPTZ,
		  last_settled_at TIMESTAMPTZ,
		  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		  PRIMARY KEY (owner_type, tenant_id, user_id)
		)
	`); err != nil {
		t.Fatalf("create retired ledger SQL audit fixture: %v", err)
	}

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	type candidate struct {
		location string
		sql      string
	}
	candidates := make([]candidate, 0, 256)
	seen := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		formattedLiterals := map[token.Pos]bool{}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Sprintf" {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || pkg.Name != "fmt" {
				return true
			}
			if literal, ok := call.Args[0].(*ast.BasicLit); ok {
				formattedLiterals[literal.Pos()] = true
			}
			return true
		})
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			if formattedLiterals[literal.Pos()] {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			value = strings.TrimSpace(value)
			if !sqlAuditStart.MatchString(value) || !strings.Contains(value, "ai_") {
				return true
			}
			value = strings.TrimSuffix(value, ";")
			if seen[value] {
				return true
			}
			seen[value] = true
			pos := fset.Position(literal.Pos())
			candidates = append(candidates, candidate{
				location: fmt.Sprintf("%s:%d", strings.TrimPrefix(pos.Filename, root+string(filepath.Separator)), pos.Line),
				sql:      value,
			})
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) < 300 {
		t.Fatalf("embedded SQL audit found only %d candidates; scanner likely regressed", len(candidates))
	}

	checked := 0
	for index, item := range candidates {
		name := fmt.Sprintf("sql_audit_%d", index)
		_, err := pool.Exec(ctx, "PREPARE "+name+" AS "+item.sql)
		if err == nil {
			checked++
			_, _ = pool.Exec(ctx, "DEALLOCATE "+name)
			continue
		}
		t.Errorf("%s: %v\nSQL: %s", item.location, err, item.sql)
	}
	t.Logf("prepared %d of %d embedded SQL candidates", checked, len(candidates))
}
