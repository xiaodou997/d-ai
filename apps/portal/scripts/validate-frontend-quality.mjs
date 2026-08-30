import { readdir, readFile, writeFile } from "node:fs/promises";
import { extname, relative, resolve } from "node:path";
import ts from "typescript";
import { fileURLToPath } from "node:url";

const scriptDirectory = resolve(import.meta.dirname);
const sourceRoot = resolve(scriptDirectory, "../src");
const baselinePath = resolve(scriptDirectory, "frontend-quality-baseline.json");
const sourceExtensions = new Set([".ts", ".vue"]);

async function listSourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...await listSourceFiles(path));
    } else if (sourceExtensions.has(extname(entry.name)) && !entry.name.endsWith(".test.ts")) {
      files.push(path);
    }
  }
  return files;
}

function scriptBlocks(content, file) {
  if (extname(file) !== ".vue") return [{ content, lineOffset: 0 }];
  const blocks = [];
  const pattern = /<script(?:\s[^>]*)?>([\s\S]*?)<\/script>/g;
  for (const match of content.matchAll(pattern)) {
    const start = match.index ?? 0;
    blocks.push({
      content: match[1],
      lineOffset: content.slice(0, start + match[0].indexOf(match[1])).split(/\r?\n/).length - 1
    });
  }
  return blocks;
}

function countExplicitAny(content, lineOffset) {
  const source = ts.createSourceFile("portal-quality.ts", content, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  let count = 0;
  const lines = [];
  function visit(node) {
    if (node.kind === ts.SyntaxKind.AnyKeyword) {
      count += 1;
      const position = source.getLineAndCharacterOfPosition(node.getStart(source));
      lines.push(position.line + lineOffset + 1);
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
  return { count, lines };
}

export async function findFrontendQualityViolations({ root = sourceRoot, baseline = null } = {}) {
  const allowed = baseline ?? JSON.parse(await readFile(baselinePath, "utf8"));
  const files = await listSourceFiles(root);
  const violations = [];
  for (const file of files) {
    const relativePath = relative(root, file).split("\\").join("/");
    const content = await readFile(file, "utf8");
    const result = scriptBlocks(content, file).reduce((total, block) => {
      const current = countExplicitAny(block.content, block.lineOffset);
      return { count: total.count + current.count, lines: [...total.lines, ...current.lines] };
    }, { count: 0, lines: [] });
    const entry = allowed[relativePath];
    const max = typeof entry === "number" ? entry : entry?.max ?? 0;
    if (result.count > max) {
      violations.push({
        file: relativePath,
        count: result.count,
        allowed: max,
        lines: result.lines,
        reason: typeof entry === "object" ? entry.reason : undefined
      });
    }
  }
  return violations;
}

export async function validateFrontendQuality(options = {}) {
  const violations = await findFrontendQualityViolations(options);
  if (violations.length > 0) {
    const details = violations.map(({ file, count, allowed, lines, reason }) => {
      const explanation = reason ? ` (${reason})` : "";
      return `- ${file}: ${count} explicit any, baseline allows ${allowed}; lines ${lines.join(", ")}${explanation}`;
    }).join("\n");
    throw new Error(`Portal frontend quality contract rejected new explicit any usage. Replace it with an explicit type or update the reviewed baseline with a reason.\n${details}`);
  }
  return violations;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv.includes("--write-baseline")) {
    const files = await listSourceFiles(sourceRoot);
    const baseline = {};
    for (const file of files) {
      const relativePath = relative(sourceRoot, file).split("\\").join("/");
      const content = await readFile(file, "utf8");
      const result = scriptBlocks(content, file).reduce((total, block) => {
        const current = countExplicitAny(block.content, block.lineOffset);
        return total + current.count;
      }, 0);
      if (result > 0) {
        baseline[relativePath] = {
          max: result,
          reason: "Existing reviewed boundary; reduce this baseline when the file is next touched."
        };
      }
    }
    await writeFile(baselinePath, `${JSON.stringify(baseline, null, 2)}\n`);
    console.log(`Wrote ${Object.keys(baseline).length} reviewed any baselines to ${baselinePath}`);
    process.exit(0);
  }
  await validateFrontendQuality();
  console.log("Portal frontend quality contract passed (no explicit any beyond reviewed baseline)");
}
