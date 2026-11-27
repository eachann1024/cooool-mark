// Package mode runs the summon modes: notch, tray, screen edge, orb.
package mode

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

var (
	curNotch *notchMode
	curTray  *trayMode
	curEdge  *edgeMode
)

// Apply creates or removes the mode windows to match a.Settings.OpenModes.
// Main thread.
func Apply(a *app.App) {
	if a == nil {
		return
	}
	syncNotch(a, a.Settings.ModeOn(model.ModeNotch))
	syncTray(a, a.Settings.ModeOn(model.ModeTray))
	syncEdge(a, a.Settings.ModeOn(model.ModeEdge))
}

// Close removes the summon surfaces before MyGo asks the remaining windows to close.
func Close() {
	syncNotch(nil, false)
	syncTray(nil, false)
	syncEdge(nil, false)
}

// HasActiveSummonMode reports whether at least one persistent summon surface
// (tray icon, notch panel, or edge trigger) is currently alive.
func HasActiveSummonMode() bool {
	return curTray != nil || (curNotch != nil && curNotch.hasNotch) || curEdge != nil
}

// HideSummoned collapses every summoned panel (after opening a bookmark).
func HideSummoned() {
	if curNotch != nil {
		curNotch.collapse()
	}
	if curTray != nil {
		curTray.hideAttached()
	}
	if curEdge != nil {
		curEdge.collapse()
	}
}

// ToggleNotch enables notch mode if needed and toggles its expanded panel.
func ToggleNotch(a *app.App) {
	if a == nil {
		return
	}
	if !a.Settings.ModeOn(model.ModeNotch) {
		a.Settings.SetMode(model.ModeNotch, true)
		a.SaveSettings()
		if curNotch != nil {
			curNotch.expand()
		}
		return
	}
	if curNotch == nil {
		syncNotch(a, true)
	}
	if curNotch == nil {
		return
	}
	if curNotch.state == notchOpen {
		curNotch.collapse()
	} else {
		curNotch.expand()
	}
}

// ToggleEdge enables edge mode if needed and toggles its expanded panel.
func ToggleEdge(a *app.App) {
	if a == nil {
		return
	}
	if !a.Settings.ModeOn(model.ModeEdge) {
		a.Settings.SetMode(model.ModeEdge, true)
		a.SaveSettings()
		if curEdge != nil {
			curEdge.expand(false)
		}
		return
	}
	if curEdge == nil {
		syncEdge(a, true)
	}
	if curEdge == nil {
		return
	}
	if curEdge.state == edgeOut {
		curEdge.collapse()
	} else {
		curEdge.expand(false)
	}
}

// ToggleTray enables tray mode if needed and toggles the menu bar panel.
func ToggleTray(a *app.App) {
	if a == nil {
		return
	}
	if !a.Settings.ModeOn(model.ModeTray) {
		a.Settings.SetMode(model.ModeTray, true)
		a.SaveSettings()
		if curTray != nil {
			curTray.showAttached()
		}
		return
	}
	if curTray == nil {
		syncTray(a, true)
	}
	if curTray != nil {
		curTray.toggleAttached()
	}
}

// RefreshChrome rebuilds tray menus/tooltips and updates mode window titles after locale changes.
func RefreshChrome(a *app.App) {
	if curTray != nil {
		curTray.refreshChrome(a)
	}
	if curNotch != nil {
		if a != nil {
			curNotch.app = a
		}
		if curNotch.win != nil {
			curNotch.win.SetTitle(locales.ModeNotchTitle())
			curNotch.win.Invalidate()
		}
	}
	if curEdge != nil {
		if a != nil {
			curEdge.app = a
		}
		if curEdge.win != nil {
			curEdge.win.SetTitle(locales.ModeEdgeTitle())
			curEdge.win.Invalidate()
		}
	}
}

// cursorDisplay returns the display containing the cursor, falling back to primary.
func cursorDisplay() mygo.Display {
	p := mygo.Screen.CursorScreenPoint()
	d := mygo.Screen.DisplayNearestPoint(p)
	if d.Bounds.Width > 0 && d.Bounds.Height > 0 {
		return d
	}
	return mygo.Screen.PrimaryDisplay()
}

// displayRect returns the usable screen bounds (WorkArea if valid, else Bounds).
func displayRect(d mygo.Display) mygo.Rectangle {
	if d.WorkArea.Width > 0 && d.WorkArea.Height > 0 {
		return d.WorkArea
	}
	return d.Bounds
}

// urlDropOptions accepts URLs and plain text drags from browsers and other apps.
func urlDropOptions() transfer.DropOptions {
	return transfer.DropOptions{
		Formats:    []transfer.Format{transfer.URIList, transfer.Text},
		Operations: transfer.Copy | transfer.Move,
	}
}

// extractDropURL extracts an http(s) URL from a drag-drop transfer.
func extractDropURL(data transfer.Data) string {
	if urls, err := data.URLs(); err == nil {
		for _, u := range urls {
			if clean := normalizeHTTPURL(u); clean != "" {
				return clean
			}
		}
	}
	if b, err := data.Read(transfer.Text); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if clean := normalizeHTTPURL(line); clean != "" {
				return clean
			}
		}
	}
	return ""
}

func normalizeHTTPURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "#") {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme == "http" || scheme == "https") && u.Host != "" {
		return s
	}
	return ""
}

func modeMenuOptions(p *theme.Palette) ui.PopoverMenuOptions {
	return ui.PopoverMenuOptions{
		Panel: func(panel ui.Element) {
			shadow := ui.RGBA(25, 15, 9, 0.22)
			if p.Dark {
				shadow = ui.RGBA(0, 0, 0, 0.5)
			}
			panel.Background(p.Surface).Border(1, p.Line).Radius(10).Padding(5).
				NoShadow().Shadow(0, 12, 32, -10, shadow)
		},
		Item: func(item ui.Element) {
			item.MinHeight(29).PaddingX(10).Radius(6)
		},
	}
}

func copyModeText(c *ui.Context, a *app.App, value, success string) {
	c.WriteClipboardThen(value, func(err error) {
		if err != nil {
			a.SayErr(locales.ToastCopyFailed(err.Error()), "", nil)
			return
		}
		a.Say(success)
	})
}

func bindBookmarkPopoverMenu(
	c *ui.Context,
	a *app.App,
	el ui.Element,
	b *model.Bookmark,
	opts ui.PopoverMenuOptions,
	menuOpen *bool,
	onOpen func(),
	onEdit func(),
	onDelete func(),
) {
	el.PopoverMenu(func(m *ui.Menu) {
		*menuOpen = true
		if m.Item(locales.ActionOpen()).Icon(theme.MenuIcon(theme.IconExternal)).Chosen() {
			*menuOpen = false
			if onOpen != nil {
				onOpen()
			}
		}
		if m.Item(locales.ActionCopyURL()).Icon(theme.MenuIcon(theme.IconLink)).Disabled(b.URL == "").Chosen() {
			*menuOpen = false
			copyModeText(c, a, b.URL, locales.ToastCopiedURL())
		}
		if m.Item(locales.ActionCopyTitle()).Icon(theme.MenuIcon(theme.IconCopy)).Disabled(b.Title == "").Chosen() {
			*menuOpen = false
			copyModeText(c, a, b.Title, locales.ToastCopiedTitle())
		}
		m.Separator()
		if m.Item(locales.ActionEdit()).Icon(theme.MenuIcon(theme.IconEdit)).Chosen() {
			*menuOpen = false
			if onEdit != nil {
				onEdit()
			}
			if a.Win.ShowEditor != nil {
				a.Win.ShowEditor(app.EditorRequest{ID: b.ID})
			}
		}
		if m.Item(locales.ActionDelete()).Icon(theme.MenuIcon(theme.IconTrash)).Danger().Chosen() {
			*menuOpen = false
			a.Delete(b.ID)
			if onDelete != nil {
				onDelete()
			}
		}
	}, opts)
}

func firstSubID(g *model.Group) string {
	if g != nil && len(g.Subs) > 0 {
		return g.Subs[0].ID
	}
	return ""
}

func initialModeGroup(saved, fallback string) string {
	if saved != "" {
		return saved
	}
	return fallback
}

func activeGroup(lib *model.Library, id string) *model.Group {
	if g := lib.Group(id); g != nil {
		return g
	}
	if len(lib.Groups) > 0 {
		return &lib.Groups[0]
	}
	return nil
}

func templateBookmark(lib *model.Library, id string) *model.Bookmark {
	if b := lib.Bookmark(id); b != nil && !b.InTrash() && b.IsTemplate() {
		return b
	}
	return nil
}

func groupBookmarks(lib *model.Library, g *model.Group) []*model.Bookmark {
	if lib == nil || g == nil {
		return nil
	}
	var out []*model.Bookmark
	seen := make(map[string]bool)
	for _, s := range g.Subs {
		for _, b := range lib.In(g.ID, s.ID) {
			seen[b.ID] = true
			out = append(out, b)
		}
	}
	for _, b := range lib.Live() {
		if b.GroupID == g.ID && !seen[b.ID] {
			out = append(out, b)
		}
	}
	return out
}

func searchBookmarks(a *app.App, query, groupID string) []*model.Bookmark {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	lib := a.Store.Lib()
	if a.Search != nil {
		hits := a.Search.Search(lib, q, groupID)
		out := make([]*model.Bookmark, 0, len(hits))
		for _, h := range hits {
			if h.B != nil {
				out = append(out, h.B)
			}
		}
		return out
	}
	low := strings.ToLower(q)
	var out []*model.Bookmark
	for _, b := range lib.Live() {
		if groupID != "" && b.GroupID != groupID {
			continue
		}
		if strings.Contains(strings.ToLower(b.Title+" "+b.URL+" "+b.Note), low) {
			out = append(out, b)
		}
	}
	return out
}

// modeSearchState holds shared search, selection, and locked-engine state for summon modes.
type modeSearchState struct {
	query         string
	lockedID      string
	lockedKW      string
	selIdx        int
	selTemplateID string
	scrollMoved   bool
	caretPending  bool
	pendingCaret  int
}

func (s *modeSearchState) reset() {
	s.query = ""
	s.lockedID = ""
	s.lockedKW = ""
	s.selIdx = 0
	s.selTemplateID = ""
	s.scrollMoved = false
	s.caretPending = false
	s.pendingCaret = 0
}

func (s *modeSearchState) unlockKeepQuery() {
	s.lockedID = ""
	s.lockedKW = ""
	s.selIdx = 0
	s.selTemplateID = ""
	s.scrollMoved = false
}

func (s *modeSearchState) lockEngine(b *model.Bookmark, kw, rest string) {
	if b == nil {
		return
	}
	if kw == "" {
		kw = firstKeyword(b)
	}
	s.lockedID = b.ID
	s.lockedKW = kw
	s.query = rest
	s.selIdx = 0
	s.selTemplateID = ""
	s.scrollMoved = false
	s.caretPending = true
	s.pendingCaret = utf8.RuneCountInString(rest)
}

func (s *modeSearchState) trimmedQuery() string {
	return strings.TrimSpace(s.query)
}

func firstKeyword(b *model.Bookmark) string {
	if b != nil && len(b.Keywords) > 0 {
		return b.Keywords[0]
	}
	return ""
}

func previewEngineURL(rawURL, query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return rawURL
	}
	return strings.ReplaceAll(rawURL, "{query}", q)
}

type modeSearchStyle struct {
	fieldBg   ui.Color
	darkNotch bool
	width     float32
}

// renderModeSearchBox renders the compact search input (with optional locked engine capsule)
// shared by tray, edge, and notch modes.
func renderModeSearchBox(
	c *ui.Context,
	a *app.App,
	p *theme.Palette,
	keyPrefix string,
	st *modeSearchState,
	handle *ui.Handle,
	style modeSearchStyle,
	onStateChange func(),
) ui.Element {
	lib := a.Store.Lib()
	locked := templateBookmark(lib, st.lockedID)
	if st.lockedID != "" && locked == nil {
		st.unlockKeepQuery()
	}

	focused := handle.Focused(c)
	hasQ := st.query != ""
	padLeft := float32(10)
	if locked != nil {
		padLeft = 5
	}
	sbox := ui.Row(c.Key(keyPrefix+"-sbox")).Height(30).Padding(0, 6, 0, padLeft).Gap(6).Radius(9).
		AlignItems(ui.Center)
	if style.width > 0 {
		sbox.Width(style.width).Shrink(0)
	} else {
		sbox.Grow(1).MinWidth(0)
	}

	var textCol, placeholderCol, iconCol, clrHoverBg, clrHoverFg, clrFg ui.Color
	var capsuleBg, capsuleHoverBg, capsuleFg ui.Color
	iconSize := float32(14)
	if style.darkNotch {
		iconSize = 13
		// The notch input is focused whenever the panel opens; keep the
		// accent for typed or locked state so it doesn't glare at rest.
		switch {
		case hasQ || locked != nil:
			sbox.Background(ui.RGBA(255, 255, 255, 0.14)).Border(1, ui.Hex("#e0875f").Alpha(0.85))
		case focused:
			sbox.Background(ui.RGBA(255, 255, 255, 0.12)).Border(1, ui.RGBA(255, 255, 255, 0.22))
		default:
			sbox.Background(ui.RGBA(255, 255, 255, 0.09)).Border(1, ui.RGBA(255, 255, 255, 0.08))
		}
		textCol = ui.Hex("#ffffff")
		placeholderCol = ui.RGBA(234, 232, 225, 0.60)
		iconCol = ui.RGBA(234, 232, 225, 0.65)
		clrFg = ui.RGBA(234, 232, 225, 0.70)
		clrHoverBg = ui.RGBA(255, 255, 255, 0.14)
		clrHoverFg = ui.Hex("#ffffff")
		capsuleBg = ui.RGBA(224, 135, 95, 0.24)
		capsuleHoverBg = ui.RGBA(224, 135, 95, 0.34)
		capsuleFg = ui.Hex("#f0a07c")
	} else {
		if focused || hasQ || locked != nil {
			sbox.Background(p.Surface).Border(1.5, p.Accent)
		} else {
			sbox.Background(style.fieldBg).Border(1.5, ui.Transparent)
		}
		textCol = p.Text
		placeholderCol = p.Muted
		iconCol = p.Muted
		clrFg = p.Muted
		clrHoverBg = p.RowSel
		clrHoverFg = p.Text
		capsuleBg = p.AccentBg
		capsuleHoverBg = p.AccentBg.Mix(p.Accent, 0.18)
		capsuleFg = p.AccentText
	}

	var in ui.Element
	sbox.Children(func() {
		if locked != nil {
			pill := ui.ButtonBase(c.Key(keyPrefix+"-lock")).Height(22).Padding(0, 6, 0, 5).Gap(4).Radius(6).
				AlignItems(ui.Center).Shrink(0).Background(capsuleBg).TextColor(capsuleFg).
				Label(locales.EngineUnlock()).Tooltip(locales.EngineUnlock())
			if pill.Hovered() {
				pill.Background(capsuleHoverBg)
			}
			maxTitleW := float32(76)
			if style.darkNotch {
				maxTitleW = 64
			}
			pill.Children(func() {
				theme.Tile(c, locked.Icon, a.Store.IconPath(locked.Icon), 14).Radius(4)
				ui.Text(c, locked.Title).FontSize(11.5).FontWeight(500).MaxWidth(maxTitleW).SingleLine()
				ui.Icon(c, theme.IconClose).FontSize(10).Shrink(0)
			})
			if pill.Clicked() {
				st.unlockKeepQuery()
				handle.Focus(c)
				if onStateChange != nil {
					onStateChange()
				}
			}
		} else {
			ui.Icon(c, theme.IconSearch).FontSize(iconSize).TextColor(iconCol).Shrink(0)
		}

		placeholder := locales.MainSearchPlaceholder()
		if locked != nil {
			placeholder = locales.EngineLockedPlaceholder(locked.Title)
		}
		in = ui.TextInputBase(c.Key(keyPrefix+"-q"), &st.query).Bind(handle).Grow(1).MinWidth(0).
			Label(placeholder).Placeholder(placeholder).
			TextColor(textCol).PlaceholderColor(placeholderCol)
		if st.caretPending {
			in.SetTextSelection(st.pendingCaret, st.pendingCaret)
			st.caretPending = false
		}
		in.OnTextKey(func(k ui.TextKey) bool {
			if st.lockedID != "" && k.Key == ui.KeyBackspace && k.Mods == 0 && k.Text() == "" {
				kw := st.lockedKW
				st.unlockKeepQuery()
				st.query = kw
				st.caretPending = true
				st.pendingCaret = utf8.RuneCountInString(kw)
				k.SetText(kw)
				if onStateChange != nil {
					onStateChange()
				}
				return true
			}
			if st.lockedID == "" && k.Key == ui.KeyTab && k.Mods == 0 && st.selTemplateID != "" {
				if b := templateBookmark(a.Store.Lib(), st.selTemplateID); b != nil {
					st.lockEngine(b, "", "")
					k.SetText("")
					if onStateChange != nil {
						onStateChange()
					}
					return true
				}
			}
			return false
		})
		if in.Changed() {
			st.selIdx = 0
			if st.lockedID == "" {
				if b, rest, ok := a.ParseLock(st.query); ok && b != nil {
					kw := ""
					if sp := strings.IndexByte(st.query, ' '); sp > 0 {
						kw = st.query[:sp]
					}
					st.lockEngine(b, kw, rest)
					in.ReplaceText(0, math.MaxInt, rest)
				}
			}
			if onStateChange != nil {
				onStateChange()
			}
		}
		if st.query != "" {
			clr := ui.ButtonBase(c.Key(keyPrefix+"-qclr")).Size(18, 18).Radius(5).Center().
				TextColor(clrFg).Label(locales.ActionClearSearch()).Tooltip(locales.ActionClearSearch()).Shrink(0)
			if clr.Hovered() {
				clr.Background(clrHoverBg).TextColor(clrHoverFg)
			}
			clr.Children(func() { ui.Icon(c, theme.IconClose).FontSize(11) })
			if clr.Clicked() {
				st.query = ""
				st.selIdx = 0
				if onStateChange != nil {
					onStateChange()
				}
				handle.Focus(c)
			}
		}
	})
	return in
}

// handleModeSearchNav processes Up/Down/Enter for locked mode or bookmark+engine search results.
func handleModeSearchNav(
	a *app.App,
	lib *model.Library,
	st *modeSearchState,
	in ui.Element,
	hits []*model.Bookmark,
	engines []*model.Bookmark,
	onStateChange func(),
) {
	if locked := templateBookmark(lib, st.lockedID); locked != nil {
		st.selTemplateID = ""
		if in.Submitted() {
			a.Open(locked, st.trimmedQuery())
		}
		return
	}
	q := st.trimmedQuery()
	if q == "" {
		st.selIdx = 0
		st.selTemplateID = ""
		return
	}
	total := len(hits) + len(engines)
	if total == 0 {
		st.selIdx = 0
		st.selTemplateID = ""
		return
	}
	st.selIdx = max(0, min(st.selIdx, total-1))
	if in.Shortcut(0, ui.KeyDown) {
		st.selIdx = min(st.selIdx+1, total-1)
		st.scrollMoved = true
	}
	if in.Shortcut(0, ui.KeyUp) {
		st.selIdx = max(st.selIdx-1, 0)
		st.scrollMoved = true
	}
	if st.selIdx < len(hits) && hits[st.selIdx].IsTemplate() {
		st.selTemplateID = hits[st.selIdx].ID
	} else {
		st.selTemplateID = ""
	}
	if in.Submitted() {
		if st.selIdx < len(hits) {
			b := hits[st.selIdx]
			if b.IsTemplate() {
				st.lockEngine(b, "", "")
				in.ReplaceText(0, math.MaxInt, "")
				if onStateChange != nil {
					onStateChange()
				}
				return
			}
			a.Open(b, "")
			return
		}
		eng := engines[st.selIdx-len(hits)]
		a.Open(eng, q)
	}
}

func renderEngineSectionHeader(c *ui.Context, p *theme.Palette, key, query string, top float32, darkNotch bool) {
	title := locales.EngineSearchSection(query)
	if darkNotch {
		ui.Box(c.Key(key)).Padding(top, 2, 2, 4).Children(func() {
			ui.Text(c, title).Font(theme.TitleFont(c)).FontSize(12.5).FontWeight(500).
				TextColor(ui.RGBA(234, 232, 225, 0.72)).SingleLine()
		})
		return
	}
	ui.Box(c.Key(key)).Padding(top, 0, 3, 8).Children(func() {
		widget.Title(c, p, title, 13)
	})
}

func renderEngineRow(
	c *ui.Context,
	a *app.App,
	p *theme.Palette,
	key string,
	eng *model.Bookmark,
	query string,
	height float32,
	selected bool,
	darkNotch bool,
	onOpen func(),
) ui.Element {
	row := ui.ButtonBase(c.Key(key)).Height(height).PaddingX(8).Gap(10).
		Radius(8).Justify(ui.Start).AlignItems(ui.Center).
		Label(query + " " + locales.EngineRowWith(eng.Title)).Tooltip(locales.EngineLockedTitle(eng.Title, query))
	var fg, muted, kwBg ui.Color
	if darkNotch {
		fg = ui.Hex("#e8e6df")
		muted = ui.RGBA(234, 232, 225, 0.65)
		kwBg = ui.RGBA(255, 255, 255, 0.10)
		if selected {
			row.Background(ui.RGBA(255, 255, 255, 0.16))
		} else if row.Hovered() {
			row.Background(ui.RGBA(255, 255, 255, 0.08))
		}
	} else {
		fg = p.Text
		muted = p.Muted
		kwBg = p.Field
		if selected {
			row.Background(p.RowSel)
			kwBg = p.Text.Alpha(0.07)
		} else if row.Hovered() {
			row.Background(p.Hover)
		}
	}
	row.Children(func() {
		theme.Tile(c, eng.Icon, a.Store.IconPath(eng.Icon), 22).Radius(6)
		ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, query).FontWeight(500).TextColor(fg).SingleLine().Shrink(1).MinWidth(0)
			ui.Text(c, locales.EngineRowWith(eng.Title)).FontSize(12).TextColor(muted).
				MaxWidth(130).SingleLine().Shrink(1)
		})
		if kw := firstKeyword(eng); kw != "" {
			ui.Row(c).Height(20).PaddingX(6).Radius(5).Background(kwBg).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, kw).FontSize(11).TextColor(muted).SingleLine()
			})
		}
	})
	if row.Clicked() && onOpen != nil {
		onOpen()
	}
	return row
}

func renderLockedEngineRow(
	c *ui.Context,
	a *app.App,
	p *theme.Palette,
	key string,
	eng *model.Bookmark,
	query string,
	height float32,
	darkNotch bool,
	onOpen func(),
) {
	title := locales.EngineLockedTitle(eng.Title, query)
	preview := previewEngineURL(eng.URL, query)
	row := ui.ButtonBase(c.Key(key)).FillWidth().Height(height).PaddingX(8).Gap(10).
		Radius(8).Justify(ui.Start).AlignItems(ui.Center).Label(title).Tooltip(preview)
	var fg, muted ui.Color
	if darkNotch {
		fg = ui.Hex("#ffffff")
		muted = ui.RGBA(234, 232, 225, 0.65)
		if row.Hovered() {
			row.Background(ui.RGBA(255, 255, 255, 0.18))
		} else {
			row.Background(ui.RGBA(255, 255, 255, 0.12))
		}
	} else {
		fg = p.Text
		muted = p.Muted
		if row.Hovered() {
			row.Background(p.RowSel.Mix(p.Hover, 0.4))
		} else {
			row.Background(p.RowSel)
		}
	}
	row.Children(func() {
		theme.Tile(c, eng.Icon, a.Store.IconPath(eng.Icon), 22).Radius(6)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontWeight(500).TextColor(fg).SingleLine()
			ui.Text(c, preview).FontSize(11.5).TextColor(muted).SingleLine()
		})
		ui.Text(c, "↵").FontSize(11.5).TextColor(muted).Shrink(0)
	})
	if row.Clicked() && onOpen != nil {
		onOpen()
	}
}

func handleModeEscape(
	st *modeSearchState,
	searchH *ui.Handle,
	c *ui.Context,
	onStateChange func(),
	onEmptyEsc func(),
	invalidate func(),
) {
	if st.lockedID != "" {
		st.unlockKeepQuery()
		if onStateChange != nil {
			onStateChange()
		}
		searchH.Focus(c)
		if invalidate != nil {
			invalidate()
		}
		return
	}
	if st.query != "" {
		st.reset()
		if onStateChange != nil {
			onStateChange()
		}
		searchH.Focus(c)
		if invalidate != nil {
			invalidate()
		}
		return
	}
	if onEmptyEsc != nil {
		onEmptyEsc()
	}
}

// trayIconBytes loads resources/trayTemplate.png (macOS) or resources/tray.png,
// falling back to an identical generated 32x32 bookmark PNG.
func trayIconBytes() []byte {
	name := "tray.png"
	col := color.NRGBA{R: 217, G: 119, B: 87, A: 255}
	if runtime.GOOS == "darwin" {
		name = "trayTemplate.png"
		col = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	}
	if dir, err := mygo.App.Path(mygo.PathResources); err == nil && dir != "" {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil && len(data) > 0 {
			return data
		}
	}
	return drawTrayBookmarkPNG(col)
}

func drawTrayBookmarkPNG(col color.NRGBA) []byte {
	const size = 32
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const (
		left   = 8.5
		right  = 23.5
		top    = 4.5
		bottom = 27.5
		midX   = 16.0
		notchY = 21.2
		rad    = 2.6
	)
	inside := func(x, y float64) bool {
		if x < left || x > right || y < top || y > bottom {
			return false
		}
		if x < left+rad && y < top+rad {
			dx, dy := (left+rad)-x, (top+rad)-y
			if dx*dx+dy*dy > rad*rad {
				return false
			}
		}
		if x > right-rad && y < top+rad {
			dx, dy := x-(right-rad), (top+rad)-y
			if dx*dx+dy*dy > rad*rad {
				return false
			}
		}
		t := math.Abs(x-midX) / (midX - left)
		if y > notchY+(bottom-notchY)*t {
			return false
		}
		return true
	}
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			hits := 0
			const sub = 4
			for sy := 0; sy < sub; sy++ {
				for sx := 0; sx < sub; sx++ {
					x := float64(px) + (float64(sx)+0.5)/sub
					y := float64(py) + (float64(sy)+0.5)/sub
					if inside(x, y) {
						hits++
					}
				}
			}
			if hits > 0 {
				a := uint8(math.Round(float64(col.A) * float64(hits) / float64(sub*sub)))
				img.SetNRGBA(px, py, color.NRGBA{R: col.R, G: col.G, B: col.B, A: a})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
