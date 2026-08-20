// Command checkdeps verifies the dependency direction of the modular monolith.
//
// The repository is being migrated incrementally. Existing architectural debt
// is recorded in docs/module-dependency-exceptions.txt; the command fails when a
// new forbidden edge appears without an explicit, reviewed exception.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const (
	modulePrefix  = "xiaodou/dai/internal/"
	defaultLedger = "docs/module-dependency-exceptions.txt"
)

type goPackage struct {
	ImportPath string
	Imports    []string
	Error      *struct {
		Err string
	} `json:"Error"`
}

type violation struct {
	From string
	To   string
	Rule string
}

type dependencyRule struct {
	Name   string
	Reason string
	Match  func(from, to string) bool
}

func main() {
	exceptionsPath := flag.String("exceptions", defaultLedger, "reviewed legacy dependency exceptions")
	flag.Parse()

	exceptions, err := loadExceptions(*exceptionsPath)
	if err != nil {
		fail(err)
	}

	packages, err := listPackages()
	if err != nil {
		fail(err)
	}

	rules := dependencyRules()
	var violations []violation
	var allowed []violation
	for _, pkg := range packages {
		if pkg.Error != nil {
			fail(fmt.Errorf("go list %s: %s", pkg.ImportPath, pkg.Error.Err))
		}
		for _, dep := range pkg.Imports {
			for _, rule := range rules {
				if !rule.Match(pkg.ImportPath, dep) {
					continue
				}
				v := violation{From: pkg.ImportPath, To: dep, Rule: rule.Name}
				if _, ok := exceptions[edgeKey(v.From, v.To)]; ok {
					allowed = append(allowed, v)
				} else {
					violations = append(violations, v)
				}
				break
			}
		}
	}

	sort.Slice(allowed, func(i, j int) bool {
		if allowed[i].From != allowed[j].From {
			return allowed[i].From < allowed[j].From
		}
		return allowed[i].To < allowed[j].To
	})
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].From != violations[j].From {
			return violations[i].From < violations[j].From
		}
		return violations[i].To < violations[j].To
	})

	if len(allowed) > 0 {
		fmt.Printf("module-deps: %d reviewed legacy edge(s) allowed\n", len(allowed))
	}
	if len(violations) == 0 {
		fmt.Println("module-deps: dependency direction is clean")
		return
	}

	fmt.Fprintf(os.Stderr, "module-deps: %d unreviewed forbidden edge(s):\n", len(violations))
	for _, v := range violations {
		fmt.Fprintf(os.Stderr, "  %s -> %s (%s)\n", v.From, v.To, v.Rule)
	}
	fmt.Fprintln(os.Stderr, "Add a narrowly-scoped exception with an owner and removal target only when the edge is intentional.")
	os.Exit(1)
}

func dependencyRules() []dependencyRule {
	return []dependencyRule{
		{
			Name:   "transport-must-not-own-persistence",
			Reason: "HTTP transport must call application ports instead of repositories, sqlc, or ledger internals",
			Match: func(from, to string) bool {
				return isTransport(from) && isPersistencePackage(to)
			},
		},
		{
			Name:   "transport-must-not-import-infrastructure",
			Reason: "HTTP transport must not depend directly on PostgreSQL or Redis clients",
			Match: func(from, to string) bool {
				return isTransport(from) && isInfrastructureImport(to)
			},
		},
		{
			Name:   "inner-layers-must-not-import-infrastructure",
			Reason: "domain, ports, and application layers cannot point at adapters or database clients",
			Match: func(from, to string) bool {
				return isInnerLayer(from) && (isPersistencePackage(to) || isInfrastructureImport(to))
			},
		},
		{
			Name:   "modules-must-not-import-http-transport",
			Reason: "HTTP registration is an outer adapter and cannot be a domain dependency",
			Match: func(from, to string) bool {
				return !isTransport(from) && isTransport(to)
			},
		},
	}
}

func isTransport(path string) bool {
	return path == modulePrefix+"transport" || strings.HasSuffix(path, "/transport")
}

func isInnerLayer(path string) bool {
	return strings.HasSuffix(path, "/domain") ||
		strings.Contains(path, "/domain/") ||
		strings.HasSuffix(path, "/ports") ||
		strings.Contains(path, "/ports/") ||
		strings.HasSuffix(path, "/application") ||
		strings.Contains(path, "/application/") ||
		strings.Contains(path, "/core/")
}

func isPersistencePackage(path string) bool {
	if !strings.HasPrefix(path, modulePrefix) {
		return false
	}
	return strings.Contains(path, "/adapters/") ||
		strings.Contains(path, "/db/") ||
		strings.HasSuffix(path, "/db") ||
		strings.Contains(path, "/pg/") ||
		strings.HasSuffix(path, "/pg") ||
		path == modulePrefix+"billing/ledger" ||
		path == modulePrefix+"billing/outbox"
}

func isInfrastructureImport(path string) bool {
	return path == "github.com/jackc/pgx/v5" ||
		strings.HasPrefix(path, "github.com/jackc/pgx/v5/") ||
		strings.HasPrefix(path, "github.com/redis/go-redis/v9")
}

func edgeKey(from, to string) string {
	return from + " -> " + to
}

func loadExceptions(path string) (map[string]struct{}, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open dependency exceptions %q: %w", path, err)
	}
	defer file.Close()

	exceptions := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		entry := strings.TrimSpace(strings.SplitN(text, "|", 2)[0])
		if !strings.Contains(entry, " -> ") {
			return nil, fmt.Errorf("%s:%d: expected '<from> -> <to> | ...'", path, line)
		}
		exceptions[entry] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read dependency exceptions %q: %w", path, err)
	}
	return exceptions, nil
}

func listPackages() ([]goPackage, error) {
	cmd := exec.Command("go", "list", "-json", "./internal/...")
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("go list ./internal/... failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("run go list ./internal/...: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []goPackage
	for decoder.More() {
		var pkg goPackage
		if err := decoder.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		packages = append(packages, pkg)
	}
	return packages, nil
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "module-deps: %v\n", err)
	os.Exit(1)
}
