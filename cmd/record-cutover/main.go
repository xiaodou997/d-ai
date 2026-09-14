package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"xiaodou/dai/internal/ai/recordcutover"
	"xiaodou/dai/internal/config"
)

type manifest struct {
	Before    recordcutover.Inventory `json:"before"`
	Assets    json.RawMessage         `json:"assets"`
	AssetHash string                  `json:"asset_sha256"`
	Dump      string                  `json:"dump"`
	DumpHash  string                  `json:"dump_sha256"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "record cutover:", err)
		os.Exit(1)
	}
}
func hashFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func run() error {
	action := flag.String("action", "inspect", "inspect, pause, backup, verify, release, resume")
	path := flag.String("backup", "", "absolute pg_dump output path on independent storage")
	offline := flag.Bool("offline", false, "the gateway, task workers and payment writers have been stopped")
	flag.Parse()
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dsn := os.Getenv("DAI_CUTOVER_DATABASE_URL")
	if dsn == "" {
		dsn = cfg.Database.BillingDSNString()
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	output := func(v any) error { e := json.NewEncoder(os.Stdout); e.SetIndent("", "  "); return e.Encode(v) }
	if *action == "inspect" {
		v, e := recordcutover.Inspect(ctx, pool)
		if e != nil {
			return e
		}
		return output(v)
	}
	if *action == "pause" || *action == "resume" {
		if *action == "resume" {
			var version int
			if e := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton`).Scan(&version); e != nil {
				return e
			}
			if version != 42 {
				return fmt.Errorf("resume requires completed schema 42 cutover")
			}
		}
		_, e := pool.Exec(ctx, `UPDATE bill_record_control SET accepting=$1,updated_at=now() WHERE singleton`, *action == "resume")
		return e
	}
	if !*offline {
		return fmt.Errorf("stop application writers and pass --offline before maintenance")
	}
	if !filepath.IsAbs(*path) {
		return fmt.Errorf("--backup must be an absolute path on independent backup storage")
	}
	if *action == "backup" {
		if _, err = os.Stat(*path); err == nil {
			return fmt.Errorf("backup path already exists")
		}
		before, e := recordcutover.Inspect(ctx, pool)
		if e != nil {
			return e
		}
		assets, assetHash, e := recordcutover.Assets(ctx, pool)
		if e != nil {
			return e
		}
		// Connection credentials are passed through the child environment, never
		// command arguments or tool output. pg_dump uses the same selected DSN.
		f, e := os.OpenFile(*path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		if e = f.Close(); e != nil {
			return e
		}
		cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-owner", "--file", *path)
		cmd.Env = append(os.Environ(), "PGDATABASE="+dsn)
		cmd.Stderr = os.Stderr
		if e = cmd.Run(); e != nil {
			return fmt.Errorf("pg_dump failed: %w", e)
		}
		if e = os.Chmod(*path, 0600); e != nil {
			return e
		}
		hash, e := hashFile(*path)
		if e != nil {
			return e
		}
		_, after, e := recordcutover.Assets(ctx, pool)
		if e != nil {
			return e
		}
		if after != assetHash {
			return fmt.Errorf("assets changed while backing up; keep writers stopped and retry with a new path")
		}
		m := manifest{Before: before, Assets: assets, AssetHash: assetHash, Dump: *path, DumpHash: hash}
		raw, e := json.MarshalIndent(m, "", "  ")
		if e != nil {
			return e
		}
		file, e := os.OpenFile(*path+".manifest.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer file.Close()
		_, e = file.Write(raw)
		return e
	}
	raw, err := os.ReadFile(*path + ".manifest.json")
	if err != nil {
		return err
	}
	var m manifest
	if err = json.Unmarshal(raw, &m); err != nil {
		return err
	}
	hash, err := hashFile(*path)
	if err != nil {
		return err
	}
	if hash != m.DumpHash {
		return fmt.Errorf("backup checksum mismatch")
	}
	current, err := recordcutover.Inspect(ctx, pool)
	if err != nil {
		return err
	}
	if current.Database != m.Before.Database || current.Schema != m.Before.Schema {
		return fmt.Errorf("backup belongs to a different database or schema")
	}
	_, assets, err := recordcutover.Assets(ctx, pool)
	if err != nil {
		return err
	}
	if assets != m.AssetHash {
		return fmt.Errorf("current assets differ from snapshot")
	}
	if *action == "verify" {
		return output(map[string]any{"asset_equality": true, "backup_verified": true})
	}
	if *action == "release" {
		after, e := recordcutover.Release(ctx, pool, m.AssetHash)
		if e != nil {
			return e
		}
		return output(map[string]any{"before": m.Before, "after": after, "asset_equality": true, "database_bytes_released": m.Before.DatabaseBytes - after.DatabaseBytes})
	}
	return fmt.Errorf("unknown action %q", *action)
}
