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

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
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

// renderTemplateBar renders an inline input bar when a template bookmark is clicked.
func renderTemplateBar(c *ui.Context, a *app.App, b *model.Bookmark, query *string, handle *ui.Handle, dark bool, onCancel func()) {
	bg := ui.Hex("#f0eee6")
	fg := ui.Hex("#1f1e1d")
	muted := ui.Hex("#6c6b64")
	accent := ui.Hex("#b5583a")
	if dark {
		bg = ui.RGBA(255, 255, 255, 0.12)
		fg = ui.Hex("#eae8e1")
		muted = ui.RGBA(234, 232, 225, 0.65)
		accent = ui.Hex("#e0875f")
	}
	row := ui.Row(c.Key("tmpl-bar")).Height(34).Padding(0, 8, 0, 10).Gap(8).Radius(9).Background(bg).AlignItems(ui.Center)
	row.Children(func() {
		ui.Text(c, b.Title).FontSize(12).FontWeight(500).TextColor(accent).MaxWidth(106).
			MinWidth(0).SingleLine().Tooltip(b.Title)
		in := ui.TextInputBase(c.Key("tmpl-in"), query).Bind(handle).Grow(1).MinWidth(80).
			FontSize(12).Label(locales.MainTemplateLabel()).Placeholder(locales.MainTemplatePlaceholder()).TextColor(fg).PlaceholderColor(muted)
		if in.Submitted() && a.Open(b, *query) {
			onCancel()
		}
		if in.Shortcut(0, ui.KeyEscape) {
			onCancel()
		}
		cancel := ui.ButtonBase(c.Key("tmpl-cancel")).PaddingX(6).Height(22).Radius(6).
			TextColor(muted).Label(locales.ActionCancel()).Tooltip(locales.ActionCancel()).Center().Shrink(0)
		cancel.Children(func() { ui.Text(c, locales.ActionCancel()).FontSize(11).SingleLine() })
		if cancel.Clicked() {
			onCancel()
		}
	})
}

// renderModeSearchBox renders the compact search input shared by summon mode panels.
func renderModeSearchBox(
	c *ui.Context,
	p *theme.Palette,
	keyPrefix string,
	query *string,
	handle *ui.Handle,
	fieldBg ui.Color,
	onScrollTop func(),
	onSubmit func(),
) {
	focused := handle.Focused(c)
	hasQ := *query != ""
	sbox := ui.Row(c.Key(keyPrefix+"-sbox")).Grow(1).MinWidth(0).Height(30).Padding(0, 6, 0, 10).Gap(6).Radius(9).
		AlignItems(ui.Center)
	if focused || hasQ {
		sbox.Background(p.Surface).Border(1.5, p.Accent)
	} else {
		sbox.Background(fieldBg).Border(1.5, ui.Transparent)
	}
	sbox.Children(func() {
		ui.Icon(c, theme.IconSearch).FontSize(14).TextColor(p.Muted).Shrink(0)
		in := ui.TextInputBase(c.Key(keyPrefix+"-q"), query).Bind(handle).Grow(1).MinWidth(0).
			Label(locales.MainSearchPlaceholder()).Placeholder(locales.MainSearchPlaceholder()).
			TextColor(p.Text).PlaceholderColor(p.Muted)
		if in.Changed() && onScrollTop != nil {
			onScrollTop()
		}
		if in.Submitted() && onSubmit != nil {
			onSubmit()
		}
		if hasQ {
			clr := ui.ButtonBase(c.Key(keyPrefix+"-qclr")).Size(18, 18).Radius(5).Center().
				TextColor(p.Muted).Label(locales.ActionClearSearch()).Tooltip(locales.ActionClearSearch()).Shrink(0)
			if clr.Hovered() {
				clr.Background(p.RowSel).TextColor(p.Text)
			}
			clr.Children(func() { ui.Icon(c, theme.IconClose).FontSize(11) })
			if clr.Clicked() {
				*query = ""
				if onScrollTop != nil {
					onScrollTop()
				}
				handle.Focus(c)
			}
		}
	})
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
