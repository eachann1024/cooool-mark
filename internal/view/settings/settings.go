// Package settings is the settings window.
package settings

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

const openAISecretKey = "openai-key"

type pageMeta struct {
	id    string
	label string
	icon  *ui.SVG
}

var pages = []pageMeta{
	{id: "general", label: "通用", icon: theme.IconSettings},
	{id: "ai", label: "AI 润色", icon: theme.IconSparkle},
	{id: "data", label: "存储与数据", icon: theme.IconCloud},
}

type state struct {
	a    *app.App
	win  *mygo.Window
	page string

	openAtLogin bool

	openAIKey      string
	openAIKeyReady bool

	saveTimer *time.Timer
	keyTimer  *time.Timer
	keySeq    int
}

var cur *state
var keyWriteVersion atomic.Uint64
var keyWriteMu sync.Mutex

// Show opens the settings window at page. Main thread.
func Show(a *app.App, page string) {
	a.PrepareRegularWindow()
	page = normalizePage(page)
	if cur != nil && cur.win != nil {
		cur.page = page
		cur.openAtLogin = mygo.App.OpenAtLogin()
		cur.win.Show()
		cur.win.Focus()
		cur.win.Invalidate()
		return
	}

	s := &state{
		a:           a,
		page:        page,
		openAtLogin: mygo.App.OpenAtLogin(),
	}
	cur = s
	s.loadAsync()

	w := mygo.NewWindow(mygo.WindowOptions{
		Title:                "设置",
		Width:                840,
		Height:               600,
		MinWidth:             720,
		MinHeight:            480,
		StateKey:             "settings",
		TitleBarStyle:        mygo.TitleBarHiddenInset,
		TrafficLightPosition: widget.Lights(),
		TitleBarHeight:       widget.HeaderH,
		Content:              ui.View(s.view),
	})
	s.win = w
	a.AddRegularWindow(w)
	w.OnClosed(func() {
		s.cleanup()
		if cur == s {
			cur = nil
		}
	})
}

func normalizePage(page string) string {
	if page == "storage" {
		return "data"
	}
	for _, p := range pages {
		if p.id == page {
			return page
		}
	}
	return "general"
}

func pageTitle(id string) string {
	for _, p := range pages {
		if p.id == id {
			return p.label
		}
	}
	return "设置"
}

func (s *state) loadAsync() {
	a := s.a
	go func() {
		raw, err := mygo.Secrets.Get(openAISecretKey)
		a.Post(func() {
			if cur != s {
				return
			}
			if err == nil {
				s.openAIKey = string(raw)
			}
			s.openAIKeyReady = true
		})
	}()
}

func (s *state) cleanup() {
	if s.saveTimer != nil {
		s.saveTimer.Stop()
		s.saveTimer = nil
		s.a.SaveSettings()
	}
	if s.keyTimer != nil {
		s.flushKeySave()
	}
}

func (s *state) saveNow() {
	if s.saveTimer != nil {
		s.saveTimer.Stop()
		s.saveTimer = nil
	}
	s.a.SaveSettings()
}

func (s *state) onOpenAIChanged() {
	if s.a.AI != nil {
		s.a.AI.Configure(s.a.Settings)
	}
	s.scheduleSave()
}

func (s *state) scheduleSave() {
	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	a := s.a
	s.saveTimer = time.AfterFunc(400*time.Millisecond, func() {
		a.Post(func() {
			if cur != s {
				return
			}
			s.saveTimer = nil
			a.SaveSettings()
		})
	})
}

func (s *state) flushSave() {
	s.saveNow()
}

func (s *state) scheduleKeySave() {
	if !s.openAIKeyReady {
		return
	}
	if s.keyTimer != nil {
		s.keyTimer.Stop()
	}
	s.keySeq++
	seq := s.keySeq
	version := keyWriteVersion.Add(1)
	val := s.openAIKey
	a := s.a
	s.keyTimer = time.AfterFunc(450*time.Millisecond, func() {
		a.Post(func() {
			if cur != s || s.keySeq != seq {
				return
			}
			s.keyTimer = nil
			go s.persistOpenAIKey(val, version)
		})
	})
}

func (s *state) flushKeySave() {
	if !s.openAIKeyReady || s.keyTimer == nil {
		return
	}
	if s.keyTimer != nil {
		s.keyTimer.Stop()
		s.keyTimer = nil
	}
	s.keySeq++
	val := s.openAIKey
	version := keyWriteVersion.Add(1)
	go s.persistOpenAIKey(val, version)
}

// Flush synchronously persists any debounced settings or API key edits
// before the app quits. Main thread.
func Flush() {
	if s := cur; s != nil {
		if s.saveTimer != nil {
			s.saveTimer.Stop()
			s.saveTimer = nil
			s.a.SaveSettings()
		}
		if s.openAIKeyReady && s.keyTimer != nil {
			s.keyTimer.Stop()
			s.keyTimer = nil
			s.keySeq++
			val := s.openAIKey
			version := keyWriteVersion.Add(1)
			_ = writeOpenAIKeySync(val, version)
		}
	}
	// Wait for any in-flight background keychain write to finish before exit.
	keyWriteMu.Lock()
	keyWriteMu.Unlock()
}

func writeOpenAIKeySync(val string, version uint64) error {
	keyWriteMu.Lock()
	defer keyWriteMu.Unlock()
	if keyWriteVersion.Load() != version {
		return nil
	}
	if val == "" {
		return mygo.Secrets.Delete(openAISecretKey)
	}
	return mygo.Secrets.Set(openAISecretKey, []byte(val))
}

func (s *state) persistOpenAIKey(val string, version uint64) {
	keyWriteMu.Lock()
	if keyWriteVersion.Load() != version {
		keyWriteMu.Unlock()
		return
	}
	var err error
	if val == "" {
		err = mygo.Secrets.Delete(openAISecretKey)
	} else {
		err = mygo.Secrets.Set(openAISecretKey, []byte(val))
	}
	keyWriteMu.Unlock()
	a := s.a
	a.Post(func() {
		if keyWriteVersion.Load() != version {
			return
		}
		if err != nil {
			a.SayErr("无法保存 API Key", "重试", func() { go s.persistOpenAIKey(val, version) })
			return
		}
		if a.AI != nil {
			a.AI.Configure(a.Settings)
		}
	})
}

func (s *state) view(c *ui.Context) {
	p := theme.Apply(c, s.a.Settings.Appearance)
	bar := c.TitleBar()

	ui.Box(c).Fill().Background(p.Bg).Children(func() {
		ui.Row(c).Fill().Children(func() {
			s.sidebar(c, p, bar)
			ui.Box(c).Width(1).FillHeight().Background(p.Line)
			s.content(c, p, bar)
		})
		widget.Toast(c, s.a, p, 16)
	})
}

func (s *state) sidebar(c *ui.Context, p *theme.Palette, bar ui.TitleBar) {
	ui.Column(c).Width(204).FillHeight().Background(p.Sidebar).Shrink(0).Children(func() {
		headH := widget.BarH(bar)
		ui.Row(c).Height(headH).FillWidth().Padding(0, 12, 0, max(bar.Left, 12)).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Spacer(c)
			if s.a.Win.ShowMain != nil {
				back := ui.ButtonBase(c).Height(26).Padding(0, 8, 0, 4).Radius(7).AlignItems(ui.Center).Gap(3).
					TextColor(p.Text2).Cursor(ui.CursorPointer).Label("返回书签").Tooltip("返回书签")
				if back.Hovered() {
					back.Background(p.SideSel)
				}
				back.Children(func() {
					ui.Icon(c, theme.IconBack).FontSize(14)
					ui.Text(c, "书签").FontSize(13).SingleLine()
				})
				if back.Clicked() {
					s.a.Win.ShowMain()
				}
			}
		})

		nav := widget.AutoScrollbar(c, ui.Scroll(c)).Grow(1).MinHeight(0).FillWidth().Padding(6, 10)
		nav.Children(func() {
			ui.Column(c).FillWidth().Gap(2).FocusGroup(ui.Vertical).Children(func() {
				for _, pg := range pages {
					on := s.page == pg.id
					row := ui.ButtonBase(c.Key("nav-"+pg.id)).Row().Height(32).FillWidth().
						Padding(0, 10, 0, 14).Radius(7).AlignItems(ui.Center).Gap(8).Cursor(ui.CursorPointer)
					if on {
						row.Background(p.SideSel)
					} else if row.Hovered() {
						row.Background(p.Hover.Mix(p.Line, 0.45))
					}
					row.Children(func() {
						if on {
							ui.Box(c).Absolute().Left(0).Top(9).Size(3, 14).Radius(2).Background(p.Accent)
						}
						iconColor := p.Muted
						textColor := p.Text2
						if on {
							iconColor = p.Accent
							textColor = p.Text
						}
						ui.Icon(c, pg.icon).FontSize(14).TextColor(iconColor).Shrink(0)
						txt := ui.Text(c, pg.label).FontSize(13).TextColor(textColor).Grow(1).SingleLine()
						if on {
							txt.FontWeight(500)
						}
						if pg.id == "ai" {
							if pct, ok := s.aiDownloadProgress(); ok {
								ui.Textf(c, "%d%%", pct).FontSize(11.5).TextColor(p.AccentText).SingleLine()
							}
						}
					})
					if row.Clicked() && s.page != pg.id {
						s.page = pg.id
					}
				}
			})
		})
	})
}

func (s *state) aiDownloadProgress() (int, bool) {
	if s.a.AI == nil {
		return 0, false
	}
	if st := s.a.AI.Status(); st.Downloading {
		return int(st.Progress * 100), true
	}
	for _, m := range localModels {
		if st := s.a.AI.LocalModelStatus(m.id); st.Downloading {
			return int(st.Progress * 100), true
		}
	}
	return 0, false
}

func (s *state) content(c *ui.Context, p *theme.Palette, bar ui.TitleBar) {
	ui.Column(c).Grow(1).FillHeight().Background(p.Bg).Children(func() {
		headH := widget.BarH(bar)
		padR := max(float32(24), bar.Right+16)
		ui.Row(c).Height(headH).FillWidth().Padding(0, padR, 0, 32).AlignItems(ui.Center).DragWindow().Children(func() {
			widget.Title(c, p, pageTitle(s.page), 16).Translate(0, -0.5)
		})
		ui.Box(c).Height(1).FillWidth().Background(p.Line)

		widget.AutoScrollbar(c, ui.Scroll(c.Key("page-"+s.page))).Grow(1).FillWidth().Padding(22, 32, 40, 32).Children(func() {
			ui.Column(c).MaxWidth(640).FillWidth().Gap(14).Children(func() {
				switch s.page {
				case "general":
					s.pageGeneral(c, p)
				case "ai":
					s.pageAI(c, p)
				case "data":
					s.pageData(c, p)
				}
			})
		})
	})
}

func card(c *ui.Context, p *theme.Palette, fn func()) ui.Element {
	return ui.Column(c).FillWidth().Radius(12).Background(p.Surface).Border(1, p.Line).Dividers(1, p.LineSoft).Children(fn)
}

func settingRow(c *ui.Context, p *theme.Palette, label string, right func()) ui.Element {
	return ui.Row(c).FillWidth().Padding(11, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(p.Text).Grow(1).SingleLine()
		right()
	})
}

func switchRow(c *ui.Context, p *theme.Palette, label string, on *bool, onChanged func()) {
	settingRow(c, p, label, func() {
		if ui.Switch(c, on).Label(label).Changed() && onChanged != nil {
			onChanged()
		}
	})
}

func segmented(c *ui.Context, p *theme.Palette, idx *int, label string, labels []string, fillEqually, disabled bool) ui.Element {
	parts := ui.SegmentedBase(c, idx, len(labels))
	track := parts.Track.AlignItems(ui.Stretch).Padding(2).Gap(2).Radius(8).Background(p.Field).Label(label).Disabled(disabled)
	track.Children(func() {
		for i, lbl := range labels {
			seg := parts.Segment(i).Height(26).PaddingX(11).Radius(6).Disabled(disabled)
			if !disabled {
				seg.Cursor(ui.CursorPointer)
			}
			if i == *idx {
				bg := p.Surface
				if p.Dark {
					bg = p.SideSel
				}
				seg.Background(bg).TextColor(p.Text).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
			} else {
				seg.TextColor(p.Muted)
				if !disabled && seg.Hovered() {
					seg.TextColor(p.Text)
				}
			}
			seg.Children(func() {
				t := ui.Text(c, lbl).FontSize(12).SingleLine()
				if i == *idx {
					t.FontWeight(500)
				}
			})
		}
	})
	if fillEqually {
		track.FillEqually()
	}
	return track
}

func segmentedRow(c *ui.Context, p *theme.Palette, label string, current string, labels []string, values []string, onPick func(val string)) {
	idx := max(0, slices.Index(values, current))
	settingRow(c, p, label, func() {
		if segmented(c, p, &idx, label, labels, false, false).Changed() && idx >= 0 && idx < len(values) {
			onPick(values[idx])
		}
	})
}

func btnSecondary(c *ui.Context, p *theme.Palette, label string) ui.Element {
	return smallButton(c, p, label, 28, 11, false)
}

func btnSecondaryCompact(c *ui.Context, p *theme.Palette, label string) ui.Element {
	return smallButton(c, p, label, 26, 10, false)
}

func btnPrimary(c *ui.Context, p *theme.Palette, label string) ui.Element {
	b := ui.ButtonBase(c).Height(28).PaddingX(11).Radius(7).AlignItems(ui.Center).
		Background(p.Primary).TextColor(p.OnPrimary).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(p.Primary.Mix(ui.Hex("#000000"), 0.08))
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(500).SingleLine() })
	return b
}

func btnDanger(c *ui.Context, p *theme.Palette, label string) ui.Element {
	return smallButton(c, p, label, 28, 9, true)
}

func smallButton(c *ui.Context, p *theme.Palette, label string, height, padX float32, danger bool) ui.Element {
	b := ui.ButtonBase(c).Height(height).PaddingX(padX).Radius(7).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if danger {
		b.TextColor(p.Danger)
		if b.Hovered() {
			b.Background(p.DangerBg)
		}
	} else {
		bg, hover := p.Field, p.Line
		if p.Dark {
			bg, hover = p.RowSel, p.SideSel
		}
		b.Background(bg).TextColor(p.Text)
		if b.Hovered() {
			b.Background(hover)
		}
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).SingleLine() })
	return b
}
