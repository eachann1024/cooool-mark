import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { basename, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { installerFor } from "./allowed-assets.mjs";
import { canReuseReleaseBinary, goRelease, releaseBinaryPath, run } from "./go-release.mjs";
import { applyReleasePlanVersion, readPackageVersion, ROOT } from "./select-release.mjs";

const ARCH = process.env.COOOOL_ARCH || (process.arch === "arm64" ? "arm64" : "x64");
if (ARCH !== "x64" && ARCH !== "amd64") {
  throw new Error(`Linux packages are built for x64 only, got ${ARCH}`);
}
const APPIMAGE_NAME = installerFor({ platform: "linux", arch: "x64", kind: "AppImage" });
const SQUASH_ARCH = process.env.APPIMAGE_ARCH || "x86_64";
const RELEASE_DIR = resolve(ROOT, "dist/release");
const HOST_LIB = /^(ld-linux|linux-vdso\.|libc\.|libm\.|libdl\.|libpthread\.|librt\.|libresolv\.|libutil\.|libvulkan\.|libGL\.|libGLX\.|libGLdispatch\.|libEGL\.|libOpenGL\.|librenderdoc\.)/;

function linkedLibraries(binary) {
  const listed = spawnSync("ldd", [binary], { encoding: "utf8" });
  const combined = `${listed.stdout ?? ""}\n${listed.stderr ?? ""}`;
  if (listed.status !== 0 || /not a dynamic executable|statically linked/i.test(combined)) {
    return { dynamic: false, paths: [] };
  }
  const paths = [];
  for (const line of (listed.stdout ?? "").split("\n")) {
    const mapped = line.match(/=>\s+(\/\S+)/);
    const absolute = line.match(/^\s*(\/\S+)\s+\(/);
    const p = mapped?.[1] || absolute?.[1];
    if (!p || HOST_LIB.test(basename(p))) continue;
    paths.push(p);
  }
  return { dynamic: true, paths };
}

function soname(path) {
  const printed = spawnSync("patchelf", ["--print-soname", path], { encoding: "utf8" });
  const name = printed.stdout?.trim();
  return printed.status === 0 && name ? name : basename(path);
}

function resolveSoname(name) {
  const listed = spawnSync("ldconfig", ["-p"], { encoding: "utf8" });
  if (listed.status !== 0) return null;
  const line = listed.stdout.split("\n").find((row) => row.trimStart().startsWith(`${name} `));
  return line?.match(/=>\s+(\S+)/)?.[1] ?? null;
}

function dlopenLibraries(binary) {
  const dumped = spawnSync("strings", ["-a", "-n", "8", binary], {
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
  });
  if (dumped.status !== 0) return [];
  const paths = [];
  for (const match of dumped.stdout.matchAll(/lib[A-Za-z0-9_+-]+\.so(?:\.\d+)*/g)) {
    const name = match[0];
    if (HOST_LIB.test(name)) continue;
    const p = resolveSoname(name);
    if (p) paths.push(p);
  }
  return paths;
}

function collectLibraries(binary) {
  const linked = linkedLibraries(binary);
  const seen = new Map();
  const queue = [...linked.paths, ...dlopenLibraries(binary)];
  while (queue.length > 0) {
    const p = queue.pop();
    if (seen.has(p)) continue;
    seen.set(p, soname(p));
    for (const next of linkedLibraries(p).paths) {
      if (!seen.has(next)) queue.push(next);
    }
  }
  return { dynamic: linked.dynamic, libs: seen };
}

function writeAppDir(binary, appDir) {
  rmSync(appDir, { recursive: true, force: true });
  const binDir = join(appDir, "usr/bin");
  const libDir = join(appDir, "usr/lib");
  mkdirSync(binDir, { recursive: true });
  mkdirSync(libDir, { recursive: true });
  const staged = join(binDir, "cooool-mark");
  copyFileSync(binary, staged);
  chmodSync(staged, 0o755);

  const { dynamic, libs } = collectLibraries(binary);
  for (const [p, name] of libs) copyFileSync(p, join(libDir, name));
  if (dynamic) run("patchelf", ["--set-rpath", "$ORIGIN/../lib", staged]);
  for (const name of libs.values()) run("patchelf", ["--set-rpath", "$ORIGIN", join(libDir, name)]);

  copyFileSync(resolve(ROOT, "resources/icon.png"), join(appDir, "cooool-mark.png"));
  writeFileSync(join(appDir, "cooool-mark.desktop"), [
    "[Desktop Entry]",
    "Type=Application",
    "Name=Cooool Mark",
    "Exec=cooool-mark %U",
    "Icon=cooool-mark",
    "Categories=Utility;",
    "MimeType=x-scheme-handler/coooolmark;text/html;application/json;",
    "Terminal=false",
    "",
  ].join("\n"));
  writeFileSync(join(appDir, "AppRun"), [
    "#!/bin/sh",
    'HERE=$(dirname "$(readlink -f "$0")")',
    'export LD_LIBRARY_PATH="$HERE/usr/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"',
    'exec "$HERE/usr/bin/cooool-mark" "$@"',
    "",
  ].join("\n"));
  chmodSync(join(appDir, "AppRun"), 0o755);
}

export async function packageLinux() {
  assert.equal(process.platform, "linux", "Linux is required to build the AppImage");
  assert.equal(process.arch, "x64", `Linux packages are built for x64 only, got ${process.arch}`);
  const reuse = process.env.COOOOL_REUSE_BINARY === "1";
  const version = reuse ? readPackageVersion(ROOT) : applyReleasePlanVersion();
  let binary = releaseBinaryPath("linux", "amd64");
  if (!reuse || !canReuseReleaseBinary({ goos: "linux", goarch: "amd64", version, outFile: binary })) {
    binary = goRelease({ goos: "linux", goarch: "amd64", outFile: binary });
  }
  assert.ok(existsSync(binary), `Release binary missing at ${binary}`);
  const appDir = resolve(ROOT, "dist/CoooolMark.AppDir");
  writeAppDir(binary, appDir);
  mkdirSync(RELEASE_DIR, { recursive: true });
  const outfile = join(RELEASE_DIR, APPIMAGE_NAME);
  rmSync(outfile, { force: true });
  const tool = process.env.APPIMAGETOOL || "appimagetool";
  run(tool, [appDir, outfile], {
    env: { ...process.env, ARCH: SQUASH_ARCH, APPIMAGE_EXTRACT_AND_RUN: "1" },
  });
  chmodSync(outfile, 0o755);
  rmSync(`${outfile}.zsync`, { force: true });
  assert.ok(existsSync(outfile), `Failed to create ${APPIMAGE_NAME}`);
  console.log(`Packaged Cooool Mark v${version}:`);
  console.log(`  ${outfile}`);
}

const isMain =
  import.meta.main ??
  (Boolean(process.argv[1]) && resolve(process.argv[1]) === fileURLToPath(import.meta.url));

if (isMain) {
  await packageLinux();
}
