import { execFileSync } from "node:child_process";
import { appendFileSync, existsSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { buildReleaseNotes, EN_FOOTER, isExcludedCommit, listCommits, ZH_FOOTER } from "./release-notes.mjs";
import { resolveReleaseRange } from "./release-range.mjs";

export const ROOT = resolve(fileURLToPath(new URL("../..", import.meta.url)));
export const REPO = process.env.GH_REPO || process.env.GITHUB_REPOSITORY || "eachann1024/cooool-mark";
export const MAX_WINDOWS_PE_PART = 65535;

export function gh(args, input) {
  return execFileSync("gh", args, {
    cwd: ROOT,
    encoding: "utf8",
    input,
    maxBuffer: 20 * 1024 * 1024,
    stdio: ["pipe", "pipe", "pipe"],
  });
}

export function versionFromTag(tag) {
  return /^v?(\d+\.\d+\.\d+)(?:-[0-9a-f]{7,40})?$/.exec(tag || "")?.[1] || null;
}

export function compareVersions(left, right) {
  const a = left.split(".").map(Number);
  const b = right.split(".").map(Number);
  for (let i = 0; i < 3; i += 1) {
    if (a[i] !== b[i]) return a[i] - b[i];
  }
  return 0;
}

export function bumpPatch(version) {
  const [major, minor, patch] = version.split(".").map(Number);
  return `${major}.${minor}.${patch + 1}`;
}

export function assertValidWindowsPEVersion(version) {
  if (typeof version !== "string" || !/^\d+\.\d+\.\d+$/.test(version)) {
    throw new Error(`Invalid semantic version: ${version}`);
  }
  for (const part of version.split(".").map(Number)) {
    if (!Number.isInteger(part) || part < 0 || part > MAX_WINDOWS_PE_PART) {
      throw new Error(`Version ${version} exceeds Windows PE 16-bit limit (${MAX_WINDOWS_PE_PART})`);
    }
  }
  return version;
}

export function listGitTagVersions(root = ROOT) {
  try {
    const out = execFileSync("git", ["tag", "--list", "v*"], {
      cwd: root,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    });
    return out
      .split("\n")
      .map((tag) => versionFromTag(tag.trim()))
      .filter(Boolean);
  } catch {
    return [];
  }
}

export function nextReleaseVersion({ currentVersion, releases = [], tagVersions = [], headSha = "" }) {
  const publishedOnly = releases
    .filter((r) => r && !r.draft)
    .map((r) => versionFromTag(r.tag_name))
    .filter(Boolean);
  const highestPublished = publishedOnly.reduce(
    (best, item) => (!best || compareVersions(item, best) > 0 ? item : best),
    null,
  );
  if (headSha) {
    const matchingDrafts = releases.filter(
      (r) => r?.draft && r.target_commitish === headSha && versionFromTag(r.tag_name),
    );
    if (matchingDrafts.length === 1) {
      const draftVer = versionFromTag(matchingDrafts[0].tag_name);
      if (draftVer && (!highestPublished || compareVersions(draftVer, highestPublished) > 0)) {
        return assertValidWindowsPEVersion(draftVer);
      }
    }
  }

  const taken = [
    ...releases.map((r) => versionFromTag(r?.tag_name)),
    ...tagVersions.map((tag) => versionFromTag(tag)),
  ].filter(Boolean);
  const highestTaken = taken.reduce(
    (best, item) => (!best || compareVersions(item, best) > 0 ? item : best),
    null,
  );
  const normalizedCurrent = versionFromTag(currentVersion);
  let version;
  if (!highestTaken) {
    version = normalizedCurrent || "1.0.0";
  } else if (normalizedCurrent && compareVersions(normalizedCurrent, highestTaken) > 0) {
    version = normalizedCurrent;
  } else {
    version = bumpPatch(highestTaken);
  }
  const used = new Set(taken);
  while (used.has(version)) version = bumpPatch(version);
  return assertValidWindowsPEVersion(version);
}

export function readPackageVersion(root = ROOT) {
  for (const file of ["package.json", "mygo.json"]) {
    const p = resolve(root, file);
    if (existsSync(p)) {
      const parsed = JSON.parse(readFileSync(p, "utf8"));
      if (typeof parsed.version === "string" && /^\d+\.\d+\.\d+$/.test(parsed.version)) {
        return parsed.version;
      }
    }
  }
  throw new Error("package.json / mygo.json is missing a valid version");
}

export function setPackageJsonVersion(text, version) {
  const pkg = JSON.parse(text);
  pkg.version = version;
  return `${JSON.stringify(pkg, null, 2)}\n`;
}

export function setMyGoJsonVersion(text, version) {
  const mygo = JSON.parse(text);
  mygo.version = version;
  return `${JSON.stringify(mygo, null, 2)}\n`;
}

export function syncVersionFiles(root, version) {
  const pkgPath = resolve(root, "package.json");
  if (existsSync(pkgPath)) {
    writeFileSync(pkgPath, setPackageJsonVersion(readFileSync(pkgPath, "utf8"), version));
  }
  const mygoPath = resolve(root, "mygo.json");
  if (existsSync(mygoPath)) {
    writeFileSync(mygoPath, setMyGoJsonVersion(readFileSync(mygoPath, "utf8"), version));
  }
}

export function applyReleasePlanVersion() {
  const envVer = process.env.COOOOL_RELEASE_VERSION?.trim();
  if (envVer && /^\d+\.\d+\.\d+$/.test(envVer)) {
    syncVersionFiles(ROOT, envVer);
    return envVer;
  }
  const planPath = resolve(ROOT, "release-plan.json");
  let version = readPackageVersion(ROOT);
  if (existsSync(planPath)) {
    const plan = JSON.parse(readFileSync(planPath, "utf8"));
    if (typeof plan.version === "string" && plan.version) {
      version = plan.version;
      syncVersionFiles(ROOT, version);
    }
  }
  return version;
}

function sleepSync(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

export function fetchReleasePage(repo = REPO, page = 1) {
  let lastError;
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    try {
      const batch = JSON.parse(gh(["api", `repos/${repo}/releases?per_page=100&page=${page}`]));
      if (!Array.isArray(batch)) throw new Error(`Unexpected releases response on page ${page}`);
      return batch;
    } catch (err) {
      lastError = err;
      if (attempt < 3) sleepSync(1500 * attempt);
    }
  }
  throw lastError;
}

export function getReleases({ repo = REPO, includeBody = false } = {}) {
  const out = [];
  for (let page = 1; ; page += 1) {
    const batch = fetchReleasePage(repo, page);
    if (batch.length === 0) break;
    if (includeBody) {
      out.push(...batch);
    } else {
      for (const r of batch) {
        out.push({
          id: r?.id,
          tag_name: r?.tag_name,
          target_commitish: r?.target_commitish,
          draft: Boolean(r?.draft),
          prerelease: Boolean(r?.prerelease),
          published_at: r?.published_at,
          created_at: r?.created_at,
        });
      }
    }
    if (batch.length < 100) break;
  }
  return out;
}

function resolveTagCommit(release) {
  if (!release) return null;
  try {
    const sha = execFileSync("git", ["rev-parse", `${release.tag_name}^{commit}`], {
      cwd: ROOT,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
    if (/^[0-9a-f]{40}$/i.test(sha)) return sha;
  } catch {}
  try {
    const sha = gh(["api", `repos/${REPO}/commits/${release.tag_name}`, "--jq", ".sha"]).trim();
    if (/^[0-9a-f]{40}$/i.test(sha)) return sha;
  } catch {}
  if (/^[0-9a-f]{40}$/i.test(release.target_commitish || "")) return release.target_commitish;
  return null;
}

function summary(line) {
  console.log(line);
  if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${line}\n`);
}

function writeOutputs(entries) {
  if (!process.env.GITHUB_OUTPUT) return;
  const lines = Object.entries(entries).map(([k, v]) => `${k}=${v}`).join("\n") + "\n";
  appendFileSync(process.env.GITHUB_OUTPUT, lines);
}

function outputFlag(value) {
  return value ? "yes" : "no";
}

function forcedReleaseNotes() {
  return [
    "## English",
    "",
    "### Improvements",
    "",
    "- This build was published manually.",
    "",
    EN_FOOTER,
    "",
    "---",
    "",
    "## 中文",
    "",
    "### 改进",
    "",
    "- 这一版由手动发布。",
    "",
    ZH_FOOTER,
  ].join("\n");
}

function feedHasReleaseTag(tag) {
  if (!tag) return true;
  let raw = null;
  for (const ref of ["origin/main", "HEAD"]) {
    try {
      raw = execFileSync("git", ["show", `${ref}:updates/feed.json`], {
        cwd: ROOT,
        encoding: "utf8",
        stdio: ["ignore", "pipe", "ignore"],
      });
      break;
    } catch {}
  }
  if (raw === null) {
    try {
      const data = JSON.parse(gh(["api", `repos/${REPO}/contents/updates/feed.json?ref=main`]));
      raw = Buffer.from(String(data.content).replaceAll("\n", ""), "base64").toString("utf8");
    } catch (err) {
      if (/\b404\b|Not Found/i.test(String(err?.stderr || err?.message || ""))) return false;
      throw err;
    }
  }
  const parsed = JSON.parse(raw);
  if (parsed?.latest_tag !== tag) return false;
  const entry = Array.isArray(parsed?.releases) ? parsed.releases.find((r) => r?.tag === tag) : null;
  const assets = Array.isArray(entry?.assets) ? entry.assets : [];
  return assets.length > 0 && assets.every((a) => typeof a?.sha256 === "string" && /^[0-9a-f]{64}$/i.test(a.sha256));
}

function writeSkipPlan({ currentVersion, tag, headSha, extraOutputs = {} }) {
  const plan = { shouldBuild: false, publish: false, version: currentVersion, tag, commit: headSha };
  writeFileSync(resolve(ROOT, "release-plan.json"), `${JSON.stringify(plan, null, 2)}\n`);
  writeOutputs({
    should_build: outputFlag(false),
    publish: outputFlag(false),
    version: currentVersion,
    release_tag: tag,
    ...extraOutputs,
  });
}

async function main() {
  const headSha = process.env.SOURCE_SHA || process.env.GITHUB_SHA
    || execFileSync("git", ["rev-parse", "HEAD"], { cwd: ROOT, encoding: "utf8" }).trim();
  const force = process.env.FORCE === "true";
  const publishInput = process.env.PUBLISH !== "false";
  const onMain = !process.env.GITHUB_REF || process.env.GITHUB_REF === "refs/heads/main";

  const currentVersion = readPackageVersion(ROOT);
  const releases = getReleases();
  const tagVersions = listGitTagVersions(ROOT);
  const version = nextReleaseVersion({ currentVersion, releases, tagVersions, headSha });

  const stable = releases
    .filter((r) => !r.draft && !r.prerelease && versionFromTag(r.tag_name))
    .sort((a, b) => compareVersions(versionFromTag(a.tag_name), versionFromTag(b.tag_name)));
  const previous = stable.at(-1) || null;
  const previousMissingFeed = Boolean(previous && !feedHasReleaseTag(previous.tag_name));
  const previousSha = resolveTagCommit(previous);
  const canSyncPostPublish = Boolean(onMain && publishInput);

  if (previousSha && previousSha === headSha && !force) {
    if (previousMissingFeed && canSyncPostPublish) {
      summary(`最新 Release ${previous.tag_name} 尚未回写 updates/feed.json，触发同步补偿。`);
      writeSkipPlan({ currentVersion, tag: previous.tag_name, headSha, extraOutputs: { sync_only_tag: previous.tag_name } });
      return;
    }
    summary(`当前 HEAD (${headSha.slice(0, 7)}) 与最新 Release ${previous.tag_name} 一致，跳过构建。`);
    writeSkipPlan({ currentVersion, tag: previous.tag_name, headSha });
    return;
  }

  let rangeSpec = null;
  if (!(previousSha && previousSha === headSha)) {
    const resolved = resolveReleaseRange({ previous, previousSha, headSha });
    rangeSpec = resolved.range;
    if (resolved.note) summary(resolved.note);
  }

  const commits = rangeSpec ? listCommits(ROOT, rangeSpec) : [];
  const userCommits = commits.filter((c) => !isExcludedCommit(c));
  if (previous && userCommits.length === 0 && !force) {
    if (previousMissingFeed && canSyncPostPublish) {
      summary(`无面向用户的新提交，但 ${previous.tag_name} 尚未写入 updates/feed.json，触发同步补偿。`);
      writeSkipPlan({ currentVersion, tag: previous.tag_name, headSha, extraOutputs: { sync_only_tag: previous.tag_name } });
      return;
    }
    summary(`自 ${previous.tag_name} 以来没有面向用户的新提交（扫描 ${commits.length} 条），跳过自动发布。`);
    writeSkipPlan({ currentVersion, tag: previous.tag_name, headSha });
    return;
  }

  let notes = await buildReleaseNotes({ commits });
  if (!notes.markdown) {
    if (force || !previous) {
      notes = { markdown: forcedReleaseNotes(), source: "forced" };
    } else {
      summary(`未生成更新说明（${notes.reason || "空"}），跳过自动发布。`);
      writeSkipPlan({ currentVersion, tag: previous.tag_name, headSha });
      return;
    }
  }

  const tag = `v${version}`;
  const autoBump = version !== currentVersion;
  const shouldPublish = onMain && publishInput;
  const plan = {
    shouldBuild: true,
    publish: shouldPublish,
    autoBump,
    version,
    tag,
    commit: headSha,
    previousTag: previous?.tag_name || null,
    notesSource: notes.source || "commits",
    releaseNotes: notes.markdown,
  };
  writeFileSync(resolve(ROOT, "release-plan.json"), `${JSON.stringify(plan, null, 2)}\n`);
  writeOutputs({
    should_build: outputFlag(true),
    publish: outputFlag(shouldPublish),
    version,
    release_tag: tag,
    auto_bump: autoBump,
  });
  summary(`准备发布 Cooool Mark ${tag}（autoBump=${autoBump}, commits=${userCommits.length}）`);
}

const isMain =
  import.meta.main ??
  (Boolean(process.argv[1]) && resolve(process.argv[1]) === fileURLToPath(import.meta.url));

if (isMain) {
  await main();
}
