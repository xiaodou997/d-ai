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
    const lines = content.split(/\r?\n/);
    lines.forEach((line, lineIndex) => {
      for (const match of line.matchAll(colorLiteralPattern)) {
        violations.push({
          file: relativePath,
          line: lineIndex + 1,
          column: (match.index ?? 0) + 1,
          value: match[0]
        });
      }
    });
  }
  return violations;
}

export async function validateSourceStyles(root = sourceRoot) {
  const violations = await findSourceStyleViolations(root);
  if (violations.length > 0) {
    const details = violations
      .map(({ file, line, column, value }) => `- ${file}:${line}:${column} ${value}`)
      .join("\n");
    throw new Error(`Portal source style contract rejected hardcoded colors; use var(--ds-*) tokens.\n${details}`);
  }
  return violations;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await validateSourceStyles();
  console.log("Portal source style contract passed (token-based colors only)");
}
