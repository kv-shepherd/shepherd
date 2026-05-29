#!/usr/bin/env node

import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const repoRoot = process.cwd();
const args = process.argv.slice(2);
let selfTest = false;
let showHelp = false;
const roots = [];

for (const arg of args) {
  if (arg === "--self-test") {
    selfTest = true;
  } else if (arg === "-h" || arg === "--help") {
    showHelp = true;
  } else {
    roots.push(arg);
  }
}

if (showHelp) {
  process.stdout.write(`Usage:
  node docs/design/ci/scripts/find_latest_live_e2e_full_evidence.mjs [root]
  node docs/design/ci/scripts/find_latest_live_e2e_full_evidence.mjs --self-test

Find the newest live E2E evidence manifest under root where mode=full.
The default root is .run/live-e2e.
`);
  process.exit(0);
}

if (selfTest) {
  runSelfTest();
  process.exit(0);
}

if (roots.length > 1) {
  fail("accepts at most one root path");
}

const root = path.resolve(repoRoot, roots[0] ?? ".run/live-e2e");
const selected = findLatestFullEvidence(root);
if (!selected) {
  fail(`no full live E2E evidence manifest found under ${displayPath(root)}`);
}

process.stdout.write(`${displayPath(selected.path)}\n`);

function findLatestFullEvidence(root) {
  const manifests = [];

  for (const manifestPath of collectManifestPaths(root)) {
    const manifest = readManifest(manifestPath);
    if (!manifest || manifest.mode !== "full") {
      continue;
    }

    const stats = fs.statSync(manifestPath);
    manifests.push({
      path: manifestPath,
      timestampMs: timestampForManifest(manifest, stats),
    });
  }

  manifests.sort((a, b) => {
    if (a.timestampMs !== b.timestampMs) {
      return a.timestampMs - b.timestampMs;
    }
    return a.path.localeCompare(b.path);
  });

  return manifests.at(-1) ?? null;
}

function collectManifestPaths(root) {
  const results = [];
  if (!fs.existsSync(root)) {
    return results;
  }

  scanDirectory(root, results);
  results.sort();
  return results;
}

function scanDirectory(dir, results) {
  let entries;
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch (error) {
    warn(`skipping unreadable directory ${displayPath(dir)}: ${error.message}`);
    return;
  }

  for (const entry of entries) {
    const entryPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      scanDirectory(entryPath, results);
      continue;
    }
    if (entry.isFile() && entry.name === "live-e2e.evidence.json") {
      results.push(entryPath);
    }
  }
}

function readManifest(manifestPath) {
  try {
    const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
    if (!manifest || typeof manifest !== "object" || Array.isArray(manifest)) {
      warn(`skipping non-object manifest ${displayPath(manifestPath)}`);
      return null;
    }
    return manifest;
  } catch (error) {
    warn(`skipping invalid manifest ${displayPath(manifestPath)}: ${error.message}`);
    return null;
  }
}

function timestampForManifest(manifest, stats) {
  if (typeof manifest.generated_at === "string") {
    const generatedAtMs = Date.parse(manifest.generated_at);
    if (Number.isFinite(generatedAtMs)) {
      return generatedAtMs;
    }
  }
  return stats.mtimeMs;
}

function displayPath(filePath) {
  const relative = path.relative(repoRoot, filePath);
  if (!relative || relative.startsWith("..") || path.isAbsolute(relative)) {
    return filePath;
  }
  return relative.replaceAll(path.sep, "/");
}

function runSelfTest() {
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "live-e2e-latest-"));
  try {
    writeManifest(path.join(tempRoot, "20260528", "0100-full-old"), {
      generated_at: "2026-05-28T01:00:00Z",
      mode: "full",
    });
    const expected = writeManifest(path.join(tempRoot, "20260528", "0200-full-new"), {
      generated_at: "2026-05-28T02:00:00Z",
      mode: "full",
    });
    writeManifest(path.join(tempRoot, "preflight", "9999-preflight"), {
      generated_at: "2099-01-01T00:00:00Z",
      mode: "preflight",
    });

    const selected = findLatestFullEvidence(tempRoot);
    if (!selected || selected.path !== expected) {
      throw new Error(`expected ${expected}, got ${selected?.path ?? "<none>"}`);
    }

    const onlyPreflightRoot = path.join(tempRoot, "only-preflight");
    writeManifest(path.join(onlyPreflightRoot, "preflight"), {
      generated_at: "2026-05-28T03:00:00Z",
      mode: "preflight",
    });
    if (findLatestFullEvidence(onlyPreflightRoot) !== null) {
      throw new Error("preflight-only root should not return a manifest");
    }

    process.stdout.write("OK: latest live E2E full evidence selector self-test passed\n");
  } finally {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

function writeManifest(dir, manifest) {
  const manifestPath = path.join(dir, "live-e2e.evidence.json");
  writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  return manifestPath;
}

function writeFile(filePath, content) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, content);
}

function warn(message) {
  console.error(`[live-e2e-latest-evidence] WARN: ${message}`);
}

function fail(message) {
  console.error(`[live-e2e-latest-evidence] ERROR: ${message}`);
  process.exit(1);
}
