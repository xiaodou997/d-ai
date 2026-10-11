import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { gzipSync } from "node:zlib";

const distDir = resolve(import.meta.dirname, process.env.DAI_PORTAL_DIST?.trim() || "../../../cmd/server/frontend_dist");
const manifest = JSON.parse(await readFile(resolve(distDir, ".vite/manifest.json"), "utf8"));

// Count each static dependency once, including extracted CSS. Dynamic imports
// are counted only when their route is selected, just as in the browser.
function assetsFor(entries) {
  const visited = new Set();
  const assets = new Set();
  function visit(key) {
    if (visited.has(key)) return;
    visited.add(key);
    const chunk = manifest[key];
    if (!chunk) throw new Error(`Missing bundle manifest entry: ${key}`);
    assets.add(chunk.file);
    for (const css of chunk.css ?? []) assets.add(css);
    for (const dependency of chunk.imports ?? []) visit(dependency);
  }
  entries.forEach(visit);
  return assets;
}

const budgets = [
  { name: "Entry", entries: ["index.html"], js: 150_000, css: 20_000 },
  { name: "Login (including entry)", entries: ["index.html", "src/views/LoginView.vue"], js: 165_000, css: 22_000 },
  { name: "Shell (including entry, excluding page)", entries: ["index.html", "src/views/LayoutView.vue"], js: 215_000, css: 35_000 }
];

const contents = new Map();
const failures = [];
for (const budget of budgets) {
  const totals = { js: { raw: 0, gzip: 0 }, css: { raw: 0, gzip: 0 } };
  for (const file of assetsFor(budget.entries)) {
    const kind = file.endsWith(".js") ? "js" : file.endsWith(".css") ? "css" : null;
    if (!kind) continue;
    if (!contents.has(file)) contents.set(file, await readFile(resolve(distDir, file)));
    const bytes = contents.get(file);
    totals[kind].raw += bytes.length;
    totals[kind].gzip += gzipSync(bytes).length;
  }
  for (const kind of ["js", "css"]) {
    const { raw, gzip } = totals[kind];
    console.log(`${budget.name}: ${kind.toUpperCase()} ${(raw / 1000).toFixed(1)} kB, gzip ${(gzip / 1000).toFixed(1)} kB (limit ${budget[kind] / 1000} kB)`);
    if (gzip > budget[kind]) failures.push(`${budget.name} ${kind.toUpperCase()} exceeds its gzip budget`);
  }
}
if (failures.length) throw new Error(failures.join("; "));
