package theme

import "github.com/egoist/mygo/ui"

// Mode icons shared by the notch, tray, edge and orb panels.
var (
	IconPinFilled     = svg(`<path fill="currentColor" d="M9 3h6l-1 6 4 4H6l4-4-1-6Z"/><path d="M12 13v8"/>`)
	IconCollapseRight = svg(`<path d="m9 6 6 6-6 6M19 5v14"/>`)
	IconCollapseLeft  = svg(`<path d="m15 6-6 6 6 6M5 5v14"/>`)
	IconOrbGrid       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><circle cx="6" cy="6" r="1.8"/><circle cx="12" cy="6" r="1.8"/><circle cx="18" cy="6" r="1.8"/><circle cx="6" cy="12" r="1.8"/><circle cx="12" cy="12" r="1.8"/><circle cx="18" cy="12" r="1.8"/><circle cx="6" cy="18" r="1.8"/><circle cx="12" cy="18" r="1.8"/><circle cx="18" cy="18" r="1.8"/></svg>`))
	IconBookmarkOrb   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#d97757" stroke="#b5583a" stroke-width="1.4" stroke-linejoin="round"><path d="M6.5 3.5h11a1 1 0 0 1 1 1V21l-6.5-4.2L5.5 21V4.5a1 1 0 0 1 1-1Z"/></svg>`))
)
