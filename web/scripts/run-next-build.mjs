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

async function restoreFile(filepath, content) {
  await fs.writeFile(filepath, content);
}

function run(bin, args, extraEnv = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(bin, args, {
      cwd: webRoot,
      env: {
        ...process.env,
        ...extraEnv,
      },
      stdio: "inherit",
    });

    child.on("exit", (code, signal) => {
      if (signal) {
        process.kill(process.pid, signal);
        return;
      }
      if (code === 0) {
        resolve(undefined);
        return;
      }
      reject(new Error(`${path.basename(bin)} ${args.join(" ")} failed with exit code ${code ?? 1}`));
    });
  });
}

async function main() {
  const sourceTsconfigPath = path.join(webRoot, "tsconfig.json");
  const sourceTsconfig = await fs.readFile(sourceTsconfigPath, "utf8");

  await fs.writeFile(tempTsconfigPath, sourceTsconfig);

  const relativeTsconfigPath = path.relative(webRoot, tempTsconfigPath);
  try {
    await run(nextBin, ["build", "--webpack"], {
      NEXT_TSCONFIG_PATH: relativeTsconfigPath,
    });
  } finally {
    await restoreFile(sourceTsconfigPath, sourceTsconfig);
  }
}

main().catch(async (error) => {
  console.error(error);
  process.exitCode = 1;
}).finally(async () => {
  await fs.rm(tempTsconfigPath, { force: true }).catch(() => undefined);
});
