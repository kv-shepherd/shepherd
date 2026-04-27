import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";
import fs from "node:fs/promises";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const webRoot = path.resolve(scriptDir, "..");
const logDir = path.join(webRoot, "..", "tmp", "vitest-local-shards");
const vitestBin = path.join(
  webRoot,
  "node_modules",
  ".bin",
  process.platform === "win32" ? "vitest.cmd" : "vitest",
);

function parsePositiveInt(value) {
  if (!value) {
    return undefined;
  }
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function defaultShardCount() {
  if (process.env.CI) {
    return 1;
  }
  const cpus = os.availableParallelism?.() ?? os.cpus().length;
  return cpus >= 8 ? 2 : 1;
}

function formatDuration(milliseconds) {
  const seconds = Math.round(milliseconds / 1000);
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m${String(seconds % 60).padStart(2, "0")}s`;
}

function runShard(index, total) {
  const startedAt = Date.now();
  const logPath = path.join(logDir, `shard-${index}-of-${total}.log`);
  const args = ["run", `--shard=${index}/${total}`];

  return new Promise((resolve) => {
    const child = spawn(vitestBin, args, {
      cwd: webRoot,
      env: process.env,
      stdio: ["ignore", "pipe", "pipe"],
    });

    let output = "";
    child.stdout.on("data", (chunk) => {
      output += chunk;
    });
    child.stderr.on("data", (chunk) => {
      output += chunk;
    });
    child.on("error", async (error) => {
      output += `${error.stack ?? error.message}\n`;
      await fs.writeFile(logPath, output);
      resolve({
        code: 1,
        duration: formatDuration(Date.now() - startedAt),
        index,
        logPath,
        signal: undefined,
      });
    });
    child.on("close", async (code, signal) => {
      await fs.writeFile(logPath, output);
      resolve({
        code: signal ? 1 : code ?? 1,
        duration: formatDuration(Date.now() - startedAt),
        index,
        logPath,
        signal,
      });
    });
  });
}

async function runSingleProcess() {
  const child = spawn(vitestBin, ["run"], {
    cwd: webRoot,
    env: process.env,
    stdio: "inherit",
  });
  return new Promise((resolve, reject) => {
    child.on("exit", (code, signal) => {
      if (signal) {
        process.kill(process.pid, signal);
        return;
      }
      if (code === 0) {
        resolve(undefined);
        return;
      }
      reject(new Error(`vitest run failed with exit code ${code ?? 1}`));
    });
  });
}

async function main() {
  const configuredShards = parsePositiveInt(process.env.FRONTEND_LOCAL_TEST_SHARDS);
  const shardCount = configuredShards ?? defaultShardCount();

  if (shardCount <= 1) {
    await runSingleProcess();
    return;
  }

  await fs.rm(logDir, { recursive: true, force: true });
  await fs.mkdir(logDir, { recursive: true });

  console.log(`Running Vitest in ${shardCount} local shards.`);
  const results = await Promise.all(
    Array.from({ length: shardCount }, (_, offset) => runShard(offset + 1, shardCount)),
  );

  let failed = false;
  for (const result of results) {
    const label = `Vitest shard ${result.index}/${shardCount}`;
    if (result.code === 0) {
      console.log(`${label} passed in ${result.duration}.`);
      continue;
    }
    failed = true;
    console.error(`${label} failed in ${result.duration}. Recent log output:`);
    const log = await fs.readFile(result.logPath, "utf8").catch(() => "");
    console.error(log.split("\n").slice(-120).join("\n"));
  }

  if (failed) {
    throw new Error(`one or more Vitest shards failed; logs are under ${logDir}`);
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
