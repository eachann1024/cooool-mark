package theme

import "github.com/egoist/mygo/ui"

func svg(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

// Stroke icons (Lucide-like, 1.75 stroke), shown with ui.Icon in the
// text color.
var (
	IconSearch   = svg(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)
	IconPlus     = svg(`<path d="M12 5v14M5 12h14"/>`)
	IconGrid     = svg(`<rect x="3.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.5"/>`)
	IconList     = svg(`<path d="M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01"/>`)
	IconMore     = svg(`<circle cx="5" cy="12" r="1" fill="currentColor"/><circle cx="12" cy="12" r="1" fill="currentColor"/><circle cx="19" cy="12" r="1" fill="currentColor"/>`)
	IconEdit     = svg(`<path d="M4 20h4L19 9l-4-4L4 16v4Z"/>`)
	IconTrash    = svg(`<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/>`)
	IconBack     = svg(`<path d="m15 6-6 6 6 6"/>`)
	IconChevron  = svg(`<path d="m9 6 6 6-6 6"/>`)
	IconClose    = svg(`<path d="M6 6l12 12M18 6 6 18"/>`)
	IconCheck    = svg(`<path d="m5 12 4.5 4.5L19 7"/>`)
	IconLink     = svg(`<path d="M10 14a4 4 0 0 0 5.66 0l3-3a4 4 0 0 0-5.66-5.66l-1 1M14 10a4 4 0 0 0-5.66 0l-3 3a4 4 0 0 0 5.66 5.66l1-1"/>`)
	IconRefresh  = svg(`<path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7"/>`)
	IconBraces   = svg(`<path d="M8 4c-2 0-3 1-3 3v2c0 1.5-.8 2.5-2 3 1.2.5 2 1.5 2 3v2c0 2 1 3 3 3M16 4c2 0 3 1 3 3v2c0 1.5.8 2.5 2 3-1.2.5-2 1.5-2 3v2c0 2-1 3-3 3"/>`)
	IconDrag     = svg(`<circle cx="9" cy="6" r="1" fill="currentColor"/><circle cx="15" cy="6" r="1" fill="currentColor"/><circle cx="9" cy="12" r="1" fill="currentColor"/><circle cx="15" cy="12" r="1" fill="currentColor"/><circle cx="9" cy="18" r="1" fill="currentColor"/><circle cx="15" cy="18" r="1" fill="currentColor"/>`)
	IconSettings = svg(`<path d="M4 7h10M18 7h2M4 17h4M12 17h8"/><circle cx="16" cy="7" r="2"/><circle cx="10" cy="17" r="2"/>`)
	IconExternal = svg(`<path d="M14 4h6v6M20 4l-8 8M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>`)
	IconPin      = svg(`<path d="M9 3h6l-1 6 4 4H6l4-4-1-6ZM12 13v8"/>`)
	IconBookmark = svg(`<path d="M6.5 3.5h11a1 1 0 0 1 1 1V21l-6.5-4.2L5.5 21V4.5a1 1 0 0 1 1-1Z"/>`)
	IconSparkle  = svg(`<path d="M12 3v4M12 17v4M3 12h4M17 12h4M6 6l2.5 2.5M15.5 15.5 18 18M6 18l2.5-2.5M15.5 8.5 18 6"/>`)
	IconDownload = svg(`<path d="M12 4v11M7 10l5 5 5-5M5 20h14"/>`)
	IconPause    = svg(`<path d="M9 5v14M15 5v14"/>`)
	IconFolder   = svg(`<path d="M3.5 6.5a1 1 0 0 1 1-1h5l2 2h8a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1Z"/>`)
	IconCloud    = svg(`<path d="M7 18a4.5 4.5 0 0 1-.6-8.96A6 6 0 0 1 18 9.5a4 4 0 0 1-.5 8.5Z"/>`)
	IconGlobe    = svg(`<circle cx="12" cy="12" r="8.5"/><path d="M3.5 12h17M12 3.5c2.5 2.6 2.5 14.4 0 17M12 3.5c-2.5 2.6-2.5 14.4 0 17"/>`)
	IconTab      = svg(`<rect x="3.5" y="5" width="17" height="14" rx="2"/><path d="M3.5 9.5h17M8 5v4.5"/>`)
	IconUndo     = svg(`<path d="M9 14 4 9l5-5M4 9h10a6 6 0 0 1 0 12h-3"/>`)
	IconCopy     = svg(`<rect x="8.5" y="8.5" width="11" height="11" rx="2"/><path d="M15.5 8.5v-3a1 1 0 0 0-1-1h-9a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h3"/>`)
	IconMove     = svg(`<path d="M3.5 6.5a1 1 0 0 1 1-1h5l2 2h8a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1Z"/><path d="M10 13h6M13.5 10.5 16 13l-2.5 2.5"/>`)
	IconImport   = svg(`<path d="M12 3v12M7 10l5 5 5-5M4 15v4a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-4"/>`)
	IconWarn     = svg(`<path d="M12 4 2.5 20h19Z"/><path d="M12 10v4M12 17h.01"/>`)
	IconContrast = svg(`<circle cx="12" cy="12" r="8.5"/><path fill="currentColor" d="M12 3.5a8.5 8.5 0 0 0 0 17Z"/>`)
	IconMonitor  = svg(`<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>`)
	IconSun      = svg(`<circle cx="12" cy="12" r="4"/><path d="M12 3v2M12 19v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M3 12h2M19 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/>`)
	IconMoon     = svg(`<path d="M20.5 14.5A8.5 8.5 0 0 1 9.5 3.5a8.5 8.5 0 1 0 11 11Z"/>`)
	IconCode     = svg(`<path d="m8 8-4 4 4 4M16 8l4 4-4 4M13.5 5l-3 14"/>`)
)
