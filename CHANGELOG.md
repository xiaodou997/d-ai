# Changelog

All notable changes to D-AI are documented in this file.

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
