package mode

import (
	"cmp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

const (
	quickSearchHotkey = "Alt+Space"
	quickSearchW      = 580
	quickSearchH      = 440
)

var (
	curQuick      *quickSearchMode
	quickApp      *app.App
	quickHotkeyOn bool
)

type quickSearchMode struct {
	app         *app.App
	win         *mygo.Window
	groupID     string
	query       string
	selIdx      int
	scrollMoved bool
	openedAt    time.Time
	searchInput ui.Handle
	tmplID      string
	tmplQuery   string
	tmplInput   ui.Handle
	list        ui.ListState
	cache       struct {
		library  *model.Library
		revision int64
		groupID  string
		query    string
		hits     []app.Hit
	}
}

func syncQuickSearch(a *app.App, on bool) {
	if !on {
		if quickHotkeyOn || mygo.GlobalShortcut.IsRegistered(quickSearchHotkey) {
			mygo.GlobalShortcut.Unregister(quickSearchHotkey)
		}
		quickHotkeyOn = false
		quickApp = nil
		if curQuick != nil {
			curQuick.destroy()
			curQuick = nil
		}
		return
	}
	quickApp = a
	if curQuick != nil {
		curQuick.app = a
		if curQuick.win != nil {
			curQuick.win.Invalidate()
		}
	}
	if !quickHotkeyOn && !mygo.GlobalShortcut.IsRegistered(quickSearchHotkey) {
		if err := mygo.GlobalShortcut.Register(quickSearchHotkey, func() {
			if quickApp != nil {
				ToggleQuickSearch(quickApp)
			}
		}); err == nil {
			quickHotkeyOn = true
		}
	}
}

// ToggleQuickSearch shows or hides the global quick search floating panel.
func ToggleQuickSearch(a *app.App) {
	if a == nil {
		return
	}
	if curQuick == nil {
		curQuick = newQuickSearchMode(a)
	} else {
		curQuick.app = a
	}
	if curQuick == nil || curQuick.win == nil {
		return
	}
	if curQuick.win.IsVisible() {
		curQuick.hide()
		return
	}
	curQuick.show()
}

func newQuickSearchMode(a *app.App) *quickSearchMode {
	q := &quickSearchMode{app: a}
	b := quickSearchBounds()
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:             "Cooool Mark 快速搜索",
		X:                 b.X,
		Y:                 b.Y,
		Width:             b.Width,
		Height:            b.Height,
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		Vibrancy:          mygo.VibrancyPopover,
		CornerRadius:      16,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		Panel:             true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Content:           ui.View(q.view),
	})
	q.win = w
	a.AddWindow(w)
	w.OnBlur(func() {
		if q.win != nil && q.win.IsVisible() && time.Since(q.openedAt) > 150*time.Millisecond {
			q.hide()
		}
	})
	w.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		q.hide()
	})
	return q
}

func quickSearchBounds() mygo.Rectangle {
	wa := displayRect(cursorDisplay())
	w := min(quickSearchW, max(320, wa.Width-32))
	h := min(quickSearchH, max(300, wa.Height-64))
	x := wa.X + (wa.Width-w)/2
	y := wa.Y + max(36, (wa.Height-h)/3)
	return mygo.Rectangle{X: x, Y: y, Width: w, Height: h}
}

func (q *quickSearchMode) show() {
	if q.win == nil {
		return
	}
	q.tmplID = ""
	q.tmplQuery = ""
	q.selIdx = 0
	q.list.ScrollTo(0, ui.Start)
	q.openedAt = time.Now()
	q.win.SetBounds(quickSearchBounds())
	q.win.Show()
	q.win.Focus()
	q.searchInput.SelectAll()
	q.win.Invalidate()
}

func (q *quickSearchMode) hide() {
	q.tmplID = ""
	q.tmplQuery = ""
	if q.win != nil && q.win.IsVisible() {
		q.win.Hide()
	}
}

func (q *quickSearchMode) destroy() {
	if q.win != nil {
		w := q.win
		q.win = nil
		w.Destroy()
	}
}

func (q *quickSearchMode) view(c *ui.Context) {
	p := theme.Apply(c, q.app.Settings.Appearance)
	c.Root().Background(ui.Transparent)

	lib := q.app.Store.Lib()
	if q.groupID != "" && lib.Group(q.groupID) == nil {
		q.groupID = ""
	}
	if q.tmplID != "" && templateBookmark(lib, q.tmplID) == nil {
		q.tmplID = ""
		q.tmplQuery = ""
	}

	raw := strings.TrimSpace(q.query)
	isURL := raw != "" && widget.LooksLikeURL(raw)
	hits := q.currentHits(lib, raw)
	offset := 0
	if isURL {
		offset = 1
	}
	total := offset + len(hits)
	if total == 0 {
		q.selIdx = 0
	} else {
		q.selIdx = max(0, min(q.selIdx, total-1))
	}

	if c.Shortcut(ui.Cmd, ui.KeyF) && !c.ShortcutRepeated() {
		q.searchInput.SelectAll(c)
	}

	if c.Shortcut(0, ui.KeyEscape) {
		if q.tmplID != "" {
			q.tmplID = ""
			q.tmplQuery = ""
			q.searchInput.Focus(c)
		} else if q.query != "" {
			q.query = ""
			q.selIdx = 0
			q.list.ScrollTo(0, ui.Start)
			q.searchInput.Focus(c)
		} else {
			q.hide()
			return
		}
	}

	if q.tmplID == "" && total > 0 {
		if c.Shortcut(0, ui.KeyDown) {
			q.selIdx = min(q.selIdx+1, total-1)
			q.scrollMoved = true
		}
		if c.Shortcut(0, ui.KeyUp) {
			q.selIdx = max(q.selIdx-1, 0)
			q.scrollMoved = true
		}
	}

	svc := c.Services()
	if !q.searchInput.Focused(c) && !q.tmplInput.Focused(c) && c.Shortcut(ui.Cmd, ui.KeyC) && !c.ShortcutRepeated() {
		if b := quickSelectedBookmark(hits, offset, q.selIdx); b != nil && b.URL != "" {
			svc.WriteClipboard(b.URL)
			q.app.Say("已复制链接")
		}
	}
	if q.tmplID == "" && !q.searchInput.Focused(c) && c.Shortcut(0, ui.KeyEnter) && !c.ShortcutRepeated() && total > 0 {
		q.activateIndex(c, lib, raw, isURL, hits, offset, q.selIdx)
	}

	box := ui.Column(c).Fill()
	if c.Vibrancy() {
		alpha := float32(0.96)
		if p.Dark {
			alpha = 0.94
		}
		box.Background(p.Bg.Alpha(alpha)).Radius(16).Border(1, p.Line)
	} else {
		box.Background(p.Bg).Radius(16).Border(1, p.Line)
	}

	box.Children(func() {
		// Top large search bar + clear + jump to main window.
		hdr := ui.Row(c.Key("qs-hdr")).Height(56).Padding(0, 14).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.LineSoft).DragWindow()
		hdr.Children(func() {
			iconCol := p.Muted
			if q.searchInput.Focused(c) || q.query != "" {
				iconCol = p.Accent
			}
			ui.Icon(c, theme.IconSearch).FontSize(18).TextColor(iconCol).Shrink(0)

			in := ui.TextInputBase(c.Key("qs-q"), &q.query).Bind(&q.searchInput).Grow(1).MinWidth(0).
				FontSize(16).Label("搜索书签").Placeholder("搜索书签或输入网址…").
				TextColor(p.Text).PlaceholderColor(p.Muted)

			start, end := in.TextSelection()
			hasSel := start != end
			in.OnTextKey(func(k ui.TextKey) bool {
				if k.Key == ui.KeyC && k.Mods == ui.Cmd && !hasSel {
					if b := quickSelectedBookmark(hits, offset, q.selIdx); b != nil && b.URL != "" {
						svc.WriteClipboard(b.URL)
						q.app.Say("已复制链接")
						return true
					}
				}
				return false
			})

			if in.Changed() {
				q.tmplID = ""
				q.tmplQuery = ""
				q.selIdx = 0
				q.list.ScrollTo(0, ui.Start)
			}
			if in.Submitted() && q.tmplID == "" && total > 0 {
				q.activateIndex(c, lib, raw, isURL, hits, offset, q.selIdx)
			}

			if q.query != "" {
				clr := ui.ButtonBase(c.Key("qs-qclr")).Size(22, 22).Radius(6).Center().
					TextColor(p.Muted).Label("清空搜索").Tooltip("清空搜索").Shrink(0)
				if clr.Hovered() {
					clr.Background(p.RowSel).TextColor(p.Text)
				}
				clr.Children(func() { ui.Icon(c, theme.IconClose).FontSize(12) })
				if clr.Clicked() {
					q.query = ""
					q.tmplID = ""
					q.tmplQuery = ""
					q.selIdx = 0
					q.list.ScrollTo(0, ui.Start)
					q.searchInput.Focus(c)
				}
			}

			mainBtn := ui.ButtonBase(c.Key("qs-main")).Height(28).PaddingX(9).Radius(7).Gap(5).
				AlignItems(ui.Center).Background(p.Field).TextColor(p.Text2).Shrink(0).
				Label("在主窗口中打开").Tooltip("在主窗口中搜索")
			if mainBtn.Hovered() {
				mainBtn.Background(p.RowSel).TextColor(p.Text)
			}
			mainBtn.Children(func() {
				ui.Icon(c, theme.IconExternal).FontSize(13)
				ui.Text(c, "主窗口").FontSize(12).FontWeight(500).SingleLine()
			})
			if mainBtn.Clicked() {
				query := q.query
				q.hide()
				if q.app.Win.ShowSearch != nil {
					q.app.Win.ShowSearch(query)
				}
			}
		})

		// Scope pills: 全部 + groups.
		if len(lib.Groups) > 0 {
			scroll := widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("qs-scopes"))).
				FillWidth().Height(38).Shrink(0).ScrollbarInsets(0, 14, 1, 14).Padding(6, 14, 6, 14).
				BorderWidth(0, 0, 1, 0).BorderColor(p.LineSoft)
			scroll.Children(func() {
				ui.Row(c.Key("qs-scopes-row")).Gap(5).AlignItems(ui.Center).Children(func() {
					allPill := widget.Pill(c.Key("qs-scope-all"), p, "全部", q.groupID == "")
					widget.RevealSelectedTab(allPill, q.groupID == "", 0)
					if allPill.Clicked() {
						q.groupID = ""
						q.selIdx = 0
						q.tmplID = ""
						q.tmplQuery = ""
						q.list.ScrollTo(0, ui.Start)
						q.searchInput.Focus(c)
					}
					for i := range lib.Groups {
						g := &lib.Groups[i]
						sel := q.groupID == g.ID
						pill := widget.Pill(c.Key("qs-scope-"+g.ID), p, g.Name, sel)
						widget.RevealSelectedTab(pill, sel, i+1)
						if pill.Clicked() {
							q.groupID = g.ID
							q.selIdx = 0
							q.tmplID = ""
							q.tmplQuery = ""
							q.list.ScrollTo(0, ui.Start)
							q.searchInput.Focus(c)
						}
					}
				})
			})
		}

		// Template input bar if active.
		if b := templateBookmark(lib, q.tmplID); b != nil {
			ui.Box(c.Key("qs-tmpl-wrap")).Padding(8, 14, 2, 14).Children(func() {
				renderTemplateBar(c, q.app, b, &q.tmplQuery, &q.tmplInput, p.Dark, func() {
					q.tmplID = ""
					q.tmplQuery = ""
					q.searchInput.Focus(c)
				})
			})
		}

		// Results or recent bookmarks.
		if total == 0 {
			msg := "还没有书签"
			if raw != "" {
				msg = "还没有匹配的书签"
			}
			ui.Column(c.Key("qs-empty")).Grow(1).FillWidth().Center().Children(func() {
				ui.Text(c, msg).Font(theme.Serif).FontSize(15).TextColor(p.Text2).SingleLine()
			})
		} else {
			if raw == "" {
				ui.Row(c.Key("qs-recent-hdr")).Padding(8, 16, 2, 16).Children(func() {
					ui.Text(c, "最近更新").FontSize(11.5).FontWeight(500).TextColor(p.Muted).SingleLine()
				})
			}
			if q.scrollMoved {
				q.list.ScrollIntoView(q.selIdx)
				q.scrollMoved = false
			}
			widget.AutoScrollbar(c, ui.List(c.Key("qs-list"), &q.list, total)).Grow(1).MinHeight(0).FillWidth().
				Padding(4, 10, 6, 10).Gap(2).
				ItemKey(func(i int) any {
					if isURL && i == 0 {
						return "qs-url-card"
					}
					return "qs-" + hits[i-offset].B.ID
				}).
				Rows(func(row ui.ListRow) {
					c := row.Context
					if isURL && row.Index == 0 {
						q.renderURLCard(c, p, lib, raw, q.selIdx == 0)
						return
					}
					hit := hits[row.Index-offset]
					q.renderHitRow(c, p, hit, row.Index, row.Index == q.selIdx)
				})
		}

		// Footer shortcut hints.
		copyKey := "⌘C"
		if runtime.GOOS != "darwin" {
			copyKey = "Ctrl+C"
		}
		ui.Row(c.Key("qs-foot")).Height(34).PaddingX(14).Gap(14).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(p.LineSoft).Children(func() {
			quickHint(c, p, "↑↓", "选择")
			quickHint(c, p, "↵", "打开")
			quickHint(c, p, copyKey, "复制链接")
			quickHint(c, p, "Esc", "清空 / 关闭")
			ui.Spacer(c)
			if raw != "" {
				ui.Textf(c, "%d 个结果", len(hits)).FontSize(11.5).TextColor(p.Muted).SingleLine()
			}
		})

		widget.Toast(c, q.app, p, 42)
	})
}

func (q *quickSearchMode) currentHits(lib *model.Library, raw string) []app.Hit {
	if q.cache.library == lib && q.cache.revision == lib.Revision && q.cache.groupID == q.groupID && q.cache.query == raw {
		return q.cache.hits
	}
	var hits []app.Hit
	if raw == "" {
		live := lib.Live()
		var recent []*model.Bookmark
		for _, b := range live {
			if q.groupID == "" || b.GroupID == q.groupID {
				recent = append(recent, b)
			}
		}
		slices.SortStableFunc(recent, func(a, b *model.Bookmark) int {
			if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
				return c
			}
			if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
				return c
			}
			return cmp.Compare(a.ID, b.ID)
		})
		if len(recent) > 6 {
			recent = recent[:6]
		}
		hits = make([]app.Hit, 0, len(recent))
		for _, b := range recent {
			hits = append(hits, app.Hit{B: b, Path: q.app.PathOf(b)})
		}
	} else if q.app.Search != nil {
		hits = q.app.Search.Search(lib, raw, q.groupID)
	} else {
		for _, b := range searchBookmarks(q.app, raw, q.groupID) {
			hits = append(hits, app.Hit{B: b, Path: q.app.PathOf(b)})
		}
	}
	q.cache.library = lib
	q.cache.revision = lib.Revision
	q.cache.groupID = q.groupID
	q.cache.query = raw
	q.cache.hits = hits
	return hits
}

func quickSelectedBookmark(hits []app.Hit, offset, selIdx int) *model.Bookmark {
	idx := selIdx - offset
	if idx < 0 || idx >= len(hits) {
		return nil
	}
	return hits[idx].B
}

func (q *quickSearchMode) activateIndex(c *ui.Context, lib *model.Library, raw string, isURL bool, hits []app.Hit, offset, idx int) {
	if isURL && idx == 0 {
		q.openURLCard(lib, raw)
		return
	}
	if b := quickSelectedBookmark(hits, offset, idx); b != nil {
		q.openItem(c, b)
	}
}

func (q *quickSearchMode) openItem(c *ui.Context, b *model.Bookmark) {
	if b.IsTemplate() {
		q.tmplID = b.ID
		q.tmplQuery = ""
		q.tmplInput.Focus(c)
		return
	}
	q.app.Open(b, "")
}

func (q *quickSearchMode) openURLCard(lib *model.Library, raw string) {
	fullURL := widget.EnsureURLScheme(raw)
	existing := lib.FindURL(fullURL)
	q.hide()
	if q.app.Win.ShowEditor == nil {
		return
	}
	if existing != nil {
		q.app.Win.ShowEditor(app.EditorRequest{ID: existing.ID})
		return
	}
	gid, sid := q.app.Group, q.app.Sub
	if q.groupID != "" {
		if g := lib.Group(q.groupID); g != nil {
			gid, sid = g.ID, firstSubID(g)
		}
	}
	q.app.Win.ShowEditor(app.EditorRequest{URL: fullURL, GroupID: gid, SubID: sid})
}

func (q *quickSearchMode) renderURLCard(c *ui.Context, p *theme.Palette, lib *model.Library, raw string, selected bool) {
	fullURL := widget.EnsureURLScheme(raw)
	existing := lib.FindURL(fullURL)

	card := ui.ButtonBase(c.Key("qs-url-btn")).Padding(9, 12).Margin(2, 0, 4, 0).Radius(10).Gap(10).
		AlignItems(ui.Center).Justify(ui.Start).Background(p.AccentBg.Alpha(0.6))
	if selected {
		card.Border(1.5, p.Accent)
	} else if card.Hovered() {
		card.Background(p.AccentBg.Alpha(0.85)).Border(1, p.Accent.Alpha(0.65))
	} else {
		card.Border(1, p.Accent.Alpha(0.4))
	}

	title := "保存这个网址"
	subtitle := raw
	if existing != nil {
		title = "编辑已保存的书签「" + existing.Title + "」"
		subtitle = raw + " · 已存在于「" + q.app.PathOf(existing) + "」"
	}
	card.Label(title).Tooltip(subtitle)

	card.Children(func() {
		ui.Row(c).Size(28, 28).Radius(8).Background(p.Primary).Center().Shrink(0).Children(func() {
			icon := theme.IconPlus
			if existing != nil {
				icon = theme.IconEdit
			}
			ui.Icon(c, icon).FontSize(14).TextColor(p.OnPrimary)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(p.Text).SingleLine()
			ui.Text(c, subtitle).FontSize(11.5).TextColor(p.Muted).SingleLine()
		})
		ui.Row(c).Padding(2, 6).Radius(6).Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
			ui.Text(c, "↵").FontSize(11.5).TextColor(p.Muted).SingleLine()
		})
	})

	if card.Clicked() {
		q.openURLCard(lib, raw)
	}
}

func (q *quickSearchMode) renderHitRow(c *ui.Context, p *theme.Palette, hit app.Hit, idx int, selected bool) {
	b := hit.B
	btn := ui.ButtonBase(c.Key("qs-bm-"+b.ID)).Height(44).Padding(0, 10).Gap(10).
		Radius(9).Justify(ui.Start).AlignItems(ui.Center).Label(b.Title).Tooltip(b.Title)
	if selected {
		btn.Background(p.RowSel)
	} else if btn.Hovered() {
		btn.Background(p.Hover)
	}

	path := hit.Path
	if path == "" {
		path = q.app.PathOf(b)
	}

	btn.Children(func() {
		theme.Tile(c, b.Icon, q.app.Store.IconPath(b.Icon), 26).Radius(7)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).MinWidth(0).Children(func() {
				ui.RichText(c, quickHighlightedSpans(b.Title, hit.Ranges, p.AccentBg, p.AccentText)...).
					FontSize(13.5).FontWeight(500).TextColor(p.Text).SingleLine().Shrink(1).MinWidth(0)
				if b.IsTemplate() {
					ui.Icon(c, theme.IconBraces).FontSize(12).TextColor(p.Primary).Shrink(0)
				}
			})
			ui.Text(c, widget.ShortDomain(b.URL)).FontSize(11.5).TextColor(p.Muted).SingleLine()
		})
		ui.Text(c, path).FontSize(11.5).TextColor(p.Muted).MaxWidth(140).SingleLine().Shrink(0).Tooltip(path)
		if selected {
			ui.Row(c).Padding(2, 6).Radius(6).Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
				ui.Text(c, "↵").FontSize(11.5).TextColor(p.Muted).SingleLine()
			})
		}
	})

	if btn.Clicked() {
		q.selIdx = idx
		q.openItem(c, b)
	}
}

func quickHint(c *ui.Context, p *theme.Palette, key, label string) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Row(c).Padding(1, 5).Radius(4).Background(p.Field).Border(1, p.Line).Children(func() {
			ui.Text(c, key).FontSize(11).TextColor(p.Text2).SingleLine()
		})
		ui.Text(c, label).FontSize(11.5).TextColor(p.Muted).SingleLine()
	})
}

func quickHighlightedSpans(title string, ranges [][2]int, hiBg, hiFg ui.Color) []ui.Span {
	if len(ranges) == 0 {
		return []ui.Span{{Text: title}}
	}
	runes := []rune(title)
	n := len(runes)
	sorted := slices.Clone(ranges)
	slices.SortFunc(sorted, func(a, b [2]int) int { return cmp.Compare(a[0], b[0]) })
	var spans []ui.Span
	pos := 0
	for _, r := range sorted {
		start := max(pos, min(r[0], n))
		end := max(start, min(r[1], n))
		if start > pos {
			spans = append(spans, ui.Span{Text: string(runes[pos:start])})
		}
		if end > start {
			spans = append(spans, ui.Span{Text: string(runes[start:end]), Background: hiBg, Color: hiFg, Radius: 3, Weight: 600})
		}
		pos = end
	}
	if pos < n {
		spans = append(spans, ui.Span{Text: string(runes[pos:])})
	}
	if len(spans) == 0 {
		return []ui.Span{{Text: title}}
	}
	return spans
}
