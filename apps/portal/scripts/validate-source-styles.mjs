import { readdir, readFile } from "node:fs/promises";
import { extname, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const sourceRoot = resolve(import.meta.dirname, "../src");
const sourceExtensions = new Set([".vue", ".ts", ".css"]);
const exemptFiles = new Set([
  "shared/ui/styles/base.css",
  "shared/ui/theme.ts"
]);
const colorLiteralPattern = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?)\s*\(|(?<!-)\b(?:white|black)\b(?!-)/g;
const radiusDeclarationPattern = /border(?:-[a-z]+)*-radius\s*:\s*([^;}{]+)/gi;
const shadowDeclarationPattern = /box-shadow\s*:\s*([^;}{]+)/gi;

function isTokenizedVisualValue(value) {
  const normalized = value.trim();
  return normalized.startsWith("var(--ds-") || ["none", "inherit", "initial", "unset"].includes(normalized);
}

async function listSourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...await listSourceFiles(path));
    } else if (sourceExtensions.has(extname(entry.name))) {
      files.push(path);
    }
  }
  return files;
}

export async function findSourceStyleViolations(root = sourceRoot) {
  const files = await listSourceFiles(root);
  const violations = [];
  for (const file of files) {
    const relativePath = relative(root, file).split("\\").join("/");
    if (exemptFiles.has(relativePath)) continue;
    const content = await readFile(file, "utf8");
    const location = (offset) => {
      const prefix = content.slice(0, offset);
      const line = prefix.split(/\r?\n/);
      return { line: line.length, column: line.at(-1).length + 1 };
    };
    for (const match of content.matchAll(colorLiteralPattern)) {
      const position = location(match.index ?? 0);
      violations.push({ file: relativePath, ...position, kind: "color", value: match[0] });
    }
    for (const match of content.matchAll(radiusDeclarationPattern)) {
      if (isTokenizedVisualValue(match[1])) continue;
      const position = location(match.index ?? 0);
      violations.push({ file: relativePath, ...position, kind: "radius", value: match[0].trim() });
    }
    for (const match of content.matchAll(shadowDeclarationPattern)) {
      if (isTokenizedVisualValue(match[1])) continue;
      const position = location(match.index ?? 0);
      violations.push({ file: relativePath, ...position, kind: "shadow", value: match[0].trim() });
    }
  }
  return violations;
}

export async function validateSourceStyles(root = sourceRoot) {
  const violations = await findSourceStyleViolations(root);
  if (violations.length > 0) {
    const details = violations
      .map(({ file, line, column, kind, value }) => `- ${file}:${line}:${column} [${kind}] ${value}`)
      .join("\n");
    throw new Error(`Portal source style contract rejected hardcoded visual values; use var(--ds-*) tokens.\n${details}`);
  }
  return violations;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await validateSourceStyles();
  console.log("Portal source style contract passed (token-based colors, radius and shadows)");
}
