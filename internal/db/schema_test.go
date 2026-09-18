package db

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestCanonicalSchemaVersionMatchesExpected(t *testing.T) {
	raw, err := os.ReadFile("init.sql")
	if err != nil {
		t.Fatalf("read canonical schema: %v", err)
	}

	pattern := regexp.MustCompile(`(?m)^INSERT INTO dai_schema_metadata \(singleton, version\) VALUES \(TRUE, ([0-9]+)\);$`)
	matches := pattern.FindAllStringSubmatch(string(raw), -1)
	if len(matches) != 1 || len(matches[0]) != 2 {
		t.Fatal("canonical schema must insert exactly one explicit schema version")
	}

	version, err := strconv.Atoi(matches[0][1])
	if err != nil {
		t.Fatalf("parse canonical schema version: %v", err)
	}
	if version != ExpectedSchemaVersion {
		t.Fatalf("canonical schema version %d does not match expected version %d", version, ExpectedSchemaVersion)
	}
}

func TestCanonicalSchemaDoesNotDowngradeMetadataVersion(t *testing.T) {
	raw, err := os.ReadFile("init.sql")
	if err != nil {
		t.Fatalf("read canonical schema: %v", err)
	}

	pattern := regexp.MustCompile(`(?i)UPDATE\s+dai_schema_metadata\s+SET\s+version\s*=\s*([0-9]+)`)
	for _, match := range pattern.FindAllStringSubmatch(string(raw), -1) {
		if len(match) != 2 {
			continue
		}
		version, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatalf("parse canonical metadata update version %q: %v", match[1], err)
		}
		if version != ExpectedSchemaVersion {
			t.Fatalf("canonical schema later updates metadata to version %d; expected %d", version, ExpectedSchemaVersion)
		}
	}
}

func TestCanonicalSchemaRejectsNonEmptySchema(t *testing.T) {
	raw, err := os.ReadFile("init.sql")
	if err != nil {
		t.Fatalf("read canonical schema: %v", err)
	}

	schema := string(raw)
	for _, required := range []string{
		"n.nspname = current_schema()",
		"D-AI canonical schema initialization requires an empty schema",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("canonical schema is missing empty-schema guard %q", required)
		}
	}
}
