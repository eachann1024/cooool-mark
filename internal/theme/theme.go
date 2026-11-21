// Package theme is Cooool Mark's look: the warm paper palette with a coral
// accent (light) and its dark twin, the serif used for titles, the stroke
// icons, and the bookmark tile every view shows.
package theme

import (
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/model"
)

// Palette are the app's colors. Views use these, never raw hexes.
type Palette struct {
	Dark       bool
	Bg         ui.Color // content
	Sidebar    ui.Color
	Line       ui.Color // hairlines
	LineSoft   ui.Color
	Hover      ui.Color
	RowSel     ui.Color // selected row
	SideSel    ui.Color // selected sidebar item
	Field      ui.Color // search field, secondary buttons
	Surface    ui.Color // dialogs, popovers, panels
	Overlay    ui.Color
	Text       ui.Color
	Text2      ui.Color // headings in sections
	Muted      ui.Color
	Accent     ui.Color // bars, focus rings, drop lines
	AccentBg   ui.Color // selected pill
	AccentText ui.Color // text on AccentBg
	Primary    ui.Color // primary button
	OnPrimary  ui.Color
	Danger     ui.Color
	OnDanger   ui.Color
	DangerBg   ui.Color
	ToastBg    ui.Color
	ToastText  ui.Color
	Notch      ui.Color // the notch panel's black
}

// Light is the paper palette.
var Light = Palette{
	Bg: ui.Hex("#faf9f5"), Sidebar: ui.Hex("#f3f1ea"), Line: ui.Hex("#ebe8df"), LineSoft: ui.Hex("#f0ede6"),
	Hover: ui.Hex("#f3f1ea"), RowSel: ui.Hex("#efece4"), SideSel: ui.Hex("#e8e5db"), Field: ui.Hex("#f0eee6"),
	Surface: ui.Hex("#ffffff"), Overlay: ui.RGBA(38, 36, 32, 0.22),
	Text: ui.Hex("#1f1e1d"), Text2: ui.Hex("#3d3c38"), Muted: ui.Hex("#6c6b64"),
	Accent: ui.Hex("#d97757"), AccentBg: ui.Hex("#f3e7e0"), AccentText: ui.Hex("#9a4a2e"),
	Primary: ui.Hex("#b5583a"), OnPrimary: ui.Hex("#ffffff"), Danger: ui.Hex("#b54a3f"), OnDanger: ui.Hex("#ffffff"), DangerBg: ui.Hex("#fbefed"),
	ToastBg: ui.Hex("#1f1e1d"), ToastText: ui.Hex("#faf9f5"), Notch: ui.Hex("#0b0b0c"),
}

// Dark is the night palette.
var Dark = Palette{
	Dark: true,
	Bg:   ui.Hex("#0f0f10"), Sidebar: ui.Hex("#161617"), Line: ui.Hex("#262624"), LineSoft: ui.Hex("#1f1f1d"),
	Hover: ui.Hex("#1e1e1c"), RowSel: ui.Hex("#262624"), SideSel: ui.Hex("#262624"), Field: ui.Hex("#1e1e1c"),
	Surface: ui.Hex("#1c1c1b"), Overlay: ui.RGBA(0, 0, 0, 0.45),
	Text: ui.Hex("#eae8e1"), Text2: ui.Hex("#c7c5bd"), Muted: ui.Hex("#b7b5ad"),
	Accent: ui.Hex("#e0875f"), AccentBg: ui.Hex("#3a2a22"), AccentText: ui.Hex("#f0a07c"),
	Primary: ui.Hex("#e0875f"), OnPrimary: ui.Hex("#141312"), Danger: ui.Hex("#e5776b"), OnDanger: ui.Hex("#141312"), DangerBg: ui.Hex("#3a2422"),
	ToastBg: ui.Hex("#262624"), ToastText: ui.Hex("#eae8e1"), Notch: ui.Hex("#0b0b0c"),
}

// tsangerJinKai lists common installed family names of Tsanger JinKai
// (仓耳今楷) across platforms and weight variants before falling back to
// system serif fonts.
const tsangerJinKai = "仓耳今楷03W04, 仓耳今楷03-W04, 仓耳今楷03-W03, 仓耳今楷03-W05, 仓耳今楷02-W04, 仓耳今楷01-W04, 仓耳今楷04-W04, 仓耳今楷05-W04, 仓耳今楷, TsangerJinKai03-W04, TsangerJinKai02-W04, TsangerJinKai01-W04, TsangerJinKai04-W04, TsangerJinKai05-W04, TsangerJinKai"

// Serif is the family of titles (Chinese locale); Sans is empty to use the
// system's built-in UI font.
var Serif, Sans = fonts()

// LatinSerif is the family of titles for non-Chinese locales; LatinSans is
// empty to use the system's built-in UI font.
var LatinSerif, LatinSans = latinFonts()

func fonts() (string, string) {
	switch runtime.GOOS {
	case "darwin":
		return tsangerJinKai + ", Songti SC", ""
	case "windows":
		return tsangerJinKai + ", SimSun", ""
	}
	return tsangerJinKai + ", Noto Serif CJK SC", ""
}

func latinFonts() (string, string) {
	switch runtime.GOOS {
	case "darwin":
		return tsangerJinKai + ", New York", ""
	case "windows":
		return tsangerJinKai + ", Georgia", ""
	}
	return tsangerJinKai + ", Noto Serif", ""
}

// TitleFont returns the display font for titles matching the active locale.
func TitleFont(c *ui.Context) string {
	if c != nil && !strings.HasPrefix(strings.ToLower(c.Locale()), "zh") && LatinSerif != "" {
		return LatinSerif
	}
	return Serif
}

// ReduceTransparency reports whether the desktop requests opaque surfaces.
func ReduceTransparency() bool {
	if !mygo.App.IsReady() {
		return false
	}
	return mygo.Theme.Preferences().ReduceTransparency
}

// Preferences reads the desktop theme preferences when the app is running.
func Preferences() mygo.ThemePreferences {
	if !mygo.App.IsReady() {
		return mygo.ThemePreferences{TextScale: 1}
	}
	return mygo.Theme.Preferences()
}

// ScrollbarStyle gives scrollbars a thin thumb and a clear track.
func ScrollbarStyle(dark bool) ui.ScrollbarStyle {
	s := ui.ScrollbarStyle{
		ThumbWidth: 3, TrackWidth: 7, MinLength: 24, Track: ui.Transparent,
		Thumb: Light.Text.Alpha(0.14), Hover: Light.Text.Alpha(0.26), Active: Light.Text.Alpha(0.38),
	}
	if dark {
		s.Thumb = Dark.Text.Alpha(0.16)
		s.Hover = Dark.Text.Alpha(0.28)
		s.Active = Dark.Text.Alpha(0.42)
	}
	return s
}

// SlimScrollbarStyle fits horizontal scrollbars in compact tab strips.
func SlimScrollbarStyle(dark bool) ui.ScrollbarStyle {
	s := ScrollbarStyle(dark)
	s.ThumbWidth, s.TrackWidth, s.MinLength = 2, 4, 20
	return s
}

// Apply sets the app theme for this frame from the appearance setting
// ("auto" follows the system) and returns the palette to draw with.
func Apply(c *ui.Context, appearance string) *Palette {
	base := c.Theme()
	dark := base.Dark
	switch appearance {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	pal := Light
	if dark {
		pal = Dark
	}
	prefs := Preferences()
	if prefs.HighContrast {
		if dark {
			pal.Line = ui.Hex("#474744")
			pal.LineSoft = ui.Hex("#353532")
			pal.Text = ui.Hex("#ffffff")
			pal.Text2 = ui.Hex("#e3e1da")
			pal.Muted = ui.Hex("#cccac2")
			pal.Accent = ui.Hex("#f09b74")
			pal.AccentText = ui.Hex("#ffb694")
			pal.Primary = ui.Hex("#f09b74")
			pal.OnPrimary = ui.Hex("#0d0c0b")
			pal.Danger = ui.Hex("#ff8b7e")
			pal.OnDanger = ui.Hex("#0d0c0b")
		} else {
			pal.Line = ui.Hex("#bdb8ac")
			pal.LineSoft = ui.Hex("#d4cfc2")
			pal.Text = ui.Hex("#111110")
			pal.Text2 = ui.Hex("#262522")
			pal.Muted = ui.Hex("#4c4b45")
			pal.Accent = ui.Hex("#9a4427")
			pal.AccentText = ui.Hex("#78321b")
			pal.Primary = ui.Hex("#8f3f25")
			pal.OnPrimary = ui.Hex("#ffffff")
			pal.Danger = ui.Hex("#992f26")
			pal.OnDanger = ui.Hex("#ffffff")
		}
	}
	p := &pal
	t := *base
	if dark != base.Dark {
		if dark {
			t = *ui.DarkTheme()
		} else {
			t = *ui.LightTheme()
		}
	}
	t = *t.WithAccent(p.Primary)
	t.Dark = dark
	t.Background = p.Bg
	t.Surface = p.Field
	t.SurfaceHover = p.Hover.Mix(p.Line, 0.5)
	t.SurfacePressed = p.Line
	t.Border = p.Line
	t.Text = p.Text
	t.TextMuted = p.Muted
	t.AccentText = p.OnPrimary
	if dark {
		t.InactiveAccent = p.Muted.Mix(p.Primary, 0.25)
	} else {
		t.InactiveAccent = p.Muted.Mix(p.Primary, 0.22)
	}
	t.Danger = p.Danger
	t.Focus = p.Accent
	t.Selection = p.Accent.Alpha(0.28)
	t.Inverse, t.InverseText = p.ToastBg, p.ToastText
	t.Radius = 7
	t.Spacing = 3.5
	t.FontSize = 13
	sb := ScrollbarStyle(dark)
	t.ScrollbarWidth = sb.ThumbWidth
	t.Scrollbar = sb.Thumb
	t.ScrollbarStyle = sb
	t.Font = ""
	if strings.HasPrefix(strings.ToLower(c.Locale()), "zh") {
		if Sans != "" {
			t.Font = Sans
		}
	} else if LatinSans != "" {
		t.Font = LatinSans
	}
	c.SetTheme(&t)
	return p
}

// TileColors are the colors given to bookmarks without an icon.
var TileColors = []string{"#2b2a27", "#0a6f8e", "#5b4a8a", "#24292f", "#a3262b", "#b2441b", "#2f5d8a", "#7a4a1e", "#b4295e", "#1f5f99", "#2f6fd0", "#b8510f", "#b0302a", "#4f52b2", "#2f62d6", "#b8590f"}

// DefaultIcon picks a letter and a color for a title/URL.
func DefaultIcon(title, url string) model.Icon {
	s := strings.TrimSpace(title)
	if s == "" {
		s = strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
		s = strings.TrimPrefix(s, "www.")
	}
	letter := "?"
	for _, r := range s {
		letter = strings.ToUpper(string(r))
		break
	}
	h := 0
	for _, r := range url + title {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return model.Icon{Letter: letter, Color: TileColors[h%len(TileColors)]}
}

var (
	imgMu    sync.Mutex
	imgCache = map[string]ui.ImageSource{}
)

// LoadIcon decodes an icon file once (nil if missing or unreadable).
func LoadIcon(path string) ui.ImageSource {
	if path == "" {
		return nil
	}
	imgMu.Lock()
	defer imgMu.Unlock()
	if src, ok := imgCache[path]; ok {
		return src
	}
	var src ui.ImageSource
	if data, err := os.ReadFile(path); err == nil {
		src, _ = ui.DecodeImage(data)
	}
	imgCache[path] = src
	return src
}

// Tile draws a bookmark's icon: its image if iconPath loads, else the
// colored square with its letter. size is in DIPs (22 in rows, 30–46 in
// grids and panels).
func Tile(c *ui.Context, icon model.Icon, iconPath string, size float32) ui.Element {
	radius := size * 0.27
	if src := LoadIcon(iconPath); src != nil {
		// A white glyph on transparency vanishes on the light card and a
		// black one on the dark card: give those a contrasting plate.
		dark := c.Theme().Dark
		var plate ui.Color
		switch tone := LoadIconTone(iconPath); {
		case tone == ToneLight && !dark:
			plate = ui.Hex("#2b2a27")
		case tone == ToneDark && dark:
			plate = ui.Hex("#eae8e1")
		default:
			return ui.Image(c, src).Size(size, size).Radius(radius)
		}
		inner := size * 0.72
		box := ui.Row(c).Size(size, size).Radius(radius).Background(plate).Center()
		box.Children(func() {
			ui.Image(c, src).Size(inner, inner)
		})
		return box
	}
	color := icon.Color
	if color == "" {
		color = TileColors[0]
	}
	letter := icon.Letter
	if letter == "" {
		letter = "?"
	}
	box := ui.Row(c).Size(size, size).Radius(radius).Background(ui.Hex(color)).
		Center()
	box.Children(func() {
		ui.Text(c, letter).FontSize(size * 0.46).FontWeight(600).TextColor(ui.Hex("#ffffff")).SingleLine()
	})
	return box
}
