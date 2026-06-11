import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const nodeModulesPath = join(process.cwd(), "node_modules");
const goModPath = join(nodeModulesPath, "go.mod");

if (existsSync(nodeModulesPath)) {
  writeFileSync(goModPath, "module clamav-desktop/app/frontend/node_modules\n\ngo 1.23.0\n");
} else {
  mkdirSync(nodeModulesPath, { recursive: true });
  writeFileSync(goModPath, "module clamav-desktop/app/frontend/node_modules\n\ngo 1.23.0\n");
}
