#!/usr/bin/env python3
"""Generate Light + Dark Warm Paper animated SVG plates (zh-CN & en) for Cooool Mark READMEs.

    python3 scripts/build_readme_svgs.py
"""
import base64
import xml.etree.ElementTree as ET
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT_DIR = ROOT / "docs" / "readme"
APP_ICON = ROOT / "resources" / "icon.png"

THEMES = {
    "light": {
        "bg": "#faf9f5",
        "paper": "#ffffff",
        "sidebar": "#f3f1ea",
        "shell": "#f0eee6",
        "border": "#ebe8df",
        "btnBorder": "#dbd7cc",
        "ink": "#1f1e1d",
        "text": "#1f1e1d",
        "secondary": "#4f4d47",
        "muted": "#6c6b64",
        "accent": "#d97757",
        "accentStrong": "#9a4a2e",
        "accentBtn": "#b5583a",
        "accentSoft": "#f3e7e0",
        "duoFill": "#efe0d7",
        "cardHover": "#f3f1ea",
        "cardHoverBorder": "#ddd9ce",
        "lineStrong": "#8f8d84",
        "lineMid": "#c7c4ba",
        "lineSoft": "#e7e4dc",
        "greenBg": "#e8f4ec",
        "greenFg": "#2f6b43",
        "keyTop": "#ffffff",
        "keyEdge": "#d8d4c8",
        "notch": "#141415",
    },
    "dark": {
        "bg": "#0f0f10",
        "paper": "#1c1c1b",
        "sidebar": "#161617",
        "shell": "#1f1f1d",
        "border": "#282826",
        "btnBorder": "#383834",
        "ink": "#eae8e1",
        "text": "#eae8e1",
        "secondary": "#bdbbb3",
        "muted": "#8e8c84",
        "accent": "#e0875f",
        "accentStrong": "#f0a07c",
        "accentBtn": "#c96a48",
        "accentSoft": "#3a2a22",
        "duoFill": "#3b2c24",
        "cardHover": "#242422",
        "cardHoverBorder": "#3a3a35",
        "lineStrong": "#7d7b73",
        "lineMid": "#474641",
        "lineSoft": "#2c2c29",
        "greenBg": "#192f21",
        "greenFg": "#6dd59c",
        "keyTop": "#262624",
        "keyEdge": "#070708",
        "notch": "#050506",
    },
}

COPY = {
    "zh": {
        "hero_label": "Cooool Mark — 书签随手唤，秒搜即达",
        "hero_slogan": "书签随手唤，秒搜即达",
        "hero_slogan_size": 42,
        "hero_sub1": "不用注册登录，书签存在自己电脑里，支持 iCloud 同步。",
        "hero_sub2": "纯 Go 原生桌面应用 · macOS / Windows / Linux 永久免费",
        "hero_cta": "下载最新版本",
        "hero_cta_w": 176,
        "hero_cta_tx": 128,
        "hero_cta_ax": 196,
        "hero_repo_x": 248,
        "hero_side1": "常规",
        "hero_side2": "常用",
        "hero_side3": "设计",
        "hero_tab1": "Mark",
        "hero_tab2": "AI",
        "hero_tab3": "Dev",
        "hero_search": "搜索书签",
        "hero_sec": "常用",
        "hero_sec_cx": 784,
        "hero_sec2_cx": 784,
        "hero_cards": [
            ("G", "#24292f", "GitHub", "github.com"),
            ("C", "#b5583a", "Claude", "claude.ai"),
            ("F", "#3b6e8f", "Figma", "figma.com"),
            ("V", "#2f62d6", "Vercel", "vercel.com"),
        ],
        "feat_label": "Cooool Mark 核心体验：四种唤出方式、拼音秒搜与贴网址入库、本地 AI 润色、模板书签与双视图",
        "f1_title": "四种方式，随手唤出",
        "f1_sub": "主窗口、菜单栏、屏幕刘海与贴边滑出按需开启",
        "f1_pills": ["主窗口", "菜单栏", "刘海", "贴边"],
        "f2_title": "拼音秒搜，贴网址即入库",
        "f2_sub": "支持拼音首字母、域名与标签检索 · 粘贴链接回车收藏",
        "f2_query": "shm / github.com",
        "f2_caret_x": 836,
        "f2_save": "回车收藏这个网址",
        "f2_hit": "书签目录",
        "f3_title": "本地 AI 润色，自动抓图标",
        "f3_sub": "自动补全标题、备注与图标 · 支持离线 Qwen 与 OpenAI 兼容",
        "f3_raw": "https://github.com/egoist/mygo",
        "f3_done": "MyGo 原生桌面框架",
        "f3_badge": "已润色",
        "f3_chips": ["Qwen 本地模型", "OpenAI 兼容", "自动抓图标"],
        "f4_title": "模板书签 &amp; 紧凑横卡视图",
        "f4_sub": "含 {query} 的链接即点即查 · 列表与横卡网格随时切换",
        "f4_tpl": "github.com/search?q=",
        "f4_q": "mygo",
        "f4_enter": "↵ 打开",
        "f4_grid": "紧凑横卡",
        "f4_list": "极简列表",
        "modes_label": "Cooool Mark 本地优先与多端同步：本地 JSON 与 iCloud、浏览器一键导入、每日备份、轻量原生",
        "modes_title": "开箱即用，数据始终属于你",
        "modes_link": "最新版本",
        "modes_cards": [
            ("本地 &amp; iCloud", "明文 JSON · 任意同步目录"),
            ("一键导入恢复", "Chrome / Safari HTML · 备份"),
            ("多端随手唤出", "刘海 · 贴边 · 菜单栏 · 窗口"),
            ("原生轻量无负担", "纯 Go + MyGo · 无 Electron"),
        ],
        "hi_label": "Cooool Mark 核心承诺：100% 本地、免注册登录、离线 AI、Apache 2.0 永久免费",
        "hi_cols": [
            ("100% 本地", "JSON 明文存在自己电脑"),
            ("免注册登录", "打开即用，断网照常查"),
            ("离线本地 AI", "内置 Qwen，不另付会员"),
            ("永久免费", "Apache 2.0 源码公开"),
        ],
        "hi_cta": "查看全部版本",
        "hi_cta_tx": 1072,
    },
    "en": {
        "hero_label": "Cooool Mark — Bookmarks at hand. Instant search.",
        "hero_slogan": "Bookmarks at hand.",
        "hero_slogan_size": 40,
        "hero_sub1": "No sign-up. Every bookmark stays on your disk with iCloud sync.",
        "hero_sub2": "Native Go desktop app · Free for macOS, Windows &amp; Linux",
        "hero_cta": "Download Latest",
        "hero_cta_w": 192,
        "hero_cta_tx": 134,
        "hero_cta_ax": 214,
        "hero_repo_x": 266,
        "hero_side1": "General",
        "hero_side2": "Daily",
        "hero_side3": "Design",
        "hero_tab1": "Mark",
        "hero_tab2": "AI",
        "hero_tab3": "Dev",
        "hero_search": "Search...",
        "hero_sec": "Daily",
        "hero_sec_cx": 790,
        "hero_sec2_cx": 802,
        "hero_cards": [
            ("G", "#24292f", "GitHub", "github.com"),
            ("C", "#b5583a", "Claude", "claude.ai"),
            ("F", "#3b6e8f", "Figma", "figma.com"),
            ("V", "#2f62d6", "Vercel", "vercel.com"),
        ],
        "feat_label": "Cooool Mark Highlights: Four Summon Modes, Instant Search &amp; Paste-to-Save, Local AI Polish, Template URLs",
        "f1_title": "Four ways to summon",
        "f1_sub": "Main window, menu bar, macOS notch &amp; screen edge drawer",
        "f1_pills": ["Window", "Tray", "Notch", "Edge"],
        "f2_title": "Instant search &amp; paste-to-save",
        "f2_sub": "Pinyin, domain &amp; tag lookup · paste any URL and hit Enter",
        "f2_query": "github.com",
        "f2_caret_x": 784,
        "f2_save": "Press Enter to save URL",
        "f2_hit": "Bookmarks",
        "f3_title": "Local AI polish &amp; auto icons",
        "f3_sub": "Auto-fetch titles &amp; icons · offline Qwen or OpenAI-compatible",
        "f3_raw": "https://github.com/egoist/mygo",
        "f3_done": "MyGo Native Desktop UI",
        "f3_badge": "Polished",
        "f3_chips": ["Local Qwen", "OpenAI API", "Auto Favicon"],
        "f4_title": "Template URLs &amp; compact grid",
        "f4_sub": "Fill {query} inline to launch · switch list and card grid anytime",
        "f4_tpl": "github.com/search?q=",
        "f4_q": "mygo",
        "f4_enter": "↵ Open",
        "f4_grid": "Compact Grid",
        "f4_list": "Clean List",
        "modes_label": "Cooool Mark Local &amp; Sync: Local JSON &amp; iCloud, Browser Import, Daily Backup, Native Lightweight",
        "modes_title": "Ready out of the box, your data stays yours",
        "modes_link": "Latest Release",
        "modes_cards": [
            ("Local &amp; iCloud", "Plain JSON · Any sync folder"),
            ("One-Click Import", "Chrome / Safari HTML · Backups"),
            ("Always Reachable", "Notch · Edge · Tray · Window"),
            ("Lightweight Native", "Pure Go + MyGo · No Electron"),
        ],
        "hi_label": "Cooool Mark Promises: 100% Local, No Sign-up, Offline AI, Free Forever",
        "hi_cols": [
            ("100% Local", "Plain JSON on your disk"),
            ("No Sign-up", "Works offline by default"),
            ("Offline AI", "Local Qwen, zero extra fee"),
            ("Free Forever", "Apache 2.0 open source"),
        ],
        "hi_cta": "All Releases",
        "hi_cta_tx": 1070,
    },
}

FONTS = """
  .sans{font-family:-apple-system,BlinkMacSystemFont,"PingFang SC","Hiragino Sans GB","Microsoft YaHei","Helvetica Neue",sans-serif}
  .serif{font-family:"Songti SC","Noto Serif CJK SC",Georgia,serif}
  .mono{font-family:"JetBrains Mono",ui-monospace,"SF Mono",Menlo,Consolas,monospace}
"""


def app_icon_b64() -> str:
    return "data:image/png;base64," + base64.b64encode(APP_ICON.read_bytes()).decode("ascii")


def arrow_ne(cx: float, cy: float, color: str, scale: float = 1.0) -> str:
    return (
        f'<g transform="translate({cx} {cy}) scale({scale})" '
        f'fill="none" stroke="{color}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">'
        f'<path d="M-4.5 4.5 L4.5 -4.5 M-1.5 -4.5 H4.5 V1.5"/></g>'
    )


def frame(w: int, h: int, uid: str, label: str, P: dict, css: str, body: str) -> str:
    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}" role="img" aria-label="{label}" preserveAspectRatio="xMidYMid meet">
  <defs>
    <clipPath id="fr{uid}"><rect x="0" y="0" width="{w}" height="{h}" rx="18"/></clipPath>
  </defs>
  <style>
{FONTS}{css}
  @media(prefers-reduced-motion:reduce){{*{{animation:none!important}}}}
  </style>
  <g clip-path="url(#fr{uid})">
    <rect width="{w}" height="{h}" fill="{P['bg']}"/>
{body}
  </g>
  <rect x="0.75" y="0.75" width="{w - 1.5}" height="{h - 1.5}" rx="17.25" fill="none" stroke="{P['border']}" stroke-width="1.5"/>
</svg>
"""


# ------------------------------------------------------------------ 1. Hero
def build_hero(P: dict, T: dict, tag: str) -> str:
    css = """
  .h-hover{animation:h-hover 7s ease-in-out infinite}
  .h-toolbar{animation:h-toolbar 7s ease-in-out infinite}
  .h-caret{animation:h-caret 1.15s steps(1,end) infinite}
  .h-pill{animation:h-pill 7s ease-in-out infinite}
  @keyframes h-hover{
    0%,14%,100%{opacity:0}
    28%,72%{opacity:1}
  }
  @keyframes h-toolbar{
    0%,18%,100%{opacity:0;transform:translateX(6px)}
    32%,70%{opacity:1;transform:translateX(0)}
  }
  @keyframes h-caret{0%,49%{opacity:1}50%,100%{opacity:0}}
  @keyframes h-pill{0%,100%{opacity:.85}50%{opacity:1}}
"""
    icon_uri = app_icon_b64()
    c0, c1, c2, c3 = T["hero_cards"]

    def card_svg(x: int, y: int, w: int, letter: str, bg: str, title: str, domain: str, is_hover: bool = False) -> str:
        if not is_hover:
            return f"""      <rect x="{x}" y="{y}" width="{w}" height="58" rx="10" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1.2"/>
      <rect x="{x + 12}" y="{y + 13}" width="32" height="32" rx="8" fill="{bg}"/>
      <text class="sans" x="{x + 28}" y="{y + 34}" text-anchor="middle" font-size="14" font-weight="700" fill="#ffffff">{letter}</text>
      <text class="sans" x="{x + 54}" y="{y + 27}" font-size="13.5" font-weight="600" fill="{P['ink']}">{title}</text>
      <text class="sans" x="{x + 54}" y="{y + 44}" font-size="11.5" fill="{P['muted']}">{domain}</text>"""
        return f"""      <rect x="{x}" y="{y}" width="{w}" height="58" rx="10" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1.2"/>
      <rect class="h-hover" x="{x}" y="{y}" width="{w}" height="58" rx="10" fill="{P['cardHover']}" stroke="{P['cardHoverBorder']}" stroke-width="1.2"/>
      <rect x="{x + 12}" y="{y + 13}" width="32" height="32" rx="8" fill="{bg}"/>
      <text class="sans" x="{x + 28}" y="{y + 34}" text-anchor="middle" font-size="14" font-weight="700" fill="#ffffff">{letter}</text>
      <text class="sans" x="{x + 54}" y="{y + 27}" font-size="13.5" font-weight="600" fill="{P['ink']}">{title}</text>
      <text class="sans" x="{x + 54}" y="{y + 44}" font-size="11.5" fill="{P['muted']}">{domain}</text>
      <g class="h-toolbar">
        <rect x="{x + w - 76}" y="{y + 15}" width="68" height="28" rx="7" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1"/>
        <path d="M{x + w - 63} {y + 33} l6 -6 m-4 0 h4 v4" fill="none" stroke="{P['secondary']}" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        <path d="M{x + w - 45} {y + 33} l5 -5 l2 2 l-5 5 h-2 z" fill="none" stroke="{P['secondary']}" stroke-width="1.4" stroke-linejoin="round"/>
        <circle cx="{x + w - 24}" cy="{y + 29}" r="1.4" fill="{P['secondary']}"/>
        <circle cx="{x + w - 19}" cy="{y + 29}" r="1.4" fill="{P['secondary']}"/>
        <circle cx="{x + w - 14}" cy="{y + 29}" r="1.4" fill="{P['secondary']}"/>
      </g>"""

    body = f"""    <!-- Left Brand & Copy -->
    <defs>
      <clipPath id="heroIcon{tag}"><rect x="52" y="54" width="58" height="58" rx="14"/></clipPath>
      <clipPath id="winClip{tag}"><rect x="588" y="40" width="564" height="360" rx="14"/></clipPath>
    </defs>
    <image href="{icon_uri}" x="52" y="54" width="58" height="58" clip-path="url(#heroIcon{tag})" preserveAspectRatio="xMidYMid slice"/>
    <rect x="52" y="54" width="58" height="58" rx="14" fill="none" stroke="{P['border']}" stroke-width="1.2"/>
    <text class="sans" x="124" y="96" font-size="34" font-weight="700" letter-spacing="-0.6" fill="{P['ink']}">Cooool Mark</text>

    <text class="sans" x="52" y="190" font-size="{T['hero_slogan_size']}" font-weight="700" letter-spacing="-0.6" fill="{P['ink']}">{T['hero_slogan']}</text>
    <text class="sans" x="52" y="240" font-size="18" fill="{P['secondary']}">{T['hero_sub1']}</text>
    <text class="sans" x="52" y="272" font-size="15.5" fill="{P['muted']}">{T['hero_sub2']}</text>

    <rect x="52" y="320" width="{T['hero_cta_w']}" height="46" rx="23" fill="{P['accentBtn']}"/>
    <text class="sans" x="{T['hero_cta_tx']}" y="349" text-anchor="middle" font-size="16" font-weight="600" fill="#ffffff">{T['hero_cta']}</text>
    {arrow_ne(T['hero_cta_ax'], 343, "#ffffff", 0.95)}

    <text class="mono" x="{T['hero_repo_x']}" y="349" font-size="14.5" font-weight="500" fill="{P['accentStrong']}">eachann1024/cooool-mark</text>

    <!-- Right Animated Cooool Mark Window -->
    <g clip-path="url(#winClip{tag})">
      <rect x="588" y="40" width="564" height="360" fill="{P['bg']}"/>

      <!-- Sidebar -->
      <rect x="588" y="40" width="136" height="360" fill="{P['sidebar']}"/>
      <line x1="724" y1="40" x2="724" y2="400" stroke="{P['border']}" stroke-width="1"/>
      <circle cx="610" cy="61" r="4.8" fill="#ec6a5e"/>
      <circle cx="626" cy="61" r="4.8" fill="#f4bf4f"/>
      <circle cx="642" cy="61" r="4.8" fill="#61c554"/>

      <!-- Sidebar Items with Counts -->
      <rect x="598" y="90" width="116" height="30" rx="7" fill="{P['border']}"/>
      <rect x="598" y="99" width="3" height="12" rx="1.5" fill="{P['accent']}"/>
      <text class="sans" x="612" y="110" font-size="12.5" font-weight="600" fill="{P['ink']}">{T['hero_side2']}</text>
      <text class="sans" x="704" y="110" text-anchor="end" font-size="11.5" fill="{P['muted']}">8</text>

      <text class="sans" x="612" y="142" font-size="12.5" fill="{P['secondary']}">{T['hero_side1']}</text>
      <text class="sans" x="704" y="142" text-anchor="end" font-size="11.5" fill="{P['muted']}">3</text>

      <text class="sans" x="612" y="172" font-size="12.5" fill="{P['secondary']}">{T['hero_side3']}</text>
      <text class="sans" x="704" y="172" text-anchor="end" font-size="11.5" fill="{P['muted']}">6</text>

      <!-- Top Header -->
      <rect x="742" y="50" width="52" height="24" rx="12" fill="none"/>
      <text class="sans" x="764" y="66" text-anchor="middle" font-size="12.5" fill="{P['muted']}">{T['hero_tab1']}</text>
      <rect class="h-pill" x="798" y="50" width="42" height="24" rx="12" fill="{P['accentSoft']}"/>
      <text class="sans" x="819" y="66" text-anchor="middle" font-size="12.5" font-weight="600" fill="{P['accentStrong']}">{T['hero_tab2']}</text>
      <text class="sans" x="862" y="66" text-anchor="middle" font-size="12.5" fill="{P['muted']}">{T['hero_tab3']}</text>

      <!-- Search Input -->
      <rect x="976" y="48" width="156" height="28" rx="7" fill="{P['shell']}"/>
      <circle cx="991" cy="61" r="4.2" fill="none" stroke="{P['muted']}" stroke-width="1.4"/>
      <line x1="994" y1="64" x2="997" y2="67" stroke="{P['muted']}" stroke-width="1.4" stroke-linecap="round"/>
      <text class="sans" x="1004" y="66" font-size="12" fill="{P['muted']}">{T['hero_search']}</text>
      <rect class="h-caret" x="1058" y="55" width="1.8" height="14" rx="0.9" fill="{P['accent']}"/>

      <!-- Section Heading with Count & Rule -->
      <text class="serif" x="744" y="114" font-size="15" font-weight="700" fill="{P['ink']}">{T['hero_sec']}</text>
      <text class="sans" x="{T['hero_sec_cx']}" y="113" font-size="12" fill="{P['muted']}">8</text>
      <line x1="{T['hero_sec_cx'] + 18}" y1="109" x2="1132" y2="109" stroke="{P['border']}" stroke-width="1"/>

      <!-- Compact Horizontal Grid Cards (2x2 preview) -->
{card_svg(744, 128, 188, *c0, False)}
{card_svg(942, 128, 190, *c1, True)}
{card_svg(744, 196, 188, *c2, False)}
{card_svg(942, 196, 190, *c3, False)}

      <!-- Second Section -->
      <text class="serif" x="744" y="286" font-size="15" font-weight="700" fill="{P['ink']}">{T['hero_side3']}</text>
      <text class="sans" x="{T['hero_sec2_cx']}" y="285" font-size="12" fill="{P['muted']}">6</text>
      <line x1="{T['hero_sec2_cx'] + 18}" y1="281" x2="1132" y2="281" stroke="{P['border']}" stroke-width="1"/>
      <rect x="744" y="300" width="188" height="58" rx="10" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1.2"/>
      <rect x="756" y="313" width="32" height="32" rx="8" fill="#5b4a8a"/>
      <rect x="798" y="320" width="84" height="7" rx="3.5" fill="{P['lineStrong']}"/>
      <rect x="798" y="335" width="110" height="6" rx="3" fill="{P['lineMid']}"/>

      <rect x="942" y="300" width="190" height="58" rx="10" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1.2"/>
      <rect x="954" y="313" width="32" height="32" rx="8" fill="#0a6f8e"/>
      <rect x="996" y="320" width="96" height="7" rx="3.5" fill="{P['lineStrong']}"/>
      <rect x="996" y="335" width="76" height="6" rx="3" fill="{P['lineMid']}"/>
    </g>
    <rect x="588" y="40" width="564" height="360" rx="14" fill="none" stroke="{P['border']}" stroke-width="1.5"/>"""
    return frame(1200, 440, f"H{tag}", T["hero_label"], P, css, body)


# ------------------------------------------------------------------ 2. Features (2x2 Bento)
def build_features(P: dict, T: dict, tag: str) -> str:
    css = """
  .f-notch{animation:f-notch 6s cubic-bezier(.22,1,.36,1) infinite}
  .f-edge{animation:f-edge 6s cubic-bezier(.22,1,.36,1) infinite}
  .f-caret{animation:f-caret 1.1s steps(1,end) infinite}
  .f-save{animation:f-save 5.5s ease-in-out infinite}
  .f-spark{transform-origin:308px 420px;animation:f-spark 5s ease-in-out infinite}
  .f-badge{animation:f-badge 5s ease-in-out infinite}
  .f-tpl{animation:f-tpl 5s ease-in-out infinite}
  @keyframes f-notch{0%,15%,100%{transform:translateY(0)}40%,72%{transform:translateY(14px)}}
  @keyframes f-edge{0%,20%,100%{transform:translateX(0)}48%,78%{transform:translateX(-14px)}}
  @keyframes f-caret{0%,49%{opacity:1}50%,100%{opacity:0}}
  @keyframes f-save{0%,100%{opacity:.78;transform:translateY(0)}50%{opacity:1;transform:translateY(-2px)}}
  @keyframes f-spark{0%,100%{opacity:.5;transform:scale(.95)}50%{opacity:1;transform:scale(1.08)}}
  @keyframes f-badge{0%,100%{opacity:.82}50%{opacity:1}}
  @keyframes f-tpl{0%,100%{opacity:.85}50%{opacity:1}}
"""
    p0, p1, p2, p3 = T["f1_pills"]
    ch0, ch1, ch2 = T["f3_chips"]

    body = f"""    <!-- Card 1 (TL): Four Summon Modes -->
    <rect x="28" y="28" width="562" height="226" rx="14" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1.2"/>
    <text class="sans" x="56" y="68" font-size="22" font-weight="700" fill="{P['ink']}">{T['f1_title']}</text>
    <text class="sans" x="56" y="96" font-size="15.5" fill="{P['secondary']}">{T['f1_sub']}</text>

    <rect x="56" y="116" width="506" height="114" rx="10" fill="{P['bg']}" stroke="{P['border']}" stroke-width="1"/>
    <!-- Mini Screen with Animated Notch + Edge Drawer -->
    <defs>
      <clipPath id="scrClip{tag}"><rect x="76" y="128" width="186" height="90" rx="8"/></clipPath>
    </defs>
    <g clip-path="url(#scrClip{tag})">
      <rect x="76" y="128" width="186" height="90" fill="{P['paper']}"/>
      <rect x="134" y="128" width="70" height="10" rx="4" fill="{P['notch']}"/>
      <g class="f-notch">
        <rect x="116" y="130" width="106" height="32" rx="7" fill="{P['notch']}"/>
        <rect x="128" y="139" width="16" height="14" rx="4" fill="{P['accent']}"/>
        <rect x="150" y="139" width="16" height="14" rx="4" fill="#3b6e8f"/>
        <rect x="172" y="139" width="16" height="14" rx="4" fill="#5b4a8a"/>
        <rect x="194" y="139" width="16" height="14" rx="4" fill="#0a6f8e"/>
      </g>
      <g class="f-edge">
        <rect x="238" y="144" width="38" height="62" rx="6" fill="{P['accentSoft']}" stroke="{P['accent']}" stroke-width="1.2"/>
        <rect x="246" y="154" width="22" height="5" rx="2.5" fill="{P['accent']}"/>
        <rect x="246" y="166" width="18" height="5" rx="2.5" fill="{P['accent']}" opacity=".6"/>
        <rect x="246" y="178" width="20" height="5" rx="2.5" fill="{P['accent']}" opacity=".4"/>
      </g>
    </g>
    <rect x="76" y="128" width="186" height="90" rx="8" fill="none" stroke="{P['border']}" stroke-width="1.2"/>

    <!-- Four Mode Pills -->
    <rect x="286" y="136" width="120" height="34" rx="10" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1"/>
    <circle cx="304" cy="153" r="4" fill="{P['accent']}"/>
    <text class="sans" x="318" y="158" font-size="14" font-weight="600" fill="{P['ink']}">{p0}</text>

    <rect x="418" y="136" width="124" height="34" rx="10" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1"/>
    <circle cx="436" cy="153" r="4" fill="{P['accent']}"/>
    <text class="sans" x="450" y="158" font-size="14" font-weight="600" fill="{P['ink']}">{p1}</text>

    <rect x="286" y="180" width="120" height="34" rx="10" fill="{P['accentSoft']}" stroke="{P['accent']}" stroke-width="1.1"/>
    <circle cx="304" cy="197" r="4" fill="{P['accent']}"/>
    <text class="sans" x="318" y="202" font-size="14" font-weight="600" fill="{P['accentStrong']}">{p2}</text>

    <rect x="418" y="180" width="124" height="34" rx="10" fill="{P['accentSoft']}" stroke="{P['accent']}" stroke-width="1.1"/>
    <circle cx="436" cy="197" r="4" fill="{P['accent']}"/>
    <text class="sans" x="450" y="202" font-size="14" font-weight="600" fill="{P['accentStrong']}">{p3}</text>

    <!-- Card 2 (TR): Instant Pinyin Search & Paste URL -->
    <rect x="610" y="28" width="562" height="226" rx="14" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1.2"/>
    <text class="sans" x="638" y="68" font-size="22" font-weight="700" fill="{P['ink']}">{T['f2_title']}</text>
    <text class="sans" x="638" y="96" font-size="15.5" fill="{P['secondary']}">{T['f2_sub']}</text>

    <rect x="638" y="116" width="506" height="114" rx="10" fill="{P['bg']}" stroke="{P['border']}" stroke-width="1"/>
    <!-- Search Input Bar -->
    <rect x="656" y="130" width="470" height="34" rx="8" fill="{P['paper']}" stroke="{P['accent']}" stroke-width="1.3"/>
    <circle cx="674" cy="146" r="5" fill="none" stroke="{P['accent']}" stroke-width="1.6"/>
    <line x1="678" y1="150" x2="682" y2="154" stroke="{P['accent']}" stroke-width="1.6" stroke-linecap="round"/>
    <text class="mono" x="692" y="152" font-size="14" fill="{P['ink']}">{T['f2_query']}</text>
    <rect class="f-caret" x="{T['f2_caret_x']}" y="139" width="2" height="16" rx="1" fill="{P['accent']}"/>
    <rect x="1076" y="137" width="40" height="20" rx="5" fill="{P['shell']}"/>
    <text class="mono" x="1096" y="151" text-anchor="middle" font-size="11.5" fill="{P['muted']}">⌘F</text>

    <!-- Paste-to-Save Result Card -->
    <g class="f-save">
      <rect x="656" y="174" width="470" height="44" rx="9" fill="{P['accentSoft']}" stroke="{P['accent']}" stroke-width="1.1"/>
      <rect x="668" y="183" width="26" height="26" rx="7" fill="{P['accentBtn']}"/>
      <path d="M681 190 v12 M675 196 h12" stroke="#ffffff" stroke-width="2" stroke-linecap="round"/>
      <text class="sans" x="706" y="201" font-size="14" font-weight="600" fill="{P['accentStrong']}">{T['f2_save']}</text>
      <rect x="1082" y="185" width="32" height="22" rx="6" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1"/>
      <text class="mono" x="1098" y="200" text-anchor="middle" font-size="12.5" fill="{P['muted']}">↵</text>
    </g>

    <!-- Card 3 (BL): Local AI Polish & Auto Icons -->
    <rect x="28" y="274" width="562" height="226" rx="14" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1.2"/>
    <text class="sans" x="56" y="314" font-size="22" font-weight="700" fill="{P['ink']}">{T['f3_title']}</text>
    <text class="sans" x="56" y="342" font-size="15.5" fill="{P['secondary']}">{T['f3_sub']}</text>

    <rect x="56" y="362" width="506" height="114" rx="10" fill="{P['bg']}" stroke="{P['border']}" stroke-width="1"/>
    <!-- Raw URL -> Polished Card -->
    <rect x="72" y="374" width="212" height="48" rx="9" fill="{P['shell']}" stroke="{P['border']}" stroke-width="1"/>
    <text class="mono" x="84" y="403" font-size="11.5" fill="{P['muted']}">{T['f3_raw'][:26]}...</text>

    <g class="f-spark">
      <path d="M294 398 h22 m-6 -5 l6 5 l-6 5" fill="none" stroke="{P['accent']}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
    </g>

    <rect x="326" y="374" width="220" height="48" rx="9" fill="{P['paper']}" stroke="{P['accent']}" stroke-width="1.2"/>
    <rect x="336" y="384" width="28" height="28" rx="7" fill="{P['accentBtn']}"/>
    <text class="sans" x="350" y="403" text-anchor="middle" font-size="13" font-weight="700" fill="#ffffff">M</text>
    <text class="sans" x="372" y="396" font-size="12.5" font-weight="600" fill="{P['ink']}">{T['f3_done']}</text>
    <rect class="f-badge" x="372" y="402" width="54" height="15" rx="4" fill="{P['accentSoft']}"/>
    <text class="sans" x="399" y="413" text-anchor="middle" font-size="10" font-weight="600" fill="{P['accentStrong']}">{T['f3_badge']}</text>

    <!-- AI Engine Chips -->
    <rect x="72" y="436" width="152" height="28" rx="14" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1"/>
    <text class="sans" x="148" y="455" text-anchor="middle" font-size="13" fill="{P['secondary']}">{ch0}</text>

    <rect x="234" y="436" width="150" height="28" rx="14" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1"/>
    <text class="sans" x="309" y="455" text-anchor="middle" font-size="13" fill="{P['secondary']}">{ch1}</text>

    <rect x="394" y="436" width="152" height="28" rx="14" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1"/>
    <text class="sans" x="470" y="455" text-anchor="middle" font-size="13" fill="{P['secondary']}">{ch2}</text>

    <!-- Card 4 (BR): Template Bookmarks & Dual View -->
    <rect x="610" y="274" width="562" height="226" rx="14" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1.2"/>
    <text class="sans" x="638" y="314" font-size="22" font-weight="700" fill="{P['ink']}">{T['f4_title']}</text>
    <text class="sans" x="638" y="342" font-size="15.5" fill="{P['secondary']}">{T['f4_sub']}</text>

    <rect x="638" y="362" width="506" height="114" rx="10" fill="{P['bg']}" stroke="{P['border']}" stroke-width="1"/>
    <!-- Template Query Bar -->
    <rect x="656" y="376" width="470" height="36" rx="8" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1.1"/>
    <text class="mono" x="670" y="399" font-size="13" fill="{P['muted']}">{T['f4_tpl']}</text>
    <g class="f-tpl">
      <rect x="852" y="384" width="54" height="20" rx="5" fill="{P['accentSoft']}"/>
      <text class="mono" x="879" y="399" text-anchor="middle" font-size="13" font-weight="600" fill="{P['accentStrong']}">{T['f4_q']}</text>
    </g>
    <rect x="1054" y="382" width="62" height="24" rx="6" fill="{P['accentBtn']}"/>
    <text class="sans" x="1085" y="398" text-anchor="middle" font-size="12" font-weight="600" fill="#ffffff">{T['f4_enter']}</text>

    <!-- Dual View Switch Preview -->
    <rect x="656" y="424" width="228" height="38" rx="9" fill="{P['paper']}" stroke="{P['accent']}" stroke-width="1.2"/>
    <rect x="668" y="434" width="18" height="18" rx="4" fill="{P['accentSoft']}"/>
    <rect x="671" y="437" width="5" height="5" rx="1" fill="{P['accent']}"/>
    <rect x="678" y="437" width="5" height="5" rx="1" fill="{P['accent']}"/>
    <rect x="671" y="444" width="5" height="5" rx="1" fill="{P['accent']}"/>
    <rect x="678" y="444" width="5" height="5" rx="1" fill="{P['accent']}"/>
    <text class="sans" x="696" y="448" font-size="13.5" font-weight="600" fill="{P['ink']}">{T['f4_grid']}</text>

    <rect x="898" y="424" width="228" height="38" rx="9" fill="{P['paper']}" stroke="{P['border']}" stroke-width="1"/>
    <line x1="912" y1="438" x2="926" y2="438" stroke="{P['muted']}" stroke-width="1.8" stroke-linecap="round"/>
    <line x1="912" y1="444" x2="926" y2="444" stroke="{P['muted']}" stroke-width="1.8" stroke-linecap="round"/>
    <line x1="912" y1="450" x2="926" y2="450" stroke="{P['muted']}" stroke-width="1.8" stroke-linecap="round"/>
    <text class="sans" x="938" y="448" font-size="13.5" fill="{P['secondary']}">{T['f4_list']}</text>"""
    return frame(1200, 528, f"F{tag}", T["feat_label"], P, css, body)


# ------------------------------------------------------------------ 3. Modes & Sync Strip
def build_modes(P: dict, T: dict, tag: str) -> str:
    css = """
  .e-d1{animation:e-dot 6s ease-in-out 0s infinite}
  .e-d2{animation:e-dot 6s ease-in-out 1.5s infinite}
  .e-d3{animation:e-dot 6s ease-in-out 3.0s infinite}
  .e-d4{animation:e-dot 6s ease-in-out 4.5s infinite}
  @keyframes e-dot{0%,100%{opacity:.28}25%,40%{opacity:1}}
"""
    icon_templates = [
        ("e-d1",
         f'<rect x="{{ix}}" y="{{iy}}" width="30" height="30" rx="8" fill="{P["duoFill"]}"/>'
         f'<path d="M{{x1}} {{y1}} h12 l-3-3 M{{x2}} {{y2}} h-12 l3 3" fill="none" stroke="{P["accent"]}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>'),
        ("e-d2",
         f'<rect x="{{ix}}" y="{{iy}}" width="30" height="30" rx="8" fill="{P["duoFill"]}"/>'
         f'<path d="M{{xc}} {{yt}} v11 M{{xl}} {{ym}} l4 4 4-4" fill="none" stroke="{P["accent"]}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>'),
        ("e-d3",
         f'<rect x="{{ix}}" y="{{iy}}" width="30" height="30" rx="8" fill="{P["duoFill"]}"/>'
         f'<rect x="{{bx}}" y="{{by}}" width="14" height="12" rx="3" fill="none" stroke="{P["accent"]}" stroke-width="1.8"/>'
         f'<line x1="{{bx}}" y1="{{bl}}" x2="{{bx2}}" y2="{{bl}}" stroke="{P["accent"]}" stroke-width="1.6"/>'),
        ("e-d4",
         f'<rect x="{{ix}}" y="{{iy}}" width="30" height="30" rx="8" fill="{P["duoFill"]}"/>'
         f'<path d="M{{xa}} {{yb}} l4-6 4 3 5-8" fill="none" stroke="{P["accent"]}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>'),
    ]
    parts = [
        f'    <text class="sans" x="32" y="52" font-size="22" font-weight="700" fill="{P["ink"]}">{T["modes_title"]}</text>',
        f'    <text class="sans" x="1146" y="50" text-anchor="end" font-size="15.5" font-weight="600" fill="{P["accentStrong"]}">{T["modes_link"]}</text>',
        f'    {arrow_ne(1160, 44, P["accentStrong"], 0.9)}',
    ]
    cw = 272
    gap = 16
    x0 = 32
    y0 = 76
    for idx, ((title, sub), (dcls, icon_tpl)) in enumerate(zip(T["modes_cards"], icon_templates)):
        cx = x0 + idx * (cw + gap)
        ix = cx + 18
        iy = y0 + 18
        icon_svg = icon_tpl.format(
            ix=ix, iy=iy,
            x1=ix + 9, y1=iy + 12, x2=ix + 21, y2=iy + 19,
            xc=ix + 15, yt=iy + 9, xl=ix + 11, ym=iy + 17,
            bx=ix + 8, by=iy + 9, bx2=ix + 22, bl=iy + 13,
            xa=ix + 8, yb=iy + 21,
        )
        parts.append(
            f'    <rect x="{cx}" y="{y0}" width="{cw}" height="108" rx="12" fill="{P["paper"]}" stroke="{P["border"]}" stroke-width="1.2"/>\n'
            f'    {icon_svg}\n'
            f'    <text class="sans" x="{cx + 58}" y="{y0 + 39}" font-size="17.5" font-weight="700" fill="{P["ink"]}">{title}</text>\n'
            f'    <circle class="{dcls}" cx="{cx + cw - 20}" cy="{y0 + 32}" r="4" fill="{P["accent"]}"/>\n'
            f'    <text class="sans" x="{cx + 18}" y="{y0 + 82}" font-size="14.5" fill="{P["secondary"]}">{sub}</text>'
        )
    return frame(1200, 212, f"M{tag}", T["modes_label"], P, css, "\n".join(parts))


# ------------------------------------------------------------------ 4. Highlights / Promises Strip
def build_highlights(P: dict, T: dict, tag: str) -> str:
    parts = []
    col_w = 236
    x0 = 36
    for idx, (head, sub) in enumerate(T["hi_cols"]):
        cx = x0 + idx * col_w
        if idx > 0:
            parts.append(f'    <line x1="{cx - 18}" y1="30" x2="{cx - 18}" y2="96" stroke="{P["border"]}" stroke-width="1.2"/>')
        parts.append(
            f'    <text class="sans" x="{cx}" y="54" font-size="21" font-weight="700" fill="{P["ink"]}">{head}</text>\n'
            f'    <text class="sans" x="{cx}" y="84" font-size="14.5" fill="{P["secondary"]}">{sub}</text>'
        )
    parts.append(
        f'    <rect x="998" y="40" width="168" height="46" rx="23" fill="{P["accentSoft"]}"/>\n'
        f'    <text class="sans" x="{T["hi_cta_tx"]}" y="69" text-anchor="middle" font-size="15.5" font-weight="600" fill="{P["accentStrong"]}">{T["hi_cta"]}</text>\n'
        f'    {arrow_ne(1140, 63, P["accentStrong"], 0.92)}'
    )
    return frame(1200, 126, f"C{tag}", T["hi_label"], P, "", "\n".join(parts))


def main() -> None:
    for lang, text in COPY.items():
        lang_root = OUT_DIR if lang == "zh" else OUT_DIR / "en"
        for theme_name, palette in THEMES.items():
            target_dir = lang_root / theme_name
            target_dir.mkdir(parents=True, exist_ok=True)
            tag = f"{lang[0].upper()}{'L' if theme_name == 'light' else 'D'}"
            plates = {
                "hero": build_hero(palette, text, tag),
                "features": build_features(palette, text, tag),
                "modes": build_modes(palette, text, tag),
                "highlights": build_highlights(palette, text, tag),
            }
            for name, svg in plates.items():
                ET.fromstring(svg)  # strict XML validation
                out = target_dir / f"{name}.svg"
                out.write_text(svg, encoding="utf-8")
                print(f"{out.relative_to(ROOT).as_posix():42s} {out.stat().st_size / 1024:5.1f} KB")


if __name__ == "__main__":
    main()
