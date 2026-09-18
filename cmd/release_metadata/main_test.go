package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNPMPackagesFromNodeModules(t *testing.T) {
	root := filepath.Join(t.TempDir(), "node_modules")
	writePackageFixture(t, filepath.Join(root, "alpha"), `{"name":"alpha","version":"1.0.0","license":"MIT"}`)
	writePackageFixture(t, filepath.Join(root, "@scope", "beta"), `{"name":"@scope/beta","version":"2.0.0","license":"Apache-2.0"}`)
	writePackageFixture(t, filepath.Join(root, "alpha", "node_modules", "legacy"), `{"name":"legacy","version":"3.0.0","license":{"type":"BSD-3-Clause"}}`)
	writePackageFixture(t, filepath.Join(root, "unknown"), `{"name":"unknown","version":"4.0.0"}`)

	packages, err := npmPackagesFromNodeModules(root)
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string]string, len(packages))
	for _, pkg := range packages {
		got[pkg.Name+"@"+pkg.VersionInfo] = pkg.LicenseDeclared
	}
	want := map[string]string{
		"@scope/beta@2.0.0": "Apache-2.0",
		"alpha@1.0.0":       "MIT",
		"legacy@3.0.0":      "BSD-3-Clause",
		"unknown@4.0.0":     "NOASSERTION",
	}
	if len(got) != len(want) {
		t.Fatalf("package count = %d, want %d: %#v", len(got), len(want), got)
	}
	for key, license := range want {
		if got[key] != license {
			t.Errorf("%s license = %q, want %q", key, got[key], license)
		}
	}
}

func TestNPMPackagesFromNodeModulesMissing(t *testing.T) {
	_, err := npmPackagesFromNodeModules(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected missing node_modules error")
	}
}

func writePackageFixture(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
