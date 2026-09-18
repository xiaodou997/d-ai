# Changelog

All notable changes to D-AI are documented in this file.

## [0.3.23] - 2026-09-18

### Security and reliability

- Serialize JWT signing-key bootstrap and rotation across processes with a PostgreSQL transaction-level advisory lock so concurrent replicas cannot commit conflicting active signing keys.
- Enforce the signing-key database invariant with a partial unique index allowing at most one `status='active'` row, and make key reload fail closed when it sees zero or multiple active keys.
- Add deterministic concurrent two-replica rotation coverage, cross-replica signing/verification checks, and invalid active-key-count regression tests.
- This patch supersedes v0.3.22 for deployments that rely on JWT key rotation; v0.3.22 contains the unknown-`kid` refresh fix but not the active-key invariant fix.

### Database

- Advance the schema contract to v44 with `0044_20260918_jwt_active_key_invariant.sql`.
- The v44 migration refuses to proceed if the database already contains more than one active JWT signing key; inspect and repair that state before retrying the migration.

### Validation

- Keep the complete Go, Portal, PostgreSQL integration, migration-chain replay, database ownership, release-evidence, and hardened-container CI gates green after the invariant fix.

## [0.3.22] - 2026-09-18

### Security and reliability

- Refresh JWT signing keys across replicas when verification encounters a locally unknown `kid`, while serializing reloads and throttling forged-`kid` fallback database work.
- Update vulnerable Go and frontend dependency baselines and refresh the hardened production-image package baseline.
- Keep the full Go, Portal, database, schema, billing-invariant, and container security gates green in CI.

### Platform

- Remove the unused batch AI usage refund endpoint and its generated Portal/OpenAPI surface.
- Retain the single-request settlement reversal path with idempotency, original payer split restoration, and audit evidence.

### Release engineering

- Generate SBOM, provenance, and SHA-256 manifests for release builds.
- Validate standalone and embedded Portal artifacts, database release helpers, production image hardening, and HIGH/CRITICAL container vulnerabilities.
- Publish validated release bundles from successful `main` CI runs and create a GitHub Release for the package version.

### Breaking changes

- Removed `POST /api/v1/ai/usage/batch-refund` (`admin-batch-refund-usage`). Use the retained single-request settlement reversal workflow where an operator correction is required.
