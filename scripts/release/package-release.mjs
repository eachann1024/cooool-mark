import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { cpSync, existsSync, mkdirSync, mkdtempSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { installerFor } from "./allowed-assets.mjs";
import { run } from "./go-release.mjs";
import { applyReleasePlanVersion, ROOT } from "./select-release.mjs";

const APP_NAME = "Cooool Mark";
const RELEASE_DIR = resolve(ROOT, "dist/release");

function normalizeArch(raw) {
  const v = String(raw || "").toLowerCase();
  if (v === "arm64" || v === "aarch64") return "arm64";
  if (v === "x64" || v === "x86_64" || v === "amd64") return "amd64";
  throw new Error(`unsupported arch ${raw}`);
}

function cloneDir(src, dest) {
  if (process.platform === "darwin") {
    const res = spawnSync("cp", ["-cR", src, dest], { stdio: "ignore" });
    if (res.status === 0) return;
  }
  cpSync(src, dest, { recursive: true });
}

function resolveTargetArches() {
  const list = process.env.COOOOL_ARCHES;
  if (list && list.trim()) {
    const arches = list.split(",").map((s) => s.trim()).filter(Boolean);
    if (arches.length > 0) return arches;
  }
  return [process.env.COOOOL_ARCH || process.arch];
}

function runAsync(cmd, args) {
  return new Promise((resolvePromise, rejectPromise) => {
    const child = spawn(cmd, args, { stdio: "inherit", cwd: ROOT });
    child.on("error", rejectPromise);
    child.on("close", (code) => {
      if (code === 0) resolvePromise();
      else rejectPromise(new Error(`${cmd} ${args.join(" ")} exited ${code}`));
    });
  });
}

async function createDmgWithRetry(stageDir, outDmg) {
  let lastError;
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    try {
      rmSync(outDmg, { force: true });
      await runAsync("hdiutil", [
        "create",
        "-volname",
        APP_NAME,
        "-srcfolder",
        stageDir,
        "-ov",
        "-format",
        "UDZO",
        outDmg,
      ]);
      return;
    } catch (err) {
      lastError = err;
      if (attempt < 3) await new Promise((r) => setTimeout(r, 1000 * attempt));
    }
  }
  throw lastError;
}

export async function packageRelease(arches = resolveTargetArches()) {
  const version = applyReleasePlanVersion();
  mkdirSync(RELEASE_DIR, { recursive: true });

  const dmgTasks = [];
  for (const rawArch of arches) {
    const goarch = normalizeArch(rawArch);
    const dmgName = installerFor({ platform: "macos", arch: rawArch, kind: "dmg" });
    run("node", [
      "scripts/build-app.mjs",
      "--no-install",
      "--profile",
      "release",
      "--platform",
      "darwin",
      "--arch",
      goarch,
    ]);
    const appBundle = resolve(ROOT, "dist", `darwin-${goarch}`, `${APP_NAME}.app`);
    assert.ok(existsSync(appBundle), `App bundle missing at ${appBundle}`);

    const stageDir = mkdtempSync(join(tmpdir(), "cooool-mark-dmg-"));
    const stagedApp = join(stageDir, `${APP_NAME}.app`);
    try {
      cloneDir(appBundle, stagedApp);
      run("codesign", ["--verify", "--verbose=2", stagedApp]);
      symlinkSync("/Applications", join(stageDir, "Applications"));
    } catch (err) {
      rmSync(stageDir, { recursive: true, force: true });
      throw err;
    }

    const dmgPath = join(RELEASE_DIR, dmgName);
    dmgTasks.push(
      (async () => {
        try {
          await createDmgWithRetry(stageDir, dmgPath);
          assert.ok(existsSync(dmgPath), `Failed to create ${dmgName}`);
          return dmgPath;
        } finally {
          rmSync(stageDir, { recursive: true, force: true });
        }
      })(),
    );
  }

  const dmgPaths = await Promise.all(dmgTasks);
  console.log(`Packaged Cooool Mark v${version}:`);
  for (const dmgPath of dmgPaths) console.log(`  ${dmgPath}`);
}

const isMain =
  import.meta.main ??
  (Boolean(process.argv[1]) && resolve(process.argv[1]) === fileURLToPath(import.meta.url));

if (isMain) {
  await packageRelease();
}
