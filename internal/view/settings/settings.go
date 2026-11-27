// Package settings is the settings window.
package settings

import (
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

type pageMeta struct {
	id    string
	label func() string
	icon  *ui.SVG
}

var pages = []pageMeta{
	{id: "general", label: locales.SettingsPageGeneral, icon: theme.IconSettings},
	{id: "engines", label: locales.SettingsPageEngines, icon: theme.IconSearch},
	{id: "ai", label: locales.SettingsPageAI, icon: theme.IconSparkle},
	{id: "data", label: locales.SettingsPageData, icon: theme.IconCloud},
}

type state struct {
	a    *app.App
	win  *mygo.Window
	page string

	openAtLogin   bool
	recordingSlot string

	openAIKey      string
	openAIKeyReady bool

	saveTimer *time.Timer
	keyTimer  *time.Timer
	keySeq    int
}

var (
	cur *state

	// keySyncMu protects only the in-memory single-flight worker state (never
	// held across OS credential store I/O), preventing both flush-vs-goroutine
	// races and re-entrant quit deadlocks.
	keySyncMu     sync.Mutex
	keyWantValid  bool
	keyWantVal    string
	keyWantVer    uint64
	keyWorkerBusy bool
	keyWorkerDone chan struct{}
	keyLastErr    error
	keyLastErrVer uint64
	keyErrShown   bool
)

// RefreshChrome updates the settings window title and contents after locale changes.
func RefreshChrome() {
	if cur != nil && cur.win != nil && !cur.win.IsDestroyed() {
		cur.win.SetTitle(locales.SettingsTitle())
		cur.win.Invalidate()
	}
}

// Export opens a native save dialog to export bookmarks as "html" or "json".
func Export(a *app.App, format string) {
	if a == nil || a.Import == nil {
		return
	}
	var parent *mygo.Window
	if cur != nil && cur.win != nil && !cur.win.IsDestroyed() {
		parent = cur.win
	}
	name := "bookmarks.html"
	filter := mygo.FileFilter{Name: locales.DataExportHTMLFilter(), Extensions: []string{"html", "htm"}}
	title := locales.DataExportHTMLTitle()
	if format == "json" {
		name = "cooool-mark-backup.json"
		filter = mygo.FileFilter{Name: locales.DataExportJSONFilter(), Extensions: []string{"json"}}
		title = locales.DataExportJSONTitle()
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      parent,
			Title:       title,
			DefaultPath: name,
			Filters:     []mygo.FileFilter{filter},
		})
		a.Post(func() {
			if err != nil {
				a.SayErr(locales.DataExportFailed(err.Error()), "", nil)
				return
			}
			if path == "" {
				return
			}
			if err := a.Import.Export(a.Store.Lib(), path, format); err != nil {
				a.SayErr(locales.DataExportFailed(err.Error()), "", nil)
			} else {
				a.Say(locales.DataExportDone(filepath.Base(path)))
			}
		})
	}()
}

// Show opens the settings window at page. Main thread.
func Show(a *app.App, page string) {
	a.PrepareRegularWindow()
	page = normalizePage(page)
	if cur != nil && cur.win != nil && !cur.win.IsDestroyed() {
		cur.page = page
		cur.setRecording("")
		cur.openAtLogin = mygo.App.OpenAtLogin()
		if cur.win.IsMinimized() {
			cur.win.Restore()
		}
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
		Title:                locales.SettingsTitle(),
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
			return p.label()
		}
	}
	return locales.SettingsTitle()
}

func (s *state) loadAsync() {
	a := s.a
	go func() {
		raw, err := app.LoadOpenAIKey()
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

func (s *state) setRecording(slot string) {
	if s.recordingSlot == slot {
		return
	}
	wasRecording := s.recordingSlot != ""
	s.recordingSlot = slot
	nowRecording := slot != ""
	if wasRecording != nowRecording && s.a != nil && s.a.Win.SetShortcutsPaused != nil {
		s.a.Win.SetShortcutsPaused(nowRecording)
	}
}

func (s *state) cleanup() {
	s.setRecording("")
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
	val := s.openAIKey
	a := s.a
	s.keyTimer = time.AfterFunc(450*time.Millisecond, func() {
		a.Post(func() {
			if cur != s || s.keySeq != seq {
				return
			}
			s.keyTimer = nil
			enqueueKeyPersist(a, val)
		})
	})
}

func (s *state) flushKeySave() {
	if !s.openAIKeyReady || s.keyTimer == nil {
		return
	}
	s.keyTimer.Stop()
	s.keyTimer = nil
	s.keySeq++
	enqueueKeyPersist(s.a, s.openAIKey)
}

// Flush flushes any pending debounced settings edits and queues any pending
// API key change to the background worker without blocking the main thread
// (safe during OnBeforeQuit where nested event loops may run).
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
			enqueueKeyPersist(s.a, s.openAIKey)
		}
	}
}

// FlushBeforeQuit flushes pending settings/API key edits and waits for the
// background credential worker to finish writing the latest key. Because
// runKeyWorker executes off the main thread without pumping the native event
// loop, waiting here cannot trigger re-entrant quits. Returns an unsurfaced
// save error so OnBeforeQuit can keep windows open and inform the user.
func FlushBeforeQuit() error {
	Flush()
	keySyncMu.Lock()
	done := keyWorkerDone
	busy := keyWorkerBusy
	keySyncMu.Unlock()
	if busy && done != nil {
		<-done
	}
	keySyncMu.Lock()
	defer keySyncMu.Unlock()
	if keyLastErr != nil && !keyErrShown {
		keyErrShown = true
		return keyLastErr
	}
	return nil
}

// FlushWait flushes pending settings/API key edits and waits for the
// background credential worker to finish writing the latest key. Call from
// OnWillQuit after the quit sequence has committed (non-reentrant). When
// sessionEnding is true (OS logout/shutdown), the wait is bounded so shutdown
// is never hung by a credential prompt.
func FlushWait(sessionEnding bool) {
	Flush()
	keySyncMu.Lock()
	done := keyWorkerDone
	busy := keyWorkerBusy
	keySyncMu.Unlock()
	if !busy || done == nil {
		return
	}
	if sessionEnding {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		return
	}
	<-done
}

func enqueueKeyPersist(a *app.App, val string) {
	keySyncMu.Lock()
	keyWantValid = true
	keyWantVal = val
	keyWantVer++
	keyLastErr = nil
	keyErrShown = false
	start := !keyWorkerBusy
	if start {
		keyWorkerBusy = true
		keyWorkerDone = make(chan struct{})
	}
	keySyncMu.Unlock()
	if start {
		go runKeyWorker(a)
	}
}

func runKeyWorker(a *app.App) {
	for {
		keySyncMu.Lock()
		if !keyWantValid {
			keyWorkerBusy = false
			done := keyWorkerDone
			keyWorkerDone = nil
			keySyncMu.Unlock()
			if done != nil {
				close(done)
			}
			return
		}
		val := keyWantVal
		ver := keyWantVer
		keyWantValid = false
		keySyncMu.Unlock()

		err := app.SaveOpenAIKey(val)
		keySyncMu.Lock()
		if keyWantVer == ver {
			keyLastErr = err
			keyLastErrVer = ver
			if err == nil {
				keyErrShown = false
			}
		}
		keySyncMu.Unlock()
		if a == nil {
			continue
		}
		a.Post(func() {
			settingsActive := mygo.App.IsActive() && cur != nil && cur.win != nil && !cur.win.IsDestroyed() &&
				cur.win.IsVisible() && !cur.win.IsMinimized() && cur.win.IsFocused()
			keySyncMu.Lock()
			latest := keyWantVer
			if latest == ver && err != nil && keyLastErrVer == ver && settingsActive {
				keyErrShown = true
			}
			keySyncMu.Unlock()
			if latest != ver {
				return
			}
			if err != nil {
				if settingsActive || a.HasRegularWindow() {
					a.SayErr(locales.AiKeySaveErr(), locales.ActionRetry(), func() { enqueueKeyPersist(a, val) })
				}
				return
			}
			if a.AI != nil {
				a.AI.Configure(a.Settings)
			}
		})
	}
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
					TextColor(p.Text2).Cursor(ui.CursorPointer).Label(locales.ActionBackToBookmarks()).Tooltip(locales.ActionBackToBookmarks())
				if back.Hovered() {
					back.Background(p.SideSel)
				}
				back.Children(func() {
					ui.Icon(c, theme.IconBack).FontSize(14)
					ui.Text(c, locales.BookmarksTitle()).FontSize(13).SingleLine()
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
						txt := ui.Text(c, pg.label()).FontSize(13).TextColor(textColor).Grow(1).SingleLine()
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
						s.setRecording("")
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
				case "engines":
					s.pageEngines(c, p)
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
