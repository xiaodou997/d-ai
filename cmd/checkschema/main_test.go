package main

import (
	"os"
	"testing"

	"xiaodou/dai/internal/db"
)

func TestCurrentSchemaChainIsContinuous(t *testing.T) {
	baseline, err := readBaseline("../../internal/db/init.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := readMigrations("../../internal/db/changes", "../../internal/db/rollback")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateChain(baseline, db.ExpectedSchemaVersion, migrations); err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 10 {
		t.Fatalf("parsed %d migrations, expected the complete chain", len(migrations))
	}
}

func TestMigrationInventoryIdentifiesCoverageGaps(t *testing.T) {
	migrations, err := readMigrations("../../internal/db/changes", "../../internal/db/rollback")
	if err != nil {
		t.Fatal(err)
	}
	byNumber := make(map[int]migration, len(migrations))
	for _, item := range migrations {
		byNumber[item.Number] = item
	}
	for _, number := range []int{2, 3, 9} {
		if byNumber[number].HasTest {
			t.Errorf("migration %04d unexpectedly reports a dedicated test", number)
		}
	}
	for _, number := range []int{4, 15, 18} {
		if !byNumber[number].HasTest {
			t.Errorf("migration %04d should report its dedicated test", number)
		}
	}
	if !byNumber[3].HasRollback {
		t.Fatal("migration 0003 should report its checked-in rollback")
	}
}

func TestReadBaselineRequiresExplicitMetadataVersion(t *testing.T) {
	path := t.TempDir() + "/init.sql"
	if err := os.WriteFile(path, []byte("CREATE TABLE dai_schema_metadata (version integer);"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBaseline(path); err == nil {
		t.Fatal("readBaseline accepted a baseline without explicit metadata version")
	}
}

func TestValidateChainRejectsGap(t *testing.T) {
	chain := []migration{
		{Filename: "0002_test.sql", Number: 2, From: 1, To: 2, HasTransaction: true, HasVersionGuard: true, HasVersionUpdate: true},
		{Filename: "0004_test.sql", Number: 4, From: 3, To: 4, HasTransaction: true, HasVersionGuard: true, HasVersionUpdate: true},
	}
	if err := validateChain(4, 4, chain); err == nil {
		t.Fatal("validateChain accepted a missing version")
	}
}
