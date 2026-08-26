// Command checkschema validates the forward-only PostgreSQL schema chain and
// renders its review inventory. It intentionally never connects to a database
// or executes SQL; production migrations remain an explicit release step.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"xiaodou/dai/internal/db"
)

const (
	initPath                 = "internal/db/init.sql"
	changesPath              = "internal/db/changes"
	rollbackPath             = "internal/db/rollback"
	docsPath                 = "docs/SCHEMA_CHAIN.md"
	filenameRegexp           = `^(\d{4})_(\d{8})_([a-z0-9][a-z0-9_]*)\.sql$`
	historicalBaselineCommit = "54135ad"
)

var (
	initVersionRegexp = regexp.MustCompile(`(?is)INSERT\s+INTO\s+dai_schema_metadata\s*\(\s*singleton\s*,\s*version\s*\)\s*VALUES\s*\(\s*TRUE\s*,\s*(\d+)\s*\)`)
	filenamePattern   = regexp.MustCompile(filenameRegexp)
	headerFromRegexp  = regexp.MustCompile(`(?im)^\s*--\s*from_version\s*:\s*(\d+)\s*$`)
	headerToRegexp    = regexp.MustCompile(`(?im)^\s*--\s*to_version\s*:\s*(\d+)\s*$`)
	legacyHeaderRegex = regexp.MustCompile(`(?im)^\s*--\s*schema\s+(\d+)\s*(?:->|→|–)\s*(\d+)\s*:\s*(.*)$`)
	versionUpdate     = regexp.MustCompile(`(?is)\bUPDATE\s+dai_schema_metadata\s+SET\s+version\s*=\s*(\d+)\b`)
)

type migration struct {
	Filename         string
	Number           int
	Date             string
	Description      string
	From             int
	To               int
	HasTransaction   bool
	HasAdvisoryLock  bool
	HasVersionGuard  bool
	HasVersionUpdate bool
	HasTest          bool
	HasRollback      bool
	SHA256           string
}

func main() {
	baseline, err := readBaseline(initPath)
	if err != nil {
		fail("read baseline", err)
	}
	migrations, err := readMigrations(changesPath, rollbackPath)
	if err != nil {
		fail("read migrations", err)
	}
	if err := validateChain(baseline, db.ExpectedSchemaVersion, migrations); err != nil {
		fail("validate chain", err)
	}
	if err := os.WriteFile(docsPath, []byte(renderDocument(baseline, migrations)), 0o644); err != nil {
		fail("write inventory", err)
	}
	from := 0
	if len(migrations) > 0 {
		from = migrations[0].From
	}
	fmt.Printf("schema chain: v%d -> v%d, %d forward migrations validated; wrote %s\n", from, baseline, len(migrations), docsPath)
}

func readBaseline(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	match := initVersionRegexp.FindStringSubmatch(string(content))
	if len(match) != 2 {
		return 0, fmt.Errorf("%s has no explicit dai_schema_metadata baseline insert", path)
	}
	version, err := strconv.Atoi(match[1])
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("invalid baseline schema version %q", match[1])
	}
	return version, nil
}

func readMigrations(changesDir, rollbackDir string) ([]migration, error) {
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return nil, err
	}
	rollbacks, err := rollbackFiles(rollbackDir)
	if err != nil {
		return nil, err
	}
	migrations := make([]migration, 0, len(entries))
	seen := map[int]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("unexpected directory %q in %s", entry.Name(), changesDir)
		}
		if entry.Name() == "README.md" {
			continue
		}
		match := filenamePattern.FindStringSubmatch(entry.Name())
		if len(match) != 4 {
			return nil, fmt.Errorf("unexpected file %q in %s; expected NNNN_YYYYMMDD_description.sql", entry.Name(), changesDir)
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("invalid migration number in %q: %w", entry.Name(), err)
		}
		if previous, ok := seen[number]; ok {
			return nil, fmt.Errorf("duplicate migration number %04d in %q and %q", number, previous, entry.Name())
		}
		seen[number] = entry.Name()
		path := filepath.Join(changesDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		parsed, err := parseMigration(entry.Name(), string(content))
		if err != nil {
			return nil, err
		}
		parsed.HasTest = migrationTestExists(changesDir, number)
		parsed.HasRollback = rollbacks[number]
		parsed.SHA256 = hash(content)
		migrations = append(migrations, parsed)
	}
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no SQL migrations found in %s", changesDir)
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Number < migrations[j].Number })
	return migrations, nil
}

func rollbackFiles(dir string) (map[int]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	rollbacks := make(map[int]bool)
	pattern := regexp.MustCompile(`^(\d{4})_rollback\.sql$`)
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("unexpected directory %q in %s", entry.Name(), dir)
		}
		if entry.Name() == "README.md" {
			continue
		}
		match := pattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 {
			return nil, fmt.Errorf("unexpected file %q in %s; expected NNNN_rollback.sql", entry.Name(), dir)
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("invalid rollback number in %q: %w", entry.Name(), err)
		}
		if rollbacks[number] {
			return nil, fmt.Errorf("duplicate rollback number %04d", number)
		}
		rollbacks[number] = true
	}
	return rollbacks, nil
}

func parseMigration(filename, content string) (migration, error) {
	match := filenamePattern.FindStringSubmatch(filename)
	if len(match) != 4 {
		return migration{}, fmt.Errorf("invalid migration filename %q", filename)
	}
	number, _ := strconv.Atoi(match[1])
	from, to, description, err := parseVersions(content)
	if err != nil {
		return migration{}, fmt.Errorf("%s: %w", filename, err)
	}
	updates := versionUpdate.FindAllStringSubmatch(content, -1)
	if len(updates) != 1 {
		return migration{}, fmt.Errorf("%s: expected exactly one UPDATE dai_schema_metadata, found %d", filename, len(updates))
	}
	updatedTo, err := strconv.Atoi(updates[0][1])
	if err != nil {
		return migration{}, fmt.Errorf("%s: invalid target version %q", filename, updates[0][1])
	}
	updateAt := strings.Index(strings.ToLower(content), "update dai_schema_metadata")
	prefix := content
	if updateAt >= 0 {
		prefix = content[:updateAt]
	}
	guard := regexp.MustCompile(fmt.Sprintf(`(?is)(?:dai_schema_metadata[\s\S]{0,500}(?:version\s*=\s*%d|is\s+distinct\s+from\s+%d)|version[\s\S]{0,500}dai_schema_metadata[\s\S]{0,500}(?:=\s*%d|is\s+distinct\s+from\s+%d))`, from, from, from, from))
	return migration{
		Filename:         filename,
		Number:           number,
		Date:             match[2],
		Description:      description,
		From:             from,
		To:               to,
		HasTransaction:   hasSQLWord(content, "BEGIN") && hasSQLWord(content, "COMMIT"),
		HasAdvisoryLock:  strings.Contains(strings.ToLower(content), "pg_advisory_xact_lock"),
		HasVersionGuard:  guard.MatchString(prefix),
		HasVersionUpdate: updatedTo == to,
	}, nil
}

func parseVersions(content string) (from, to int, description string, err error) {
	fromMatch := headerFromRegexp.FindStringSubmatch(content)
	toMatch := headerToRegexp.FindStringSubmatch(content)
	if len(fromMatch) == 2 && len(toMatch) == 2 {
		from, err = strconv.Atoi(fromMatch[1])
		if err != nil {
			return 0, 0, "", fmt.Errorf("invalid from_version %q", fromMatch[1])
		}
		to, err = strconv.Atoi(toMatch[1])
		if err != nil {
			return 0, 0, "", fmt.Errorf("invalid to_version %q", toMatch[1])
		}
		if match := regexp.MustCompile(`(?im)^\s*--\s*description\s*:\s*(.+?)\s*$`).FindStringSubmatch(content); len(match) == 2 {
			description = strings.TrimSpace(match[1])
		}
	} else if match := legacyHeaderRegex.FindStringSubmatch(content); len(match) == 4 {
		from, err = strconv.Atoi(match[1])
		if err != nil {
			return 0, 0, "", fmt.Errorf("invalid legacy source version %q", match[1])
		}
		to, err = strconv.Atoi(match[2])
		if err != nil {
			return 0, 0, "", fmt.Errorf("invalid legacy target version %q", match[2])
		}
		description = strings.TrimSpace(match[3])
	} else {
		return 0, 0, "", fmt.Errorf("missing from/to version header")
	}
	if description == "" {
		description = "(description not declared)"
	}
	return from, to, description, nil
}

func validateChain(baseline, expected int, migrations []migration) error {
	if baseline != expected {
		return fmt.Errorf("init.sql baseline v%d does not match db.ExpectedSchemaVersion v%d", baseline, expected)
	}
	if len(migrations) == 0 {
		return fmt.Errorf("migration chain is empty")
	}
	previous := migrations[0].From
	for index, migration := range migrations {
		if migration.Number != migration.To {
			return fmt.Errorf("%s: filename number %04d must equal target version %d", migration.Filename, migration.Number, migration.To)
		}
		if migration.To != migration.From+1 {
			return fmt.Errorf("%s: migration must advance exactly one version (%d -> %d)", migration.Filename, migration.From, migration.To)
		}
		if index > 0 && migration.From != previous {
			return fmt.Errorf("%s: chain gap or overlap; expected source v%d, found v%d", migration.Filename, previous, migration.From)
		}
		if !migration.HasTransaction {
			return fmt.Errorf("%s: migration must contain BEGIN and COMMIT", migration.Filename)
		}
		if !migration.HasVersionGuard {
			return fmt.Errorf("%s: no source-version guard before metadata update", migration.Filename)
		}
		if !migration.HasVersionUpdate {
			return fmt.Errorf("%s: metadata update target does not match v%d", migration.Filename, migration.To)
		}
		previous = migration.To
	}
	if previous != baseline {
		return fmt.Errorf("migration chain ends at v%d, baseline is v%d", previous, baseline)
	}
	return nil
}

func renderDocument(baseline int, migrations []migration) string {
	baselineContent, _ := os.ReadFile(initPath)
	baselineHash := hash(baselineContent)
	var b strings.Builder
	b.WriteString("<!-- generated by `go run ./cmd/checkschema`; do not edit manually. -->\n")
	b.WriteString("# Schema migration chain\n\n")
	fmt.Fprintf(&b, "Baseline: `internal/db/init.sql` v**%d**<br>\nRuntime contract: `internal/db/schema.go` v**%d**<br>\nBaseline SHA-256: `%s`<br>\nForward chain: **v%d → v%d**, %d migrations, no gaps\n\n", baseline, db.ExpectedSchemaVersion, baselineHash, migrations[0].From, baseline, len(migrations))
	b.WriteString("This is a generated review artifact. `init.sql` is the complete empty-database baseline; `changes/` is the forward-only upgrade chain for existing databases. The application only verifies the metadata version and never executes these scripts. Release automation must apply the required SQL explicitly, with backup and recovery controls.\n\n")
	fmt.Fprintf(&b, "Replay provenance: immutable v1 baseline commit `%s`; the earlier pre-release snapshot is not used because unversioned billing changes landed before the forward chain began.\n\n", historicalBaselineCommit)
	b.WriteString("## Migration inventory\n\n")
	b.WriteString("| Target | File | Source | Transaction | Source guard | Advisory lock | Test | SQL rollback | SHA-256 |\n| ---: | --- | ---: | :---: | :---: | :---: | :---: | :---: | --- |\n")
	for _, migration := range migrations {
		fmt.Fprintf(&b, "| v%d | `%s` | v%d | %s | %s | %s | %s | %s | `%s` |\n", migration.To, migration.Filename, migration.From, yesNo(migration.HasTransaction), yesNo(migration.HasVersionGuard), yesNo(migration.HasAdvisoryLock), yesNo(migration.HasTest), yesNo(migration.HasRollback), migration.SHA256[:12])
	}
	b.WriteString("\n`Test` means a matching `internal/db/migration_NNNN_test.go` exists. `SQL rollback` only reports a checked-in rollback script; forward-only migrations without one must use a verified database backup for recovery.\n\n")
	b.WriteString("## Release invariants\n\n")
	b.WriteString("- Apply scripts in ascending target-version order; never edit an already released script or jump by changing `dai_schema_metadata.version`.\n- Validate the target database backup, migration result, and application compatibility before restoring traffic.\n- A new migration must update `init.sql`, `internal/db/schema.go`, and this generated inventory in the same change.\n- `go run ./cmd/checkschema` is the structural gate; `scripts/replay_schema_chain.sh` replays the immutable v1 Git baseline from commit `" + historicalBaselineCommit + "` and compares the final schema-only dump with `init.sql`.\n")
	return b.String()
}

func migrationTestExists(changesDir string, number int) bool {
	_, err := os.Stat(filepath.Join(changesDir, "..", fmt.Sprintf("migration_%04d_test.go", number)))
	return err == nil
}

func hasSQLWord(content, word string) bool {
	pattern := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(word) + `\b`)
	return pattern.MatchString(content)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "checkschema: %s: %v\n", action, err)
	os.Exit(1)
}
