// Generate the Portal's TypeScript contract types from the single backend
// OpenAPI snapshot. Runtime API facades live beside the generated output in
// apps/portal/src/api; this script owns only the code-first contract boundary.
//
// Run with `bun run generate:api` or `bun run ensure:api`.
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { generationHash, generationIsCurrent } from "./generation-state.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "../../..");
const specPath = resolve(repoRoot, "contracts/openapi.yaml");
const outDir = resolve(here, "../src/api/generated");
const outputPath = resolve(outDir, "dai.ts");
const markerPath = resolve(outDir, ".openapi.sha256");
const generatorPath = resolve(repoRoot, "node_modules/.bin/openapi-typescript");
const mode = process.argv.includes("--ensure") ? "ensure" : "generate";

if (!existsSync(specPath)) {
  console.error(`统一 OpenAPI 契约不存在: ${specPath}`);
  console.error("请先运行 `make openapi`。");
  process.exit(1);
}

mkdirSync(outDir, { recursive: true });
const hash = generationHash([{ name: "dai", content: readFileSync(specPath) }]);

if (
  mode === "ensure" &&
  generationIsCurrent({
    expectedHash: hash,
    marker: existsSync(markerPath) ? readFileSync(markerPath, "utf8").trim() : "",
    outputsExist: existsSync(outputPath)
  })
) {
  console.log("Portal OpenAPI 类型已是最新版本。");
  process.exit(0);
}

console.log("generating: contracts/openapi.yaml -> src/api/generated/dai.ts");
execFileSync(generatorPath, [specPath, "-o", outputPath], { stdio: "inherit" });
writeFileSync(markerPath, `${hash}\n`, "utf8");
