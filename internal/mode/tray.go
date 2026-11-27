package mode

import (
	"math"
	"runtime"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

type trayMode struct {
	app            *app.App
	tray           *mygo.Tray
	flyout         *mygo.Window
	detached       *mygo.Window
	menu           *mygo.Menu
	groupID        string
	menuOpen       bool
	refocus        bool
	search         modeSearchState
	flyoutSearch   ui.Handle
	detachedSearch ui.Handle
	flyoutList     ui.ListState
	detachedList   ui.ListState
	cache          struct {
		library  *model.Library
		revision int64
		groupID  string
		query    string
		items    []*model.Bookmark
		engines  []*model.Bookmark
	}
}

func (t *trayMode) searchHandle(detached bool) *ui.Handle {
	if detached {
		return &t.detachedSearch
	}
	return &t.flyoutSearch
}

// refocusOnActivate asks the panel's next build to focus its search field
// whenever w becomes the active window again.
func (t *trayMode) refocusOnActivate(w *mygo.Window) {
	w.OnFocus(func() {
		if w.IsDestroyed() {
			return
		}
		t.refocus = true
		w.Invalidate()
	})
}

func syncTray(a *app.App, on bool) {
	if !on {
		if curTray != nil {
			curTray.destroy()
			curTray = nil
		}
		return
	}
	if curTray == nil {
		curTray = newTrayMode(a)
		return
	}
	curTray.app = a
	if curTray.flyout != nil {
		curTray.flyout.Invalidate()
	}
	if curTray.detached != nil {
		curTray.detached.Invalidate()
	}
}

func newTrayMode(a *app.App) *trayMode {
	t := &trayMode{
		app:     a,
		groupID: initialModeGroup(a.Settings.TrayGroup, a.Group),
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "cooool-mark-tray",
		Icon:           trayIconBytes(),
		IconIsTemplate: runtime.GOOS == "darwin",
		ToolTip:        locales.AppName(),
	})
	if err != nil {
		a.SayErr(locales.ModeTrayInitFailed(err.Error()), "", nil)
		return nil
	}
	t.tray = tray
	t.menu = t.buildMenu()

	if runtime.GOOS == "linux" {
		t.tray.SetMenu(t.menu)
	} else {
		t.tray.OnClick(t.toggleAttached)
		t.tray.OnRightClick(func() {
			t.hideAttached()
			t.tray.SetMenu(t.menu)
			defer t.tray.SetMenu(nil)
			t.tray.PopUpMenu()
		})
	}
	return t
}

func (t *trayMode) refreshChrome(a *app.App) {
	if a != nil {
		t.app = a
	}
	t.menu = t.buildMenu()
	if t.tray != nil {
		t.tray.SetToolTip(locales.AppName())
		if runtime.GOOS == "linux" {
			t.tray.SetMenu(t.menu)
		}
	}
	if t.flyout != nil {
		t.flyout.Invalidate()
	}
	if t.detached != nil {
		t.detached.SetTitle(locales.AppName())
		t.detached.Invalidate()
	}
}

func (t *trayMode) buildMenu() *mygo.Menu {
	var items []*mygo.MenuItem
	if runtime.GOOS == "linux" {
		items = append(items,
			&mygo.MenuItem{
				Label: locales.MenuOpenBookmarkPanel(),
				Click: func(*mygo.MenuItem, *mygo.Window) { t.toggleAttached() },
			},
			mygo.Separator(),
		)
	}
	items = append(items,
		&mygo.MenuItem{
			Label: locales.MenuNewBookmark(),
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.hideAttached()
				if t.app.Win.ShowEditor != nil {
					gid, sid := t.selectedGroupSub()
					t.app.Win.ShowEditor(app.EditorRequest{GroupID: gid, SubID: sid})
				}
			},
		},
		&mygo.MenuItem{
			Label: locales.MenuQuickAddClipboard(),
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.hideAttached()
				t.app.QuickAddFromClipboard()
			},
		},
		&mygo.MenuItem{
			Label: locales.MenuQuickSearch(),
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.hideAttached()
				if t.app.Win.ShowQuickSearch != nil {
					t.app.Win.ShowQuickSearch()
				}
			},
		},
		&mygo.MenuItem{
			Label: locales.MenuOpenMain(),
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.hideAttached()
				if t.app.Win.ShowMain != nil {
					t.app.Win.ShowMain()
				}
			},
		},
		&mygo.MenuItem{
			Label: locales.MenuSettings(),
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.hideAttached()
				if t.app.Win.ShowSettings != nil {
					t.app.Win.ShowSettings("general")
				}
			},
		},
		mygo.Separator(),
		&mygo.MenuItem{
			Label: locales.MenuQuit(),
			Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() },
		},
	)
	return mygo.NewMenu(items)
}

func (t *trayMode) destroy() {
	if t.flyout != nil {
		w := t.flyout
		t.flyout = nil
		w.Destroy()
	}
	if t.detached != nil {
		w := t.detached
		t.detached = nil
		w.Destroy()
	}
	if t.tray != nil {
		tr := t.tray
		t.tray = nil
		tr.Destroy()
	}
}

func (t *trayMode) hideAttached() {
	if t.flyout != nil {
		t.flyout.Close()
	}
}

func (t *trayMode) toggleAttached() {
	if t.detached != nil {
		if !t.detached.IsVisible() {
			t.detached.Show()
		}
		t.detached.Focus()
		t.detachedSearch.SelectAll()
		return
	}
	if t.flyout != nil {
		t.flyout.Close()
		return
	}
	t.showAttached()
}

func (t *trayMode) showAttached() {
	if t.tray == nil || t.flyout != nil {
		return
	}
	t.search.reset()
	t.flyoutList.ScrollTo(0, ui.Start)
	w := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:         t.tray,
		Placement:    mygo.PlacementBottom,
		Gap:          4,
		Width:        340,
		Height:       472,
		Focusable:    true,
		Shadow:       true,
		Popover:      true,
		CornerRadius: 14,
		Content:      ui.View(func(c *ui.Context) { t.view(c, false) }),
	})
	t.flyout = w
	t.app.AddWindow(w)
	t.flyoutSearch.SelectAll()
	t.refocusOnActivate(w)
	w.OnClosed(func() {
		if t.flyout == w {
			t.flyout = nil
			if t.detached == nil {
				t.search.reset()
			}
		}
	})
}

func (t *trayMode) tearOff() {
	if t.detached != nil {
		t.detached.Show()
		t.detached.Focus()
		t.detachedSearch.SelectAll()
		return
	}
	var bx, by int
	if t.flyout != nil {
		b := t.flyout.Bounds()
		bx, by = b.X, b.Y
		if first, last := t.flyoutList.Visible(); first <= last {
			t.detachedList.ScrollTo(first, ui.Start)
		}
		t.flyout.Close()
		t.flyout = nil
	} else {
		wa := displayRect(cursorDisplay())
		bx = wa.X + wa.Width - 360
		by = wa.Y + 40
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:             locales.AppName(),
		Width:             340,
		Height:            472,
		X:                 bx,
		Y:                 by,
		Frameless:         true,
		Transparent:       true,
		Vibrancy:          mygo.VibrancyPopover,
		CornerRadius:      14,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		Panel:             true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		StateKey:          "tray-detached",
		Content:           ui.View(func(c *ui.Context) { t.view(c, true) }),
	})
	t.detached = w
	t.app.AddWindow(w)
	w.Show()
	w.Focus()
	t.detachedSearch.SelectAll()
	t.refocusOnActivate(w)
	w.OnClosed(func() {
		if t.detached == w {
			t.detached = nil
			t.search.reset()
		}
	})
}

func (t *trayMode) reattach() {
	if t.detached != nil {
		if first, last := t.detachedList.Visible(); first <= last {
			t.flyoutList.ScrollTo(first, ui.Start)
		}
		w := t.detached
		t.detached = nil
		w.Destroy()
	}
	t.showAttached()
}

func (t *trayMode) selectedGroupSub() (string, string) {
	lib := t.app.Store.Lib()
	if len(lib.Groups) == 0 {
		return "", ""
	}
	g := activeGroup(lib, t.groupID)
	return g.ID, firstSubID(g)
}

func (t *trayMode) view(c *ui.Context, detached bool) {
	menuOpenThisFrame := false
	defer func() { t.menuOpen = menuOpenThisFrame }()

	p := theme.Apply(c, t.app.Settings.Appearance)
	c.Root().Background(ui.Transparent)

	lib := t.app.Store.Lib()
	if t.search.lockedID != "" && templateBookmark(lib, t.search.lockedID) == nil {
		t.search.unlockKeepQuery()
	}

	searchH := t.searchHandle(detached)

	// Coming back to the panel from elsewhere puts the caret back in search.
	if t.refocus {
		t.refocus = false
		if !t.menuOpen {
			searchH.Focus(c)
		}
	}

	if !t.menuOpen && c.Shortcut(ui.Cmd, ui.KeyF) && !c.ShortcutRepeated() {
		searchH.SelectAll(c)
	}

	if !t.menuOpen && !searchH.Focused(c) {
		if ch, ok := widget.TypedSearchKey(c); ok {
			t.search.query += ch
			t.search.selIdx = 0
			t.search.caretPending = true
			t.search.pendingCaret = utf8.RuneCountInString(t.search.query)
			t.flyoutList.ScrollTo(0, ui.Start)
			t.detachedList.ScrollTo(0, ui.Start)
			searchH.Focus(c)
		}
	}

	if g := activeGroup(lib, t.groupID); g != nil {
		t.groupID = g.ID
	} else {
		t.groupID = ""
	}

	reduceTrans := theme.ReduceTransparency()
	box := ui.Column(c).Fill()
	switch {
	case reduceTrans && runtime.GOOS == "windows" && !detached:
		box.Background(p.Bg)
	case reduceTrans:
		box.Background(p.Bg).Radius(14).Border(1, p.Line)
	case c.Vibrancy():
		alpha := float32(0.96)
		if p.Dark {
			alpha = 0.92
		}
		box.Background(p.Surface.Alpha(alpha)).Radius(14).Border(1, p.Line)
	case runtime.GOOS == "windows" && !detached:
		box.Background(p.Surface)
	default:
		box.Background(p.Surface).Radius(14).Border(1, p.Line)
	}

	// Accept URL drops directly into the current group.
	dropOpts := urlDropOptions()
	_, dragOver := ui.DataDragOver(box, dropOpts)
	if drop, ok := ui.DropData(box, dropOpts); ok {
		if raw := extractDropURL(drop.Data); raw != "" {
			gid, sid := t.selectedGroupSub()
			t.app.QuickAddToPlace(raw, "", gid, sid, false)
		}
	}
	if dragOver {
		box.Border(1.5, p.Accent)
	}

	box.Children(func() {
		// Top row: group tabs + pin/reattach (+ close when detached), followed by persistent auto-focused search input.
		hdr := ui.Row(c.Key("tray-hdr")).Height(44).Padding(0, 8, 0, 10).Gap(6).AlignItems(ui.Center)
		if detached {
			hdr.DragWindow()
		}
		hdr.Children(func() {
			if len(lib.Groups) > 0 {
				scroll := widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("tray-tabs"))).
					Grow(1).MinWidth(0).Height(30).ScrollbarInsets(0, 2, 0, 2).Padding(2, 0)
				scroll.Children(func() {
					ui.Row(c.Key("tray-tabs-row")).Gap(4).AlignItems(ui.Center).Children(func() {
						for i := range lib.Groups {
							g := &lib.Groups[i]
							selected := g.ID == t.groupID && t.search.trimmedQuery() == "" && t.search.lockedID == ""
							pill := widget.Pill(c.Key("tray-grp-"+g.ID), p, g.Name, selected)
							widget.RevealSelectedTab(pill, selected, i)
							if pill.Clicked() {
								t.groupID = g.ID
								t.app.SetModeGroup(model.ModeTray, g.ID)
								t.search.reset()
								t.flyoutList.ScrollTo(0, ui.Start)
								t.detachedList.ScrollTo(0, ui.Start)
								searchH.Focus(c)
							}
						}
					})
				})
			} else {
				ui.Spacer(c)
			}

			pinLabel := locales.ActionDetachWindow()
			pinIcon := theme.IconPin
			if detached {
				pinLabel = locales.ActionAttachMenuBar()
				pinIcon = theme.IconPinFilled
			}
			pinBtn := widget.IconButton(c.Key("tray-pin"), p, pinIcon, pinLabel).Shrink(0)
			if detached {
				pinBtn.Background(p.AccentBg).TextColor(p.Primary)
			}
			if pinBtn.Clicked() {
				if detached {
					t.reattach()
				} else {
					t.tearOff()
				}
			}
			if detached {
				closeBtn := widget.IconButton(c.Key("tray-close"), p, theme.IconClose, locales.ActionClose()).Shrink(0)
				if closeBtn.Clicked() {
					if t.detached != nil {
						t.detached.Close()
					}
				}
			}
		})

		scrollTop := func() {
			t.flyoutList.ScrollTo(0, ui.Start)
			t.detachedList.ScrollTo(0, ui.Start)
		}
		fieldBg := p.Text.Alpha(0.06)
		if !c.Vibrancy() || reduceTrans {
			fieldBg = p.Field
		}
		var searchIn ui.Element
		ui.Row(c.Key("tray-srow")).Padding(0, 10, 8, 10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.LineSoft).Children(func() {
			searchIn = renderModeSearchBox(c, t.app, p, "tray", &t.search, searchH, modeSearchStyle{fieldBg: fieldBg}, scrollTop)
		})

		q := t.search.trimmedQuery()
		locked := templateBookmark(lib, t.search.lockedID)
		var items, engines []*model.Bookmark
		if locked == nil {
			items, engines = t.currentItems(lib, q)
		}
		var hits []*model.Bookmark
		if locked == nil && q != "" {
			hits = items
		}
		handleModeSearchNav(t.app, lib, &t.search, searchIn, hits, engines, scrollTop)
		locked = templateBookmark(lib, t.search.lockedID)

		openOrLock := func(b *model.Bookmark) {
			if b.IsTemplate() {
				t.search.lockEngine(b, "", "")
				searchIn.ReplaceText(0, math.MaxInt, "")
				scrollTop()
				searchH.Focus(c)
				return
			}
			t.app.Open(b, "")
		}

		// Body: locked engine preview, search hits + engines, or browse list
		if locked != nil {
			ui.Column(c.Key("tray-locked-wrap")).Grow(1).Padding(4, 6).Children(func() {
				renderLockedEngineRow(c, t.app, p, "tray-locked", locked, q, 46, false, func() {
					t.app.Open(locked, q)
				})
			})
		} else if q != "" {
			type trayRowKind uint8
			const (
				trayRowBookmark trayRowKind = iota
				trayRowEngHeader
				trayRowEngine
			)
			type trayRow struct {
				kind   trayRowKind
				b      *model.Bookmark
				selIdx int
			}
			rows := make([]trayRow, 0, len(hits)+len(engines)+1)
			for i, b := range hits {
				rows = append(rows, trayRow{kind: trayRowBookmark, b: b, selIdx: i})
			}
			if len(engines) > 0 {
				rows = append(rows, trayRow{kind: trayRowEngHeader, selIdx: -1})
				for j, eng := range engines {
					rows = append(rows, trayRow{kind: trayRowEngine, b: eng, selIdx: len(hits) + j})
				}
			}
			if len(rows) == 0 {
				ui.Column(c.Key("tray-empty")).Grow(1).Center().Padding(60, 16).Children(func() {
					ui.Text(c, locales.MainNoMatchingBookmarks()).Font(theme.TitleFont(c)).FontSize(15).TextColor(p.Text2).SingleLine()
				})
			} else {
				listState := &t.flyoutList
				if detached {
					listState = &t.detachedList
				}
				if t.search.scrollMoved {
					t.search.scrollMoved = false
					rowIdx := t.search.selIdx
					if rowIdx >= len(hits) {
						rowIdx++ // account for engine section header
					}
					listState.ScrollIntoView(rowIdx)
				}
				widget.AutoScrollbar(c, ui.List(c.Key("tray-search-list"), listState, len(rows))).Grow(1).MinHeight(0).FillWidth().
					Padding(2, 6).Gap(1).
					ItemKey(func(i int) any {
						r := rows[i]
						switch r.kind {
						case trayRowEngHeader:
							return "eng-hdr"
						case trayRowEngine:
							return "eng-" + r.b.ID
						default:
							return "bm-" + r.b.ID
						}
					}).
					Rows(func(row ui.ListRow) {
						c := row.Context
						r := rows[row.Index]
						switch r.kind {
						case trayRowEngHeader:
							top := float32(10)
							if row.Index == 0 {
								top = 4
							}
							renderEngineSectionHeader(c, p, "tray-eng-hdr", q, top, false)
						case trayRowEngine:
							selected := r.selIdx == t.search.selIdx
							renderEngineRow(c, t.app, p, "tray-eng-"+r.b.ID, r.b, q, 36, selected, false, func() {
								t.app.Open(r.b, q)
							})
						default:
							b := r.b
							selected := r.selIdx == t.search.selIdx
							btn := ui.ButtonBase(c.Key("tr-" + b.ID)).Height(36).PaddingX(8).Gap(10).
								Radius(8).Justify(ui.Start).AlignItems(ui.Center).Label(b.Title)
							titleH := ui.Local(btn, "titleH", func() ui.Handle { return ui.Handle{} })
							if widget.TextTruncated(c, titleH, b.Title, 0, 0, 1) {
								btn.Tooltip(b.Title)
							}
							if selected {
								btn.Background(p.RowSel)
							} else if btn.Hovered() {
								btn.Background(p.Hover)
							}
							btn.Children(func() {
								theme.Tile(c, b.Icon, t.app.Store.IconPath(b.Icon), 22).Radius(6)
								ui.Text(c, b.Title).Grow(1).MinWidth(0).TextColor(p.Text).SingleLine().Bind(titleH)
								if selected && b.IsTemplate() {
									ui.Text(c, locales.EngineTabLock()).FontSize(11).TextColor(p.Muted).SingleLine().Shrink(0)
								}
							})
							if btn.Clicked() {
								openOrLock(b)
							}
							bindBookmarkPopoverMenu(c, t.app, btn, b, modeMenuOptions(p), &menuOpenThisFrame,
								func() { openOrLock(b) },
								func() {
									if !detached {
										t.hideAttached()
									}
								},
								nil,
							)
						}
					})
			}
		} else if len(items) == 0 {
			ui.Column(c.Key("tray-empty")).Grow(1).Center().Padding(60, 16).Children(func() {
				ui.Text(c, locales.MainNoBookmarks()).Font(theme.TitleFont(c)).FontSize(15).TextColor(p.Text2).SingleLine()
			})
		} else {
			listState := &t.flyoutList
			if detached {
				listState = &t.detachedList
			}
			widget.AutoScrollbar(c, ui.List(c.Key("tray-list"), listState, len(items))).Grow(1).MinHeight(0).FillWidth().
				Padding(2, 6).Gap(1).
				ItemKey(func(i int) any { return items[i].ID }).
				Rows(func(row ui.ListRow) {
					c := row.Context
					b := items[row.Index]
					btn := ui.ButtonBase(c.Key("tr-" + b.ID)).Height(36).PaddingX(8).Gap(10).
						Radius(8).Justify(ui.Start).AlignItems(ui.Center).Label(b.Title)
					titleH := ui.Local(btn, "titleH", func() ui.Handle { return ui.Handle{} })
					if widget.TextTruncated(c, titleH, b.Title, 0, 0, 1) {
						btn.Tooltip(b.Title)
					}
					if btn.Hovered() {
						btn.Background(p.Hover)
					}
					btn.Children(func() {
						theme.Tile(c, b.Icon, t.app.Store.IconPath(b.Icon), 22).Radius(6)
						ui.Text(c, b.Title).Grow(1).MinWidth(0).TextColor(p.Text).SingleLine().Bind(titleH)
					})
					if btn.Clicked() {
						openOrLock(b)
					}
					bindBookmarkPopoverMenu(c, t.app, btn, b, modeMenuOptions(p), &menuOpenThisFrame,
						func() { openOrLock(b) },
						func() {
							if !detached {
								t.hideAttached()
							}
						},
						nil,
					)
				})
		}

		// Footer bar
		ui.Row(c.Key("tray-foot")).Height(44).PaddingX(8).Gap(2).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(func() {
			addBtn := widget.IconButton(c, p, theme.IconPlus, locales.MenuNewBookmark())
			if addBtn.Clicked() {
				if !detached {
					t.hideAttached()
				}
				if t.app.Win.ShowEditor != nil {
					gid, sid := t.selectedGroupSub()
					t.app.Win.ShowEditor(app.EditorRequest{GroupID: gid, SubID: sid})
				}
			}
			ui.Spacer(c)
			mainBtn := widget.IconButton(c, p, theme.IconExternal, locales.MenuOpenMain())
			if mainBtn.Clicked() {
				if !detached {
					t.hideAttached()
				}
				if t.app.Win.ShowMain != nil {
					t.app.Win.ShowMain()
				}
			}
			setBtn := widget.IconButton(c, p, theme.IconSettings, locales.SettingsTitle())
			if setBtn.Clicked() {
				if !detached {
					t.hideAttached()
				}
				if t.app.Win.ShowSettings != nil {
					t.app.Win.ShowSettings("general")
				}
			}
		})
		widget.Toast(c, t.app, p, 52)
	})

	// Evaluate Escape after TextInputBase so t.search.query is synced with text typed this frame.
	if !t.menuOpen && !menuOpenThisFrame && c.Shortcut(0, ui.KeyEscape) {
		inv := func() {
			if detached && t.detached != nil {
				t.detached.Invalidate()
			} else if !detached && t.flyout != nil {
				t.flyout.Invalidate()
			}
		}
		handleModeEscape(&t.search, searchH, c, func() {
			t.flyoutList.ScrollTo(0, ui.Start)
			t.detachedList.ScrollTo(0, ui.Start)
		}, func() {
			if !detached {
				t.hideAttached()
			}
		}, inv)
	}
}

func (t *trayMode) currentItems(lib *model.Library, q string) ([]*model.Bookmark, []*model.Bookmark) {
	if t.cache.library == lib && t.cache.revision == lib.Revision && t.cache.groupID == t.groupID && t.cache.query == q {
		return t.cache.items, t.cache.engines
	}
	var items, engines []*model.Bookmark
	if q != "" {
		items = searchBookmarks(t.app, q, "")
		engines = t.app.Engines()
	} else if len(lib.Groups) > 0 {
		items = groupBookmarks(lib, activeGroup(lib, t.groupID))
	}
	t.cache.library = lib
	t.cache.revision = lib.Revision
	t.cache.groupID = t.groupID
	t.cache.query = q
	t.cache.items = items
	t.cache.engines = engines
	return items, engines
}
