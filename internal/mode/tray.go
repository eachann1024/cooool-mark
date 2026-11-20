package mode

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
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
	searching      bool
	query          string
	flyoutSearch   ui.Handle
	detachedSearch ui.Handle
	tmplID         string
	tmplQuery      string
	flyoutTmpl     ui.Handle
	detachedTmpl   ui.Handle
	flyoutList     ui.ListState
	detachedList   ui.ListState
	cache          struct {
		library  *model.Library
		revision int64
		groupID  string
		query    string
		items    []*model.Bookmark
	}
}

func (t *trayMode) searchHandle(detached bool) *ui.Handle {
	if detached {
		return &t.detachedSearch
	}
	return &t.flyoutSearch
}

func (t *trayMode) tmplHandle(detached bool) *ui.Handle {
	if detached {
		return &t.detachedTmpl
	}
	return &t.flyoutTmpl
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
		ToolTip:        "Cooool Mark",
	})
	if err != nil {
		a.SayErr("菜单栏初始化失败："+err.Error(), "", nil)
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

func (t *trayMode) buildMenu() *mygo.Menu {
	var items []*mygo.MenuItem
	if runtime.GOOS == "linux" {
		items = append(items,
			&mygo.MenuItem{
				Label: "打开书签面板",
				Click: func(*mygo.MenuItem, *mygo.Window) { t.toggleAttached() },
			},
			mygo.Separator(),
		)
	}
	items = append(items,
		&mygo.MenuItem{
			Label: "新建书签",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.hideAttached()
				if t.app.Win.ShowEditor != nil {
					gid, sid := t.selectedGroupSub()
					t.app.Win.ShowEditor(app.EditorRequest{GroupID: gid, SubID: sid})
				}
			},
		},
		&mygo.MenuItem{
			Label: "打开主窗口",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.hideAttached()
				if t.app.Win.ShowMain != nil {
					t.app.Win.ShowMain()
				}
			},
		},
		&mygo.MenuItem{
			Label: "设置…",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.hideAttached()
				if t.app.Win.ShowSettings != nil {
					t.app.Win.ShowSettings("general")
				}
			},
		},
		mygo.Separator(),
		&mygo.MenuItem{
			Label: "退出",
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
		if t.searching {
			t.detachedSearch.Focus()
		}
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
	t.searching = false
	t.query = ""
	t.tmplID = ""
	t.tmplQuery = ""
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
	w.OnClosed(func() {
		if t.flyout == w {
			t.flyout = nil
			if t.detached == nil {
				t.searching = false
				t.query = ""
			}
		}
	})
}

func (t *trayMode) tearOff() {
	if t.detached != nil {
		t.detached.Show()
		t.detached.Focus()
		if t.searching {
			t.detachedSearch.Focus()
		}
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
		Title:             "Cooool Mark",
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
	if t.searching {
		t.detachedSearch.Focus()
	}
	w.OnClosed(func() {
		if t.detached == w {
			t.detached = nil
			t.searching = false
			t.query = ""
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
	p := theme.Apply(c, t.app.Settings.Appearance)
	c.Root().Background(ui.Transparent)

	lib := t.app.Store.Lib()
	if t.tmplID != "" && templateBookmark(lib, t.tmplID) == nil {
		t.tmplID = ""
	}

	searchH := t.searchHandle(detached)
	tmplH := t.tmplHandle(detached)

	if c.Shortcut(ui.Cmd, ui.KeyF) && !c.ShortcutRepeated() {
		t.searching = true
		searchH.SelectAll(c)
	}

	if c.Shortcut(0, ui.KeyEscape) {
		if t.tmplID != "" {
			t.tmplID = ""
			t.tmplQuery = ""
			if t.searching {
				searchH.Focus(c)
			}
		} else if t.searching {
			t.searching = false
			t.query = ""
			t.flyoutList.ScrollTo(0, ui.Start)
			t.detachedList.ScrollTo(0, ui.Start)
			searchH.CancelFocus(c)
		} else if !detached {
			t.hideAttached()
			return
		}
	}

	if !t.searching && !searchH.Focused(c) && !tmplH.Focused(c) {
		if ch, ok := widget.TypedSearchKey(c); ok {
			t.searching = true
			t.query = ch
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

	box := ui.Column(c).Fill()
	switch {
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
			saveDroppedURL(t.app, raw, gid, sid)
		}
	}
	if dragOver {
		box.Border(1.5, p.Accent)
	}

	box.Children(func() {
		// Single-row header: browse (group pills + search icon + pin/close) OR search (input + close)
		hdr := ui.Row(c.Key("tray-hdr")).Height(48).Padding(0, 8, 0, 10).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.LineSoft)
		if detached {
			hdr.DragWindow()
		}
		hdr.Children(func() {
			if !t.searching {
				if len(lib.Groups) > 0 {
					scroll := widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("tray-tabs"))).
						Grow(1).MinWidth(0).Height(30).ScrollbarInsets(0, 2, 0, 2).Padding(2, 0)
					scroll.Children(func() {
						ui.Row(c.Key("tray-tabs-row")).Gap(4).AlignItems(ui.Center).Children(func() {
							for i := range lib.Groups {
								g := &lib.Groups[i]
								selected := g.ID == t.groupID
								pill := widget.Pill(c.Key("tray-grp-"+g.ID), p, g.Name, selected)
								widget.RevealSelectedTab(pill, selected, i)
								if pill.Clicked() {
									t.groupID = g.ID
									t.app.SetModeGroup(model.ModeTray, g.ID)
									t.tmplID = ""
									t.flyoutList.ScrollTo(0, ui.Start)
									t.detachedList.ScrollTo(0, ui.Start)
								}
							}
						})
					})
				} else {
					ui.Spacer(c)
				}

				searchBtn := widget.IconButton(c.Key("tray-search-btn"), p, theme.IconSearch, "搜索书签").Shrink(0)
				if searchBtn.Clicked() {
					t.searching = true
					searchH.Focus(c)
				}

				pinLabel := "撕下成小窗"
				pinIcon := theme.IconPin
				if detached {
					pinLabel = "收回菜单栏"
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
					closeBtn := widget.IconButton(c.Key("tray-close"), p, theme.IconClose, "关闭").Shrink(0)
					if closeBtn.Clicked() {
						if t.detached != nil {
							t.detached.Close()
						}
					}
				}
				return
			}

			fieldBg := p.Text.Alpha(0.06)
			if !c.Vibrancy() {
				fieldBg = p.Field
			}
			focused := searchH.Focused(c)
			hasQ := t.query != ""
			sbox := ui.Row(c.Key("tray-sbox")).Grow(1).MinWidth(0).Height(30).Padding(0, 6, 0, 10).Gap(6).Radius(9).
				AlignItems(ui.Center)
			if focused || hasQ {
				sbox.Background(p.Surface).Border(1.5, p.Accent)
			} else {
				sbox.Background(fieldBg).Border(1.5, ui.Transparent)
			}
			sbox.Children(func() {
				ui.Icon(c, theme.IconSearch).FontSize(14).TextColor(p.Muted).Shrink(0)
				in := ui.TextInputBase(c.Key("tray-q"), &t.query).Bind(searchH).Grow(1).MinWidth(0).
					Label("搜索书签").Placeholder("搜索书签").TextColor(p.Text).PlaceholderColor(p.Muted)
				if in.Changed() {
					t.flyoutList.ScrollTo(0, ui.Start)
					t.detachedList.ScrollTo(0, ui.Start)
				}
				if in.Submitted() {
					hits := searchBookmarks(t.app, t.query, "")
					if len(hits) > 0 {
						t.openItem(hits[0])
						if t.tmplID != "" {
							tmplH.Focus(c)
						}
					}
				}
				if hasQ {
					clr := ui.ButtonBase(c.Key("tray-qclr")).Size(18, 18).Radius(5).Center().
						TextColor(p.Muted).Label("清空搜索").Tooltip("清空搜索").Shrink(0)
					if clr.Hovered() {
						clr.Background(p.RowSel).TextColor(p.Text)
					}
					clr.Children(func() { ui.Icon(c, theme.IconClose).FontSize(11) })
					if clr.Clicked() {
						t.query = ""
						t.flyoutList.ScrollTo(0, ui.Start)
						t.detachedList.ScrollTo(0, ui.Start)
						searchH.Focus(c)
					}
				}
			})
			closeSearch := widget.IconButton(c.Key("tray-search-close"), p, theme.IconClose, "退出搜索").Shrink(0)
			if closeSearch.Clicked() {
				t.searching = false
				t.query = ""
				t.flyoutList.ScrollTo(0, ui.Start)
				t.detachedList.ScrollTo(0, ui.Start)
				searchH.CancelFocus(c)
			}
		})

		q := strings.TrimSpace(t.query)

		if b := templateBookmark(lib, t.tmplID); b != nil {
			ui.Box(c.Key("tray-tmpl-wrap")).Padding(6, 10, 6, 10).Children(func() {
				renderTemplateBar(c, t.app, b, &t.tmplQuery, tmplH, p.Dark, func() {
					t.tmplID = ""
					t.tmplQuery = ""
					if t.searching {
						searchH.Focus(c)
					}
				})
			})
		}

		// Bookmark list
		items := t.currentItems(lib, q)
		if len(items) == 0 {
			msg := "还没有书签"
			if q != "" {
				msg = "还没有匹配的书签"
			}
			ui.Column(c.Key("tray-empty")).Grow(1).Center().Padding(60, 16).Children(func() {
				ui.Text(c, msg).Font(theme.Serif).FontSize(15).TextColor(p.Text2).SingleLine()
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
						Radius(8).Justify(ui.Start).AlignItems(ui.Center).Label(b.Title).Tooltip(b.Title)
					if btn.Hovered() {
						btn.Background(p.Hover)
					}
					btn.Children(func() {
						theme.Tile(c, b.Icon, t.app.Store.IconPath(b.Icon), 22).Radius(6)
						ui.Text(c, b.Title).Grow(1).TextColor(p.Text).SingleLine()
					})
					if btn.Clicked() {
						t.openItem(b)
						if t.tmplID != "" {
							tmplH.Focus(c)
						}
					}
				})
		}

		// Footer bar
		ui.Row(c.Key("tray-foot")).Height(44).PaddingX(8).Gap(2).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(func() {
			addBtn := widget.IconButton(c, p, theme.IconPlus, "新建书签")
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
			mainBtn := widget.IconButton(c, p, theme.IconExternal, "打开主窗口")
			if mainBtn.Clicked() {
				if !detached {
					t.hideAttached()
				}
				if t.app.Win.ShowMain != nil {
					t.app.Win.ShowMain()
				}
			}
			setBtn := widget.IconButton(c, p, theme.IconSettings, "设置")
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
}

func (t *trayMode) currentItems(lib *model.Library, q string) []*model.Bookmark {
	if t.cache.library == lib && t.cache.revision == lib.Revision && t.cache.groupID == t.groupID && t.cache.query == q {
		return t.cache.items
	}
	var items []*model.Bookmark
	if q != "" {
		items = searchBookmarks(t.app, q, "")
	} else if len(lib.Groups) > 0 {
		items = groupBookmarks(lib, activeGroup(lib, t.groupID))
	}
	t.cache.library = lib
	t.cache.revision = lib.Revision
	t.cache.groupID = t.groupID
	t.cache.query = q
	t.cache.items = items
	return items
}

func (t *trayMode) openItem(b *model.Bookmark) {
	if b.IsTemplate() {
		t.tmplID = b.ID
		t.tmplQuery = ""
		return
	}
	t.app.Open(b, "")
}
