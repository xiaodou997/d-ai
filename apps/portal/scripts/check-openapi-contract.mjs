import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "yaml";

const repoRoot = resolve(import.meta.dirname, "../../..");
const specPath = resolve(repoRoot, "contracts/openapi.yaml");
const generatedPath = resolve(repoRoot, "apps/portal/src/api/generated/dai.ts");
const markerPath = resolve(repoRoot, "apps/portal/src/api/generated/.openapi.sha256");
const approvedRemovalsPath = resolve(repoRoot, "contracts/openapi-approved-removals.json");

export function operationIdsFromSpec(spec) {
  const ids = [];
  for (const [path, item] of Object.entries(spec?.paths ?? {})) {
    for (const [method, operation] of Object.entries(item ?? {})) {
      if (!["get", "put", "post", "patch", "delete", "options", "head", "trace"].includes(method)) continue;
      if (typeof operation?.operationId !== "string" || operation.operationId.trim() === "") {
        throw new Error(`OpenAPI operation ${method.toUpperCase()} ${path} is missing operationId`);
      }
      ids.push(operation.operationId);
    }
  }
  return ids;
}

export function operationIdsFromGenerated(source) {
  const section = source.slice(source.indexOf("export interface operations {") );
  return [...section.matchAll(/^    "([^"]+)": \{/gm)].map((match) => match[1]);
}

function assertUnique(values, label) {
  const duplicates = values.filter((value, index) => values.indexOf(value) !== index);
  if (duplicates.length) throw new Error(`${label} contains duplicate operationId(s): ${[...new Set(duplicates)].join(", ")}`);
}

function assertSame(expected, actual) {
  const expectedSet = new Set(expected);
  const actualSet = new Set(actual);
  const missing = expected.filter((value) => !actualSet.has(value));
  const extra = actual.filter((value) => !expectedSet.has(value));
  if (missing.length || extra.length) {
    throw new Error(`generated operations differ from OpenAPI (missing: ${missing.join(", ") || "none"}; extra: ${extra.join(", ") || "none"})`);
  }
}

function baselineSource() {
  const ref = process.env.OPENAPI_BASELINE_REF?.trim();
  if (!ref) return "";
  try {
    return execFileSync("git", ["show", `${ref}:contracts/openapi.yaml`], { cwd: repoRoot, encoding: "utf8" });
  } catch (error) {
    throw new Error(`cannot read OpenAPI baseline ${ref}: ${error.message}`);
  }
}

function approvedRemovalIds() {
  const ref = process.env.OPENAPI_BASELINE_REF?.trim();
  if (!ref || !existsSync(approvedRemovalsPath)) return [];
  const approval = JSON.parse(readFileSync(approvedRemovalsPath, "utf8"));
  if (approval?.baselineSha !== ref) return [];
  if (!Array.isArray(approval?.removedOperationIds) || approval.removedOperationIds.some((id) => typeof id !== "string" || !id.trim())) {
    throw new Error("approved OpenAPI removals must define non-empty removedOperationIds");
  }
  return approval.removedOperationIds;
}

export function checkContract({ specSource, generatedSource, marker = "", baselineSource: baseline = "", approvedRemovedOperationIds = [] }) {
  const spec = parse(specSource);
  const currentIds = operationIdsFromSpec(spec);
  assertUnique(currentIds, "OpenAPI");
  const generatedIds = operationIdsFromGenerated(generatedSource);
  assertUnique(generatedIds, "generated operations");
  assertSame(currentIds, generatedIds);
  if (baseline) {
    const baselineIds = operationIdsFromSpec(parse(baseline));
    const removed = baselineIds.filter((id) => !currentIds.includes(id));
    const approved = new Set(approvedRemovedOperationIds);
    const unapproved = removed.filter((id) => !approved.has(id));
    if (unapproved.length && process.env.ALLOW_BREAKING_OPENAPI !== "1") {
      throw new Error(`breaking OpenAPI change removed unapproved operationId(s): ${unapproved.join(", ")}; add an exact baseline-scoped approval or set ALLOW_BREAKING_OPENAPI=1 only with an approved migration`);
    }
  }
  if (!marker.trim()) throw new Error("generated OpenAPI marker is missing; run bun run generate:api");
  return { operationCount: currentIds.length };
}

if (process.argv[1] && fileURLToPath(import.meta.url) === resolve(process.argv[1])) {
  if (!existsSync(specPath) || !existsSync(generatedPath)) {
    throw new Error("OpenAPI contract or generated operations output is missing");
  }
  const result = checkContract({
    specSource: readFileSync(specPath, "utf8"),
    generatedSource: readFileSync(generatedPath, "utf8"),
    marker: existsSync(markerPath) ? readFileSync(markerPath, "utf8") : "",
    baselineSource: baselineSource(),
    approvedRemovedOperationIds: approvedRemovalIds()
  });
  console.log(`OpenAPI contract gate passed (${result.operationCount} operations)`);
}
