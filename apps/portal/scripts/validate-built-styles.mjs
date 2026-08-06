import { readdir, readFile } from "node:fs/promises";
import { resolve } from "node:path";

const assetsDir = resolve(import.meta.dirname, "../../../cmd/server/frontend_dist/assets");
const bootstrapPath = resolve(import.meta.dirname, "../src/platform/bootstrap.ts");
const assetNames = await readdir(assetsDir);
const cssNames = assetNames.filter((name) => name.endsWith(".css"));
const jsNames = assetNames.filter((name) => name.endsWith(".js"));

const css = (await Promise.all(cssNames.map((name) => readFile(resolve(assetsDir, name), "utf8")))).join("\n");
const bootstrap = await readFile(bootstrapPath, "utf8");

const requiredCss = [
  ["Element Plus buttons", /\.el-button\{/],
  ["Element Plus dialogs", /\.el-dialog\{/],
  ["Tailwind flex utility", /\.flex\{display:flex\}/],
  ["Tailwind grid utility", /\.grid\{display:grid\}/],
  ["DsUI theme tokens", /--ds-accent:/]
];

const missingCss = requiredCss.filter(([, pattern]) => !pattern.test(css)).map(([label]) => label);
if (missingCss.length > 0) {
  throw new Error(`Frontend style contract is incomplete: ${missingCss.join(", ")}`);
}

if (!/app\.use\(ElementPlus,\s*\{\s*locale:\s*zhCn\s*\}\)/.test(bootstrap)) {
  throw new Error("Standard portal bootstrap does not install Element Plus globally");
}

console.log(`Frontend style contract passed (${cssNames.length} CSS assets, ${jsNames.length} JS assets)`);
