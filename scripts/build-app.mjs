import { spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import {
  accessSync,
  chmodSync,
  constants,
  copyFileSync,
  cpSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  renameSync,
  rmSync,
  statSync,
  utimesSync,
  writeFileSync,
} from "node:fs";
import { homedir, tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const appName = "Cooool Mark";
const binName = "Cooool Mark";
const bundleId = "app.cooool.mark";
const installPath = `/Applications/${appName}.app`;
const lsregisterPath =
  "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister";
const lockPath = join(root, "dist/.build.lock");

function argValue(flag, argv = process.argv) {
  const eq = argv.find((a) => a.startsWith(`${flag}=`));
  if (eq) return eq.slice(flag.length + 1);
  const idx = argv.indexOf(flag);
  if (idx >= 0 && idx + 1 < argv.length) return argv[idx + 1];
  return null;
}

function normalizePlatform(raw = process.platform) {
  const v = String(raw || "").toLowerCase();
  if (v === "darwin" || v === "macos" || v === "mac") return "darwin";
  if (v === "win32" || v === "windows" || v === "win") return "windows";
  if (v === "linux") return "linux";
  throw new Error(`unsupported platform: ${raw}`);
}

function normalizeArch(raw = process.arch) {
  const v = String(raw || "").toLowerCase();
  if (v === "arm64" || v === "aarch64") return "arm64";
  if (v === "x64" || v === "x86_64" || v === "amd64") return "amd64";
  throw new Error(`unsupported arch: ${raw}`);
}

function readAppVersion() {
  for (const file of ["mygo.json", "package.json"]) {
    try {
      const data = JSON.parse(readFileSync(join(root, file), "utf8"));
      if (typeof data.version === "string" && /^\d+\.\d+\.\d+/.test(data.version)) {
        return data.version;
      }
    } catch {}
  }
  return "0.1.0";
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { encoding: "utf8", maxBuffer: 64 * 1024 * 1024, ...options });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const detail = `${result.stderr ?? ""}${result.stdout ?? ""}`.trim();
    throw new Error(detail || `${command} exited ${result.status}`);
  }
  return result.stdout ?? "";
}

function pathExists(p) {
  try {
    lstatSync(p);
    return true;
  } catch (error) {
    if (error?.code === "ENOENT") return false;
    throw error;
  }
}

function remove(p) {
  rmSync(p, { recursive: true, force: true });
}

function markNeverIndex(dir) {
  try {
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, ".metadata_never_index"), "");
  } catch {}
}

function isProcessAlive(pid) {
  if (!Number.isInteger(pid) || pid <= 0) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return error?.code === "EPERM";
  }
}

async function acquireLock() {
  markNeverIndex(join(root, "dist"));
  const deadline = Date.now() + 90_000;
  while (true) {
    try {
      writeFileSync(lockPath, `${process.pid}\n`, { flag: "wx" });
      return () => remove(lockPath);
    } catch (error) {
      if (error?.code !== "EEXIST") throw error;
      let holder = 0;
      try {
        holder = Number.parseInt(readFileSync(lockPath, "utf8").trim(), 10);
      } catch {}
      if (!isProcessAlive(holder)) {
        remove(lockPath);
        continue;
      }
      if (Date.now() >= deadline) {
        throw new Error(`等待其他并行构建锁 (${lockPath}, PID ${holder}) 超时`);
      }
      await new Promise((r) => setTimeout(r, 250));
    }
  }
}

function workspaceTemp() {
  const prefix = `cooool-mark.${process.pid}.`;
  let dir;
  try {
    dir = mkdtempSync(join(tmpdir(), prefix));
  } catch {
    dir = mkdtempSync(join("/tmp", prefix));
  }
  markNeverIndex(dir);
  return dir;
}

function sameVolumeTemp(target) {
  const dir = dirname(target);
  mkdirSync(dir, { recursive: true });
  const staging = join(dir, `.cooool-mark-install.${process.pid}.${randomBytes(4).toString("hex")}`);
  mkdirSync(staging);
  markNeverIndex(staging);
  return staging;
}

function goBuild({ goos, goarch, outFile, profile, tempDir }) {
  mkdirSync(dirname(outFile), { recursive: true });
  const env = { ...process.env, CGO_ENABLED: "0", GOOS: goos, GOARCH: goarch };
  if (tempDir) env.TMPDIR = tempDir;

  const version = readAppVersion();
  const ldflags = [
    `-X cooool-mark/internal/app.BuildVersion=${version}`,
    `-X 'github.com/egoist/mygo.packageName=${appName}'`,
    `-X github.com/egoist/mygo.packageVersion=${version}`,
    `-X github.com/egoist/mygo.packageIdentifier=${bundleId}`,
    `-X github.com/egoist/mygo.packageURLSchemes=coooolmark`,
    `-X github.com/egoist/mygo.packageFileExtensions=html,htm,json`,
  ];
  if (profile === "release" || goos !== "darwin") ldflags.push("-s", "-w");
  if (goos === "windows") ldflags.push("-H=windowsgui");

  const args = ["build"];
  if (ldflags.length) args.push(`-ldflags=${ldflags.join(" ")}`);
  args.push("-o", outFile, ".");

  run("go", args, { cwd: root, env });
  if (!existsSync(outFile)) throw new Error(`go build did not produce ${outFile}`);
}

function ensureIcns(tempDir) {
  const srcIcon = join(root, "resources", "icon.png");
  const cachedIcns = join(root, ".mygo", "AppIcon.icns");
  if (!existsSync(srcIcon)) return null;

  const srcMtime = statSync(srcIcon).mtimeMs;
  if (existsSync(cachedIcns) && statSync(cachedIcns).mtimeMs >= srcMtime) {
    return cachedIcns;
  }

  const iconset = join(tempDir, "AppIcon.iconset");
  mkdirSync(iconset, { recursive: true });
  const sizes = [
    [16, "icon_16x16.png"],
    [32, "icon_16x16@2x.png"],
    [32, "icon_32x32.png"],
    [64, "icon_32x32@2x.png"],
    [128, "icon_128x128.png"],
    [256, "icon_128x128@2x.png"],
    [256, "icon_256x256.png"],
    [512, "icon_256x256@2x.png"],
    [512, "icon_512x512.png"],
    [1024, "icon_512x512@2x.png"],
  ];
  for (const [px, name] of sizes) {
    run("sips", ["-z", String(px), String(px), srcIcon, "--out", join(iconset, name)]);
  }
  mkdirSync(dirname(cachedIcns), { recursive: true });
  run("iconutil", ["-c", "icns", iconset, "-o", cachedIcns]);
  return cachedIcns;
}

function plistXml(version) {
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDisplayName</key>
  <string>${appName}</string>
  <key>CFBundleExecutable</key>
  <string>${binName}</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon.icns</string>
  <key>CFBundleIdentifier</key>
  <string>${bundleId}</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleLocalizations</key>
  <array>
    <string>en</string>
    <string>zh-Hans</string>
  </array>
  <key>CFBundleName</key>
  <string>${appName}</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>${version}</string>
  <key>CFBundleURLTypes</key>
  <array>
    <dict>
      <key>CFBundleURLName</key>
      <string>${bundleId}.url</string>
      <key>CFBundleURLSchemes</key>
      <array>
        <string>coooolmark</string>
      </array>
    </dict>
  </array>
  <key>CFBundleDocumentTypes</key>
  <array>
    <dict>
      <key>CFBundleTypeName</key>
      <string>HTML Bookmarks</string>
      <key>CFBundleTypeRole</key>
      <string>Viewer</string>
      <key>LSHandlerRank</key>
      <string>Alternate</string>
      <key>CFBundleTypeExtensions</key>
      <array>
        <string>html</string>
        <string>htm</string>
      </array>
    </dict>
    <dict>
      <key>CFBundleTypeName</key>
      <string>Cooool Mark Backup</string>
      <key>CFBundleTypeRole</key>
      <string>Viewer</string>
      <key>LSHandlerRank</key>
      <string>Alternate</string>
      <key>CFBundleTypeExtensions</key>
      <array>
        <string>json</string>
      </array>
    </dict>
  </array>
  <key>CFBundleVersion</key>
  <string>${version}</string>
  <key>LSMinimumSystemVersion</key>
  <string>12.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>NSPrincipalClass</key>
  <string>NSApplication</string>
  <key>NSSupportsAutomaticGraphicsSwitching</key>
  <true/>
</dict>
</plist>
`;
}

function assembleMacBundle(bundle, executable, icnsPath) {
  const macos = join(bundle, "Contents/MacOS");
  const resources = join(bundle, "Contents/Resources");
  mkdirSync(macos, { recursive: true });
  mkdirSync(resources, { recursive: true });

  writeFileSync(join(bundle, "Contents/Info.plist"), plistXml(readAppVersion()));
  if (icnsPath && existsSync(icnsPath)) {
    copyFileSync(icnsPath, join(resources, "AppIcon.icns"));
  }
  const resSrc = join(root, "resources");
  if (existsSync(resSrc)) {
    for (const ent of readdirSync(resSrc, { withFileTypes: true })) {
      if (ent.isFile() && !ent.name.startsWith(".")) {
        copyFileSync(join(resSrc, ent.name), join(resources, ent.name));
      }
    }
  }
  const packaged = join(macos, binName);
  copyFileSync(executable, packaged);
  chmodSync(packaged, 0o755);
}

function signBundle(bundle, { development = false } = {}) {
  run("plutil", ["-lint", join(bundle, "Contents/Info.plist")]);
  const reqs = development
    ? ["--identifier", bundleId, "--requirements", `=designated => identifier "${bundleId}"`]
    : [];
  run("codesign", ["--force", "--sign", "-", ...reqs, bundle]);
  run("codesign", ["--verify", "--verbose=2", bundle]);
}

function unregisterMacBundle(bundlePath) {
  if (process.platform !== "darwin" || !bundlePath) return;
  spawnSync(lsregisterPath, ["-u", bundlePath], { stdio: "ignore" });
}

function pruneLaunchServices(canonicalPath) {
  if (process.platform !== "darwin") return;
  const canonical = resolve(canonicalPath);
  const res = spawnSync(lsregisterPath, ["-dump"], {
    encoding: "utf8",
    maxBuffer: 128 * 1024 * 1024,
  });
  if (res.status === 0 && typeof res.stdout === "string") {
    const stale = new Set();
    let curPath = "";
    let curId = "";
    const flush = () => {
      if (curPath && curId === bundleId && resolve(curPath) !== canonical) {
        stale.add(curPath);
      }
      curPath = "";
      curId = "";
    };
    for (const rawLine of res.stdout.split("\n")) {
      const line = rawLine.replace(/\r$/, "");
      if (line.startsWith("--------------------------------------------------------------------------------")) {
        flush();
        continue;
      }
      if (!curPath && line.startsWith("path:")) {
        curPath = line.slice(5).replace(/\s+\(0x[0-9a-fA-F]+\)\s*$/, "").trim();
      } else if (!curId && line.startsWith("identifier:")) {
        curId = line.slice(11).replace(/\s+\(0x[0-9a-fA-F]+\)\s*$/, "").trim();
      }
    }
    flush();
    for (const p of stale) unregisterMacBundle(p);
  }
  if (existsSync(canonical)) {
    spawnSync(lsregisterPath, ["-f", canonical], { stdio: "ignore" });
  }
}

function pruneLocalBundles(dirName) {
  const base = join(root, dirName);
  if (!existsSync(base)) return;
  markNeverIndex(base);
  for (const ent of readdirSync(base, { withFileTypes: true })) {
    const full = join(base, ent.name);
    if (ent.isDirectory() && ent.name.endsWith(".app")) {
      unregisterMacBundle(full);
      remove(full);
    } else if (ent.isDirectory() && ent.name.startsWith("darwin-")) {
      try {
        for (const sub of readdirSync(full, { withFileTypes: true })) {
          if (sub.isDirectory() && sub.name.endsWith(".app")) {
            const subPath = join(full, sub.name);
            unregisterMacBundle(subPath);
            remove(subPath);
          }
        }
      } catch {}
    }
  }
}

function assertInstallTarget(target) {
  let stat;
  try {
    stat = lstatSync(target);
  } catch (error) {
    if (error?.code === "ENOENT") return false;
    throw error;
  }
  if (stat.isSymbolicLink() || !stat.isDirectory()) {
    throw new Error(`${target} is not an app bundle directory`);
  }
  const info = join(target, "Contents/Info.plist");
  const id = run("plutil", ["-extract", "CFBundleIdentifier", "raw", "-o", "-", info]).trim();
  if (id !== bundleId) throw new Error(`${target} is ${id}, not ${bundleId}`);
  return true;
}

function commitMacInstall(bundle) {
  const present = assertInstallTarget(installPath);
  accessSync(dirname(installPath), constants.W_OK);

  const staging = sameVolumeTemp(installPath);
  const staged = join(staging, "staged-bundle");
  const backup = join(staging, "previous-bundle");
  try {
    cpSync(bundle, staged, { recursive: true });
    if (present) renameSync(installPath, backup);
    try {
      renameSync(staged, installPath);
    } catch (error) {
      if (present && pathExists(backup)) renameSync(backup, installPath);
      throw error;
    }
    try {
      run("codesign", ["--verify", "--verbose=2", installPath]);
    } catch (error) {
      if (present && pathExists(backup)) {
        if (pathExists(installPath)) renameSync(installPath, staged);
        renameSync(backup, installPath);
      } else {
        remove(installPath);
      }
      throw error;
    }
    if (present) remove(backup);
  } finally {
    remove(staging);
  }

  try {
    const now = new Date();
    utimesSync(installPath, now, now);
  } catch {}
  pruneLocalBundles("dist");
  pruneLocalBundles("build");
  pruneLaunchServices(installPath);
  return installPath;
}

function publishDir(srcDir, finalDir) {
  mkdirSync(dirname(finalDir), { recursive: true });
  const staged = join(dirname(finalDir), `.${appName}.staging-${process.pid}`);
  const previous = join(dirname(finalDir), `.${appName}.previous-${process.pid}`);
  remove(staged);
  cpSync(srcDir, staged, { recursive: true });
  let aside = false;
  try {
    if (pathExists(finalDir)) {
      renameSync(finalDir, previous);
      aside = true;
    }
    renameSync(staged, finalDir);
  } catch (error) {
    if (aside && !pathExists(finalDir)) renameSync(previous, finalDir);
    remove(staged);
    throw error;
  }
  if (aside) remove(previous);
  return finalDir;
}

function publishFile(src, dest, mode = 0o755) {
  mkdirSync(dirname(dest), { recursive: true });
  const staged = join(dirname(dest), `.cooool-mark.staging-${process.pid}`);
  copyFileSync(src, staged);
  chmodSync(staged, mode);
  renameSync(staged, dest);
  return dest;
}

async function main() {
  const started = Date.now();
  const platform = normalizePlatform(argValue("--platform") || process.platform);
  const arch = normalizeArch(argValue("--arch") || process.arch);
  const hostPlatform = normalizePlatform(process.platform);
  const hostArch = normalizeArch(process.arch);
  const noInstall = process.argv.includes("--no-install") || process.env.COOOOL_NO_INSTALL === "1";
  const install = !noInstall && platform === hostPlatform && arch === hostArch;
  const profile = argValue("--profile") || (install ? "fast" : "release");

  const release = await acquireLock();
  const staging = workspaceTemp();
  try {
    if (platform === "darwin") {
      if (hostPlatform !== "darwin") throw new Error("macOS app bundling requires macOS");
      const stageBin = join(staging, binName);
      goBuild({ goos: "darwin", goarch: arch, outFile: stageBin, profile, tempDir: staging });
      publishFile(stageBin, join(root, "dist", "cooool-mark"), 0o755);

      const icnsPath = ensureIcns(staging);
      const bundle = join(staging, `${appName}.app`);
      assembleMacBundle(bundle, stageBin, icnsPath);
      signBundle(bundle, { development: profile !== "release" });

      if (install) {
        const installed = commitMacInstall(bundle);
        console.log(`${installed} (${((Date.now() - started) / 1000).toFixed(2)}s)`);
        return;
      }

      const distArchApp = join(root, "dist", `darwin-${arch}`, `${appName}.app`);
      const distRootApp = join(root, "dist", `${appName}.app`);
      markNeverIndex(join(root, "dist"));
      publishDir(bundle, distArchApp);
      publishDir(bundle, distRootApp);
      if (pathExists(installPath)) {
        unregisterMacBundle(distArchApp);
        unregisterMacBundle(distRootApp);
      }
      console.log(`${distRootApp} (${((Date.now() - started) / 1000).toFixed(2)}s)`);
      return;
    }

    if (platform === "windows") {
      const outExe = join(root, "dist", `windows-${arch}`, "cooool-mark.exe");
      goBuild({ goos: "windows", goarch: arch, outFile: outExe, profile, tempDir: staging });
      console.log(`${outExe} (${((Date.now() - started) / 1000).toFixed(2)}s)`);
      return;
    }

    const outBin = join(root, "dist", `linux-${arch}`, "cooool-mark");
    goBuild({ goos: "linux", goarch: arch, outFile: outBin, profile, tempDir: staging });
    chmodSync(outBin, 0o755);
    console.log(`${outBin} (${((Date.now() - started) / 1000).toFixed(2)}s)`);
  } finally {
    unregisterMacBundle(join(staging, `${appName}.app`));
    remove(staging);
    release();
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exitCode = 1;
});
