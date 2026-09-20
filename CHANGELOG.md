# Changelog

All notable changes to D-AI are documented in this file.

## [0.3.25] - 2026-09-20

### Security and authorization

- Protect invitation bearer credentials, bind invitation use to an active tenant, and refresh the invitation authentication contract.
- Enforce disabled OAuth-pool boundaries when resolving active pool credentials.
- Encrypt sensitive upstream endpoint headers, including caller-supplied sensitive headers, before persistence.
- Sign async-task webhook deliveries with JWKS-verifiable signatures and cover the signed-delivery path with regression tests.
- Redact endpoint secrets from diagnostics and close sensitive diagnostic read paths.
- Expand multi-tenant ownership coverage for user API keys and subscription orders, and scope top-up order detail access by billing scene.

### Network security

- Harden remote image downloads against SSRF.
- Guard direct upstream HTTP egress, pin validated destination addresses, and pin proxy request targets after validation.

### Reliability

- Preserve disabled API-key and proxy states across patch/update operations.
- Preserve nullable API-key limits on patch.

### Upgrade notes

- No new database migration is introduced in v0.3.25; the schema contract remains at v46.

### Validation

- Re-run the complete Go and Portal suites, PostgreSQL no-skip gates, authorization and OpenAPI contracts, ownership regressions, release artifacts, SBOM/provenance, hardened production image, and HIGH/CRITICAL container vulnerability baseline.

## [0.3.24] - 2026-09-19

### Security and authorization

- Bind recent authentication to the current login session and require step-up authentication for MFA configuration and tenant-operations token issuance.
- Revoke pre-MFA sessions when MFA is enabled, aggregate MFA verification limits by the stable account identity, and reject reuse of the same TOTP time step across concurrent challenges.
- Make API-key runtime authorization PostgreSQL-authoritative so disable, delete, rotate, tenant suspension, and owner-account suspension take effect without relying on Redis cache invalidation timing.
- Revalidate API-key owners and JWT login sessions before every queued async-task attempt so credentials or privileges revoked while work is queued cannot continue authorizing execution.
- Treat every non-`active` account or tenant state as banned in Redis security projections and reconciliation, including locked, suspended, deleting, and purging states.
- Prevent tenant status changes from bypassing the dedicated lifecycle path that cascades account state and session revocation.
- Revoke the entire login session family when an account role or tenant scope changes so an old refresh token cannot inherit a newly elevated role or a different tenant scope.
- Keep tenant-operations tokens bound to the original operator session and current role, require recent authentication before issuance, and cap the derived token expiration at the parent access token's expiration.

### Database

- Advance the schema contract from v44 to v46.
- Add `0045_20260919_async_task_auth_reference.sql` to persist the JWT session and credential reference that authorized queued work.
- Add `0046_20260919_auth_session_context_invariant.sql` so changes to `user_type` or `tenant_id` revoke active login sessions in PostgreSQL.
- Upgrades from v0.3.23 must apply migrations 0045 and 0046 in order; do not advance `dai_schema_metadata` manually.

### Upgrade notes

- Historical queued JWT tasks that predate schema v45 intentionally have no durable authentication reference and fail closed instead of executing with an unverifiable identity.
- Role or tenant-scope changes now require affected users to sign in again because their existing refresh-token families are revoked.

### Validation

- Validate the complete Go and Portal suites, PostgreSQL no-skip gates, authorization and OpenAPI contracts, schema v46 freshness and full migration-chain replay, database ownership probes, release artifacts, SBOM/provenance, hardened production image, and HIGH/CRITICAL container vulnerability baseline.

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
