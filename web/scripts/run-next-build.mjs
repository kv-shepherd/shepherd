import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const webRoot = path.resolve(scriptDir, "..");
const tempTsconfigPath = path.join(webRoot, "tsconfig.build.json");
const nextBin = path.join(
  webRoot,
  "node_modules",
  ".bin",
  process.platform === "win32" ? "next.cmd" : "next",
);

async function main() {
  const sourceTsconfigPath = path.join(webRoot, "tsconfig.json");
  const sourceTsconfig = await fs.readFile(sourceTsconfigPath, "utf8");

  await fs.writeFile(tempTsconfigPath, sourceTsconfig);

  const relativeTsconfigPath = path.relative(webRoot, tempTsconfigPath);

  const child = spawn(nextBin, ["build", "--webpack"], {
    cwd: webRoot,
    env: {
      ...process.env,
      NEXT_TSCONFIG_PATH: relativeTsconfigPath,
    },
    stdio: "inherit",
  });

  child.on("exit", async (code, signal) => {
    await fs.rm(tempTsconfigPath, { force: true });
    if (signal) {
      process.kill(process.pid, signal);
      return;
    }
    process.exit(code ?? 1);
  });
}

main().catch(async (error) => {
  await fs.rm(tempTsconfigPath, { force: true }).catch(() => undefined);
  console.error(error);
  process.exit(1);
});
