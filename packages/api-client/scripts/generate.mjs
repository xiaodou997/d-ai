// 扫描 services/<svc>/api/openapi.yaml，为每个服务生成 TypeScript 类型到
// src/generated/<svc>.ts。OpenAPI 契约本身由各服务 code-first 导出（见仓库根
// README 的「契约生成链」），本脚本只负责 OpenAPI → TS 这一段。
//
// 运行：bun run --filter @dai/api-client generate|ensure
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { generationHash, generationIsCurrent } from "./generation-state.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "../../..");
const servicesDir = resolve(repoRoot, "services");
const outDir = resolve(here, "../src/generated");
const markerPath = resolve(outDir, ".openapi.sha256");
const generatorPath = resolve(repoRoot, "node_modules/.bin/openapi-typescript");
const mode = process.argv.includes("--ensure") ? "ensure" : "generate";

if (!existsSync(servicesDir)) {
  console.error("services/ 目录不存在");
  process.exit(0);
}

mkdirSync(outDir, { recursive: true });

const services = readdirSync(servicesDir, { withFileTypes: true })
  .filter((d) => d.isDirectory())
  .map((d) => d.name);

const inputs = services
  .map((service) => ({
    service,
    spec: resolve(servicesDir, service, "api/openapi.yaml"),
    output: resolve(outDir, `${service}.ts`)
  }))
  .filter(({ spec }) => existsSync(spec));

const hash = generationHash(
  inputs.map(({ service, spec }) => ({ name: service, content: readFileSync(spec) }))
);

if (
  mode === "ensure" &&
  generationIsCurrent({
    expectedHash: hash,
    marker: existsSync(markerPath) ? readFileSync(markerPath, "utf8").trim() : "",
    outputsExist: inputs.every(({ output }) => existsSync(output))
  })
) {
  console.log("OpenAPI TypeScript 类型已是最新版本。");
  process.exit(0);
}

let generated = 0;
for (const { service, spec, output } of inputs) {
  console.log(`generating: ${service} -> src/generated/${service}.ts`);
  execFileSync(generatorPath, [spec, "-o", output], { stdio: "inherit" });
  generated++;
}

if (generated === 0) {
  console.log("未发现任何 services/*/api/openapi.yaml，跳过（首个服务上线后再运行）。");
} else {
  writeFileSync(markerPath, `${hash}\n`, "utf8");
}
