package db_test

import (
	"os"
	"testing"
)

// Historical migrations must not reverse-engineer a destructively rebuilt schema.
func legacySchema40(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/schema_0040.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
