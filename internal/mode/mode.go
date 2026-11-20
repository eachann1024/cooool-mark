// Package mode runs the summon modes: notch, tray, screen edge, orb.
package mode

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
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

func titleFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host != "" {
		return host
	}
	return raw
}

// saveDroppedURL saves a dropped URL into groupID/subID and fetches page metadata asynchronously.
func saveDroppedURL(a *app.App, rawURL, groupID, subID string) {
	if a == nil || rawURL == "" {
		return
	}
	lib := a.Store.Lib()
	if existing := lib.FindURL(rawURL); existing != nil {
		a.Say("已在「" + a.PathOf(existing) + "」")
		return
	}
	g := activeGroup(lib, groupID)
	if g == nil {
		groupID = a.AddGroup("收藏")
	} else {
		groupID = g.ID
	}
	if lib.Sub(groupID, subID) == nil {
		subID = firstSubID(lib.Group(groupID))
		if subID == "" {
			subID = a.AddSub(groupID, "常用")
		}
	}
	fallbackTitle := titleFromURL(rawURL)
	id := a.SaveBookmark(model.Bookmark{
		Title:   fallbackTitle,
		URL:     rawURL,
		GroupID: groupID,
		SubID:   subID,
	})
	if a.Fetch == nil {
		return
	}
	dir := a.Store.Dir()
	app.Background(12*time.Second, func(ctx context.Context) {
		meta, err := a.Fetch.Fetch(ctx, rawURL)
		if err != nil {
			return
		}
		a.Post(func() {
			b := a.Store.Lib().Bookmark(id)
			if a.Store.Dir() != dir || b == nil || b.URL != rawURL {
				return
			}
			var iconRel string
			if len(meta.Icon) > 0 && b.Icon.Image == "" {
				ext := meta.IconExt
				if ext == "" {
					ext = ".png"
				}
				if path, err := a.Store.SaveIcon(meta.Icon, ext); err == nil {
					iconRel = path
				}
			}
			a.Store.Update(func(l *model.Library) {
				b := l.Bookmark(id)
				if b == nil {
					return
				}
				if meta.Title != "" && (b.Title == "" || b.Title == fallbackTitle || b.Title == b.URL) {
					b.Title = meta.Title
				}
				if meta.Description != "" && b.Note == "" {
					b.Note = meta.Description
				}
				if iconRel != "" && b.Icon.Image == "" {
					b.Icon.Image = iconRel
				}
				b.UpdatedAt = time.Now()
			})
		})
	})
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
		if strings.Contains(strings.ToLower(b.Title+" "+b.URL+" "+b.Note+" "+strings.Join(b.Tags, " ")), low) {
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
			FontSize(12).Label("模板参数").Placeholder("输入参数，回车打开").TextColor(fg).PlaceholderColor(muted)
		if in.Submitted() && a.Open(b, *query) {
			onCancel()
		}
		if in.Shortcut(0, ui.KeyEscape) {
			onCancel()
		}
		cancel := ui.ButtonBase(c.Key("tmpl-cancel")).PaddingX(6).Height(22).Radius(6).
			TextColor(muted).Label("取消").Tooltip("取消").Center().Shrink(0)
		cancel.Children(func() { ui.Text(c, "取消").FontSize(11).SingleLine() })
		if cancel.Clicked() {
			onCancel()
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
