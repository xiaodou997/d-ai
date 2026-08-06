import { access, readFile } from "node:fs/promises";
import { resolve } from "node:path";

const portalRoot = resolve(import.meta.dirname, "..");
const sourceRoot = resolve(portalRoot, "src");
const router = await readFile(resolve(sourceRoot, "router.ts"), "utf8");
const menuStore = await readFile(resolve(sourceRoot, "stores/menus.ts"), "utf8");
const moduleRegistry = await readFile(resolve(sourceRoot, "modules/portalModules.ts"), "utf8");

const requiredContracts = [
  ["router generation", router, /buildPortalModuleRoutes\(\)/],
  ["route capability resolver", router, /hasCapability:\s*userHasPortalCapability/],
  ["menu generation", menuStore, /buildPortalNav\(/],
  ["module capability metadata", moduleRegistry, /capability:\s*"[a-z.]+"/]
];

for (const [label, source, pattern] of requiredContracts) {
  if (!pattern.test(source)) throw new Error(`Portal architecture contract missing: ${label}`);
}

const removedLegacyFiles = [
  resolve(sourceRoot, "menus/unifiedMenus.ts"),
  resolve(sourceRoot, "views/RoleBasedView.vue")
];

for (const file of removedLegacyFiles) {
  try {
    await access(file);
    throw new Error(`Legacy portal architecture returned: ${file}`);
  } catch (error) {
    if (error instanceof Error && error.message.startsWith("Legacy portal architecture returned")) throw error;
  }
}

console.log("Portal architecture contract passed");
