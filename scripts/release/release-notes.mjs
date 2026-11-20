import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const DIR = fileURLToPath(new URL(".", import.meta.url));
const ROOT = resolve(DIR, "../..");
const REPO_URL = "https://github.com/eachann1024/cooool-mark";

export const EN_SECTIONS = ["### New", "### Improvements", "### Fixes", "### Other"];
export const ZH_SECTIONS = ["### 新功能", "### 改进", "### 修复", "### 其他"];
export const MAX_PER_SECTION = 50;

export const EN_FOOTER = [
  "### Download",
  "",
  "Download the macOS, Windows, or Linux installer from the files attached to this release.",
  "",
  `Repository: ${REPO_URL}`,
].join("\n");

export const ZH_FOOTER = [
  "### 下载",
  "",
  "macOS、Windows 和 Linux 的安装包可以在本页附件里下载。",
  "",
  `仓库：${REPO_URL}`,
].join("\n");

const BOT_AUTHORS = new Set(["github-actions[bot]", "github-actions"]);
const VERSION_BUMP = /^chore\(release\):\s*v\d+\.\d+\.\d+\s*$/i;
const INTERNAL_COMMIT = /^(?:chore|ci|docs|test)(?:\([^)]*\))?!?:\s|^Update README\.md$/i;
const NON_SHIPPABLE_EXACT = new Set([
  "updates/feed.json",
  "README.md",
  "README.en.md",
  "AGENTS.md",
  "LICENSE",
  "NOTICE",
  "THIRD_PARTY_NOTICES.md",
  ".github/pull_request_template.md",
  ".github/workflows/ci.yml",
  "scripts/build_readme_svgs.py",
]);

function isNonShippablePath(filePath) {
  const p = String(filePath || "").replaceAll("\\", "/").replace(/^\.\//, "");
  if (!p) return true;
  if (NON_SHIPPABLE_EXACT.has(p)) return true;
  if (p.startsWith("docs/")) return true;
  if (p.endsWith("_test.go")) return true;
  return false;
}

export function isExcludedCommit({ author = "", subject = "", releaseNote = "", releaseNoteEn = "", paths = [] }) {
  if (BOT_AUTHORS.has(author)) return true;
  if (/^none$/i.test(releaseNote.trim()) || /^none$/i.test(releaseNoteEn.trim())) return true;
  if (VERSION_BUMP.test(subject)) return true;
  const hasExplicitNote = Boolean(releaseNote.trim() || releaseNoteEn.trim());
  if (!hasExplicitNote && INTERNAL_COMMIT.test(subject.trim())) return true;
  if (!hasExplicitNote && Array.isArray(paths) && (paths.length === 0 || paths.every(isNonShippablePath))) return true;
  return false;
}

function splitNote(raw = "") {
  const text = raw.trim();
  if (!text || !text.includes("|||")) return { zh: text, en: "" };
  const [zh, ...rest] = text.split("|||");
  return { zh: zh.trim(), en: rest.join("|||").trim() };
}

export function listCommits(root = ROOT, rangeSpec = "-n 20") {
  const COMMIT_MARKER = "<<<COOOOL_MARK_COMMIT>>>";
  const args = [
    "-c",
    "core.quotepath=false",
    "log",
    "--no-merges",
    "--topo-order",
    "--reverse",
    "--date=format-local:%Y-%m-%d",
    "--name-only",
    "-z",
    `--format=${COMMIT_MARKER}%x00%H%x00%an%x00%cd%x00%s%x00%(trailers:key=Release-Note,valueonly,separator=%x20)%x00%(trailers:key=Release-Note-En,valueonly,separator=%x20)%x00`,
  ];
  if (rangeSpec.startsWith("-n")) args.push(...rangeSpec.split(/\s+/));
  else args.push(rangeSpec);
  const out = execFileSync("git", args, {
    cwd: root,
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
    env: { ...process.env, TZ: "Asia/Shanghai" },
  });
  if (!out) return [];
  const tokens = out.split("\x00");
  const commits = [];
  let idx = 0;
  while (idx < tokens.length) {
    const token = tokens[idx].replace(/^\n+/, "");
    if (token !== COMMIT_MARKER) {
      idx += 1;
      continue;
    }
    const sha = (tokens[idx + 1] || "").trim();
    const author = tokens[idx + 2] || "";
    const date = tokens[idx + 3] || "";
    const subject = tokens[idx + 4] || "";
    const releaseNoteRaw = tokens[idx + 5] || "";
    const releaseNoteEnRaw = tokens[idx + 6] || "";
    idx += 7;
    const paths = [];
    while (idx < tokens.length) {
      const next = tokens[idx].replace(/^\n+/, "");
      if (next === COMMIT_MARKER) break;
      const trimmed = next.trim();
      if (trimmed) paths.push(trimmed);
      idx += 1;
    }
    if (!/^[0-9a-f]{40}$/i.test(sha)) continue;
    const split = splitNote(releaseNoteRaw);
    commits.push({
      sha,
      author,
      date,
      subject,
      releaseNote: split.zh,
      releaseNoteEn: (releaseNoteEnRaw || "").trim() || split.en,
      paths,
    });
  }
  return commits;
}

const TOPIC_RULES = [
  [/grid|card|compact/i, "网格视图改用紧凑横卡排版，并在分组标题与侧栏显示书签数量。", "Redesigned grid view with compact horizontal cards and subgroup bookmark counts."],
  [/sidebar|follow|scroll/i, "滚动浏览书签列表时，左侧子分组选中项会自动联动高亮。", "Sidebar subgroup selection now follows your scroll position automatically."],
  [/notch|edge|tray|mode/i, "优化菜单栏、刘海与贴边滑出唤出体验。", "Improved menu bar, notch, and screen-edge summon modes."],
  [/search|pinyin|template/i, "优化拼音秒搜、贴网址快速入库与模板书签体验。", "Improved Pinyin search, paste-to-save URLs, and template bookmarks."],
  [/ai|polish|qwen|llama|openai/i, "优化本地 AI 自动润色与 OpenAI 兼容接口体验。", "Improved local AI polishing and OpenAI-compatible model support."],
  [/import|backup|sync|icloud|store/i, "优化本地存储、iCloud 同步与浏览器书签导入恢复。", "Improved local storage, iCloud sync, and browser bookmark import/backup."],
  [/icon|fetch|webview/i, "优化网页标题与站点图标自动抓取体验。", "Improved automatic page title and favicon fetching."],
  [/release|workflow|package|license|open.?source/i, "完善跨平台打包发布与开源项目配置。", "Set up cross-platform release packaging and open-source packaging."],
  [/initial|initialize/i, "Cooool Mark 首个开源桌面版本发布，支持 macOS、Windows 与 Linux。", "Initial open-source desktop release of Cooool Mark for macOS, Windows, and Linux."],
];

function kindOf(subject) {
  const typed = /^([a-z]+)(?:\([^)]*\))?!?:/i.exec(subject)?.[1]?.toLowerCase() || "";
  if (typed === "feat") return "feat";
  if (typed === "fix") return "fix";
  if (["perf", "refactor", "style", "improve"].includes(typed)) return "improve";
  if (["ci", "build", "chore", "docs", "test", "release"].includes(typed)) return "other";
  if (/^(fix|resolve|repair)\b/i.test(subject)) return "fix";
  if (/^(add|init|initialize|introduce|support|redesign)\b/i.test(subject)) return "feat";
  return "improve";
}

function finish(text, punct) {
  const clean = String(text || "").trim().replace(/[.。!！?？]+$/, "");
  return clean ? `${clean}${punct}` : "";
}

function cleanEnglish(raw) {
  const text = String(raw || "")
    .replace(/`+/g, "")
    .replace(/\(#\d+\)/g, "")
    .replace(/#\d+\b/g, "")
    .replace(/[\u3400-\u9fff]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  if (!text || text.length < 4) return "Polished everyday bookmark management and interface details.";
  return finish(text.charAt(0).toUpperCase() + text.slice(1), ".");
}

function describeCommit(commit) {
  const subject = String(commit.subject || "").replace(/\s*\(#\d+\)$/, "").trim();
  const kind = kindOf(subject);
  const zhTrailer = String(commit.releaseNote || "").trim();
  const enTrailer = String(commit.releaseNoteEn || "").trim();
  if (zhTrailer && enTrailer) {
    return { kind, zh: finish(zhTrailer, "。"), en: cleanEnglish(enTrailer) };
  }
  const bare = subject.replace(/^[a-z]+(?:\([^)]*\))?!?:\s*/i, "").trim() || subject;
  const matched = TOPIC_RULES.find(([re]) => re.test(subject));
  if (matched) {
    return { kind, zh: finish(matched[1], "。"), en: cleanEnglish(bare || matched[2]) };
  }
  const zh =
    kind === "fix"
      ? "修复了一处日常使用与界面交互问题。"
      : kind === "feat"
        ? "新增了一项提升书签整理与检索效率的功能。"
        : kind === "other"
          ? "完善了安装包构建与发布流程。"
          : "优化了界面细节与交互流畅度。";
  return { kind, zh, en: cleanEnglish(bare) };
}

function renderHalf(title, sections, buckets, lang, footer) {
  const lines = [title, ""];
  let any = false;
  for (let i = 0; i < sections.length; i += 1) {
    const items = buckets[i][lang];
    if (!items.length) continue;
    if (any) lines.push("");
    lines.push(sections[i], ...items.map((item) => `- ${item}`));
    any = true;
  }
  if (!any) return "";
  lines.push("", footer);
  return lines.join("\n");
}

export async function buildReleaseNotes({ commits }) {
  const userCommits = commits.filter((c) => !isExcludedCommit(c));
  if (userCommits.length === 0) {
    return { skip: true, reason: "没有面向用户的新提交", markdown: "" };
  }
  const buckets = [
    { zh: [], en: [], seen: new Set() },
    { zh: [], en: [], seen: new Set() },
    { zh: [], en: [], seen: new Set() },
    { zh: [], en: [], seen: new Set() },
  ];
  const kindIndex = { feat: 0, improve: 1, fix: 2, other: 3 };
  for (const commit of userCommits) {
    const pair = describeCommit(commit);
    const bucket = buckets[kindIndex[pair.kind] ?? 3];
    const key = `${pair.zh}\x00${pair.en}`;
    if (bucket.seen.has(key) || bucket.zh.length >= MAX_PER_SECTION) continue;
    bucket.seen.add(key);
    bucket.zh.push(pair.zh);
    bucket.en.push(pair.en);
  }
  const en = renderHalf("## English", EN_SECTIONS, buckets, "en", EN_FOOTER);
  const zh = renderHalf("## 中文", ZH_SECTIONS, buckets, "zh", ZH_FOOTER);
  if (!en || !zh) {
    return { skip: true, reason: "未生成有效更新说明", markdown: "" };
  }
  return { skip: false, source: "commits", markdown: `${en}\n\n---\n\n${zh}` };
}
