package mode

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

type edgeState uint8

const (
	edgeHidden edgeState = iota
	edgeOut
	edgeDrop
)

const (
	edgeHandleWinW = 24
	edgeHandleWinH = 110
	edgeDropWinW   = 188
	edgeOutWinW    = 360
	edgePanelW     = 320
)

type edgeRow struct {
	sub model.Sub
	b   *model.Bookmark
}

type edgeMode struct {
	app             *app.App
	win             *mygo.Window
	state           edgeState
	side            string // "left" or "right"
	groupID         string
	menuOpen        bool
	autoHideOnLeave bool
	query           string
	searchInput     ui.Handle
	hoverSince      time.Time
	leaveSince      time.Time
	dragLeftAt      time.Time
	openedAt        time.Time
	needSlide       bool
	tmplID          string
	tmplQuery       string
	tmplInput       ui.Handle
	disp            mygo.Display
	unsubDisp       func()
	list            ui.ListState
	cache           struct {
		library  *model.Library
		revision int64
		groupID  string
		query    string
		rows     []edgeRow
	}
}

func normalizeEdgeSide(s string) string {
	if s == "left" {
		return "left"
	}
	return "right"
}

func syncEdge(a *app.App, on bool) {
	if !on {
		if curEdge != nil {
			curEdge.destroy()
			curEdge = nil
		}
		return
	}
	side := normalizeEdgeSide(a.Settings.EdgeSide)
	if curEdge == nil {
		curEdge = newEdgeMode(a, side)
		return
	}
	curEdge.app = a
	if curEdge.side != side {
		curEdge.side = side
		curEdge.applyBounds()
	}
	if curEdge.win != nil {
		curEdge.win.Invalidate()
	}
}

func newEdgeMode(a *app.App, side string) *edgeMode {
	e := &edgeMode{
		app:     a,
		side:    side,
		groupID: initialModeGroup(a.Settings.EdgeGroup, a.Group),
		disp:    cursorDisplay(),
	}
	x, y, w, h := e.boundsFor(edgeHidden)
	e.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             locales.ModeEdgeTitle(),
		X:                 x,
		Y:                 y,
		Width:             w,
		Height:            h,
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		Panel:             true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		DisableShadow:     true,
		Content:           ui.View(e.view),
	})
	e.win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: w, Height: h})
	a.AddWindow(e.win)
	e.win.ShowInactive()
	e.win.OnBlur(func() {
		if !e.menuOpen && e.state != edgeHidden && time.Since(e.openedAt) > 150*time.Millisecond {
			e.collapse()
		}
	})
	e.win.OnClose(func(ev *mygo.CloseEvent) {
		ev.PreventDefault()
		e.collapse()
	})
	e.unsubDisp = mygo.Screen.OnDisplaysChanged(func() {
		if curEdge == e && e.state == edgeHidden {
			e.disp = cursorDisplay()
			e.applyBounds()
		}
	})
	return e
}

func (e *edgeMode) destroy() {
	if e.unsubDisp != nil {
		e.unsubDisp()
		e.unsubDisp = nil
	}
	if e.win != nil {
		w := e.win
		e.win = nil
		w.Destroy()
	}
}

func (e *edgeMode) boundsFor(st edgeState) (x, y, w, h int) {
	d := e.disp
	if d.Bounds.Width <= 0 {
		d = cursorDisplay()
		e.disp = d
	}
	wa := displayRect(d)
	switch st {
	case edgeOut:
		w = edgeOutWinW
		h = max(360, wa.Height-48)
		y = wa.Y + 24
	case edgeDrop:
		groups := len(e.app.Store.Lib().Groups)
		w = edgeDropWinW
		h = min(max(wa.Height-80, 220), max(220, groups*52+40))
		y = wa.Y + (wa.Height-h)/2
	default:
		w = edgeHandleWinW
		h = edgeHandleWinH
		y = wa.Y + (wa.Height-h)/2
	}
	if e.side == "left" {
		x = wa.X
	} else {
		x = wa.X + wa.Width - w
	}
	return x, y, w, h
}

func (e *edgeMode) applyBounds() {
	if e.win == nil {
		return
	}
	x, y, w, h := e.boundsFor(e.state)
	e.win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: w, Height: h})
}

func (e *edgeMode) setState(st edgeState) {
	if e.state == st {
		return
	}
	wasFocused := st == edgeHidden && e.win != nil && e.win.IsFocused()
	e.state = st
	e.hoverSince = time.Time{}
	e.leaveSince = time.Time{}
	e.dragLeftAt = time.Time{}
	if st == edgeHidden {
		e.menuOpen = false
		e.autoHideOnLeave = false
		e.query = ""
		e.tmplID = ""
		e.tmplQuery = ""
		e.list.ScrollTo(0, ui.Start)
	} else {
		e.disp = cursorDisplay()
		e.openedAt = time.Now()
		e.needSlide = st == edgeOut
	}
	if wasFocused {
		e.win.Hide()
	}
	e.applyBounds()
	if e.win != nil {
		if st == edgeOut {
			e.win.Show()
			e.win.Focus()
			e.searchInput.SelectAll()
		} else if wasFocused {
			e.win.ShowInactive()
		}
		e.win.Invalidate()
	}
}

func (e *edgeMode) expand(autoHide bool) {
	e.autoHideOnLeave = autoHide
	if e.state == edgeOut {
		if e.win != nil {
			e.win.Show()
			e.win.Focus()
		}
		e.searchInput.SelectAll()
		return
	}
	e.setState(edgeOut)
}

func (e *edgeMode) collapse() {
	e.setState(edgeHidden)
}

func (e *edgeMode) view(c *ui.Context) {
	menuOpenThisFrame := false
	defer func() {
		if e.state == edgeHidden {
			e.menuOpen = false
			return
		}
		e.menuOpen = menuOpenThisFrame
	}()

	p := theme.Apply(c, e.app.Settings.Appearance)
	c.Root().Background(ui.Transparent)

	lib := e.app.Store.Lib()
	if e.tmplID != "" && templateBookmark(lib, e.tmplID) == nil {
		e.tmplID = ""
	}

	if !e.menuOpen && e.state == edgeOut && c.Shortcut(ui.Cmd, ui.KeyF) && !c.ShortcutRepeated() {
		e.searchInput.SelectAll(c)
	}

	if !e.menuOpen && e.state == edgeOut && !e.searchInput.Focused(c) && !e.tmplInput.Focused(c) {
		if ch, ok := widget.TypedSearchKey(c); ok {
			e.query += ch
			e.list.ScrollTo(0, ui.Start)
			e.searchInput.Focus(c)
		}
	}

	if g := activeGroup(lib, e.groupID); g != nil {
		e.groupID = g.ID
	} else {
		e.groupID = ""
	}

	switch e.state {
	case edgeHidden:
		e.viewHandle(c, p)
	case edgeDrop:
		e.viewDrop(c, p, lib)
	case edgeOut:
		e.viewPanel(c, p, lib, &menuOpenThisFrame)
	}

	// Evaluate Escape after TextInputBase so e.query is synced with text typed this frame.
	if !e.menuOpen && !menuOpenThisFrame && e.state == edgeOut && c.Shortcut(0, ui.KeyEscape) {
		if e.tmplID != "" {
			e.tmplID = ""
			e.tmplQuery = ""
			e.searchInput.Focus(c)
			if e.win != nil {
				e.win.Invalidate()
			}
		} else if e.query != "" {
			e.query = ""
			e.list.ScrollTo(0, ui.Start)
			e.searchInput.Focus(c)
			if e.win != nil {
				e.win.Invalidate()
			}
		} else {
			e.collapse()
		}
	}
}

func (e *edgeMode) viewHandle(c *ui.Context, p *theme.Palette) {
	root := ui.Row(c.Key("eg-root")).Fill().AlignItems(ui.Center)
	if e.side == "left" {
		root.Justify(ui.Start)
	} else {
		root.Justify(ui.End)
	}

	dropOpts := urlDropOptions()
	_, dragOver := ui.DataDragOver(root, dropOpts)
	if dragOver {
		e.setState(edgeDrop)
		return
	}
	if drop, ok := ui.DropData(root, dropOpts); ok {
		if raw := extractDropURL(drop.Data); raw != "" {
			var gid, sid string
			lib := e.app.Store.Lib()
			if len(lib.Groups) > 0 {
				g := activeGroup(lib, e.groupID)
				gid, sid = g.ID, firstSubID(g)
			}
			e.app.QuickAddToPlace(raw, "", gid, sid, false)
		}
	}

	root.Children(func() {
		btn := ui.ButtonBase(c.Key("eg-handle")).Size(22, 110).AlignItems(ui.Center).
			Label(locales.ActionExpandBookmarks()).Tooltip(locales.ActionExpandBookmarks()).FocusRing(false)
		if e.side == "left" {
			btn.Justify(ui.Start)
		} else {
			btn.Justify(ui.End)
		}
		barW := float32(6)
		barColor := p.Accent
		if btn.Hovered() {
			barW = 8
			barColor = p.Primary
		}
		shadowX := float32(-4)
		if e.side == "left" {
			shadowX = 4
		}
		btn.Children(func() {
			bar := ui.Box(c.Key("eg-bar")).Size(barW, 76).Background(barColor).
				Shadow(shadowX, 0, 14, -4, ui.RGBA(181, 88, 58, 0.5))
			if e.side == "left" {
				bar.Radius(0, 6, 6, 0)
			} else {
				bar.Radius(6, 0, 0, 6)
			}
		})
		if btn.Clicked() {
			e.expand(false)
			return
		}
		if btn.Hovered() {
			if e.hoverSince.IsZero() {
				e.hoverSince = time.Now()
				c.After(140 * time.Millisecond)
			} else if time.Since(e.hoverSince) >= 140*time.Millisecond {
				e.expand(true)
			} else {
				c.After(50 * time.Millisecond)
			}
		} else {
			e.hoverSince = time.Time{}
		}
	})
}

func (e *edgeMode) viewDrop(c *ui.Context, p *theme.Palette, lib *model.Library) {
	root := ui.Column(c.Key("eg-drop-root")).Fill().Justify(ui.Center).Gap(6).Padding(12, 0)
	if e.side == "left" {
		root.AlignItems(ui.Start)
	} else {
		root.AlignItems(ui.End)
	}

	dropOpts := urlDropOptions()
	_, rootOver := ui.DataDragOver(root, dropOpts)
	anyOver := rootOver
	if drop, ok := ui.DropData(root, dropOpts); ok {
		if raw := extractDropURL(drop.Data); raw != "" {
			var gid, sid string
			if len(lib.Groups) > 0 {
				g := activeGroup(lib, e.groupID)
				gid, sid = g.ID, firstSubID(g)
			}
			e.app.QuickAddToPlace(raw, "", gid, sid, false)
		}
		e.collapse()
		return
	}

	reduceTrans := theme.ReduceTransparency()
	root.Children(func() {
		if len(lib.Groups) == 0 {
			pill := ui.Row(c.Key("eg-drop-def")).Height(44).PaddingX(18).AlignItems(ui.Center).
				Background(p.Primary).TextColor(p.OnPrimary)
			if e.side == "left" {
				pill.Radius(0, 999, 999, 0)
			} else {
				pill.Radius(999, 0, 0, 999)
			}
			pill.Children(func() { ui.Text(c, locales.ModeDropSaveDefault()).FontWeight(500).SingleLine() })
			return
		}
		for i := range lib.Groups {
			g := &lib.Groups[i]
			pill := ui.Row(c.Key("eg-drop-" + g.ID)).AlignItems(ui.Center)
			if e.side == "left" {
				pill.Radius(0, 999, 999, 0)
			} else {
				pill.Radius(999, 0, 0, 999)
			}
			_, over := ui.DataDragOver(pill, dropOpts)
			sx := float32(-8)
			if e.side == "left" {
				sx = 8
			}
			if over {
				anyOver = true
				hotSX := float32(-10)
				padL, padR := float32(20), float32(22)
				if e.side == "left" {
					hotSX = 10
					padL, padR = 22, 20
				}
				pill.Height(48).Padding(0, padR, 0, padL).Background(p.Primary).TextColor(p.OnPrimary).
					Shadow(hotSX, 8, 30, -12, ui.RGBA(181, 88, 58, 0.7))
			} else {
				pillBg := p.Bg.Alpha(0.94)
				if reduceTrans {
					pillBg = p.Bg
				}
				pill.Height(38).Padding(0, 16).Background(pillBg).TextColor(p.Text2).
					Shadow(sx, 6, 24, -14, ui.RGBA(60, 35, 20, 0.45))
			}
			pill.Children(func() {
				t := ui.Text(c, g.Name).SingleLine()
				if over {
					t.FontSize(14).FontWeight(500)
				}
			})
			if drop, ok := ui.DropData(pill, dropOpts); ok {
				if raw := extractDropURL(drop.Data); raw != "" {
					e.app.QuickAddToPlace(raw, "", g.ID, firstSubID(g), false)
				}
				e.collapse()
				return
			}
		}
	})

	if anyOver {
		e.dragLeftAt = time.Time{}
	} else {
		if e.dragLeftAt.IsZero() {
			e.dragLeftAt = time.Now()
			c.After(160 * time.Millisecond)
		} else if time.Since(e.dragLeftAt) >= 140*time.Millisecond {
			e.collapse()
		} else {
			c.After(60 * time.Millisecond)
		}
	}
}

func (e *edgeMode) viewPanel(c *ui.Context, p *theme.Palette, lib *model.Library, menuOpen *bool) {
	wrap := ui.Row(c.Key("eg-wrap")).Fill()
	if e.side == "left" {
		wrap.Justify(ui.Start).Padding(14, 30, 14, 10)
	} else {
		wrap.Justify(ui.End).Padding(14, 10, 14, 30)
	}

	wrap.Children(func() {
		shadowX := float32(-24)
		if e.side == "left" {
			shadowX = 24
		}
		panelBg := p.Bg.Alpha(0.97)
		if theme.ReduceTransparency() {
			panelBg = p.Bg
		}
		panel := ui.Column(c.Key("eg-panel")).Width(edgePanelW).FillHeight().Shrink(0).
			Background(panelBg).Radius(16).Border(1, p.Line).
			Shadow(shadowX, 0, 60, -28, ui.RGBA(60, 35, 20, 0.5))

		// Slide-in animation from the screen edge
		fromX := float32(edgePanelW + 24)
		if e.side == "left" {
			fromX = -fromX
		}
		if e.needSlide {
			panel.Jump("slide", fromX)
			e.needSlide = false
		}
		slide := panel.AnimateTo("slide", 0, ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond, Bounce: 0.08}})
		panel.Translate(slide, 0)

		// Collapse hover-opened panel when pointer leaves for 600ms (unless typing, template, or menu open).
		_, _, over := wrap.PointerPosition()
		if !e.autoHideOnLeave || over || e.menuOpen || e.query != "" || e.tmplID != "" || e.tmplInput.Focused(c) {
			e.leaveSince = time.Time{}
		} else if delay := 400*time.Millisecond - time.Since(e.openedAt); delay > 0 {
			c.After(delay)
		} else {
			if e.leaveSince.IsZero() {
				e.leaveSince = time.Now()
				c.After(600 * time.Millisecond)
			} else if time.Since(e.leaveSince) >= 600*time.Millisecond {
				e.collapse()
				return
			} else {
				c.After(80 * time.Millisecond)
			}
		}

		// Accept URL drop on open panel too.
		dropOpts := urlDropOptions()
		if _, over := ui.DataDragOver(panel, dropOpts); over {
			panel.Border(1.5, p.Accent)
		}
		if drop, ok := ui.DropData(panel, dropOpts); ok {
			if raw := extractDropURL(drop.Data); raw != "" {
				var gid, sid string
				if len(lib.Groups) > 0 {
					g := activeGroup(lib, e.groupID)
					gid, sid = g.ID, firstSubID(g)
				}
				e.app.QuickAddToPlace(raw, "", gid, sid, false)
			}
		}

		panel.Children(func() {
			defer widget.Toast(c, e.app, p, 12)
			// Top row: group tabs + new + collapse, followed by persistent auto-focused search input.
			ui.Row(c.Key("eg-hdr")).Height(46).Padding(0, 8, 0, 10).Gap(6).AlignItems(ui.Center).Children(func() {
				if len(lib.Groups) > 0 {
					scroll := widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("eg-tabs"))).
						Grow(1).MinWidth(0).Height(30).ScrollbarInsets(0, 2, 0, 2).Padding(2, 0)
					scroll.Children(func() {
						ui.Row(c.Key("eg-tabs-row")).Gap(4).AlignItems(ui.Center).Children(func() {
							for i := range lib.Groups {
								g := &lib.Groups[i]
								selected := g.ID == e.groupID && strings.TrimSpace(e.query) == ""
								pill := widget.Pill(c.Key("eg-grp-"+g.ID), p, g.Name, selected)
								widget.RevealSelectedTab(pill, selected, i)
								if pill.Clicked() {
									e.groupID = g.ID
									e.app.SetModeGroup(model.ModeEdge, g.ID)
									e.query = ""
									e.tmplID = ""
									e.list.ScrollTo(0, ui.Start)
									e.searchInput.Focus(c)
								}
							}
						})
					})
				} else {
					ui.Spacer(c)
				}

				addBtn := widget.IconButton(c.Key("eg-add"), p, theme.IconPlus, locales.MenuNewBookmark()).Shrink(0)
				if addBtn.Clicked() {
					e.collapse()
					if e.app.Win.ShowEditor != nil {
						var gid, sid string
						if len(lib.Groups) > 0 {
							g := activeGroup(lib, e.groupID)
							gid, sid = g.ID, firstSubID(g)
						}
						e.app.Win.ShowEditor(app.EditorRequest{GroupID: gid, SubID: sid})
					}
				}
				collapseIcon := theme.IconCollapseRight
				if e.side == "left" {
					collapseIcon = theme.IconCollapseLeft
				}
				hideBtn := widget.IconButton(c.Key("eg-hide"), p, collapseIcon, locales.ActionCollapseEdge()).Shrink(0)
				if hideBtn.Clicked() {
					e.collapse()
				}
			})

			ui.Row(c.Key("eg-srow")).Padding(0, 10, 8, 10).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(p.LineSoft).Children(func() {
				renderModeSearchBox(c, p, "eg", &e.query, &e.searchInput, p.Field,
					func() { e.list.ScrollTo(0, ui.Start) },
					func() {
						hits := searchBookmarks(e.app, e.query, "")
						if len(hits) > 0 {
							e.openItem(hits[0])
							if e.tmplID != "" {
								e.tmplInput.Focus(c)
							}
						}
					},
				)
			})

			q := strings.TrimSpace(e.query)

			if b := templateBookmark(lib, e.tmplID); b != nil {
				ui.Box(c.Key("eg-tmpl-wrap")).Padding(6, 10, 4, 10).Children(func() {
					renderTemplateBar(c, e.app, b, &e.tmplQuery, &e.tmplInput, p.Dark, func() {
						e.tmplID = ""
						e.tmplQuery = ""
						if e.state == edgeOut {
							e.searchInput.Focus(c)
						}
					})
				})
			}

			// Body: search results OR sub-group sections
			rows := e.currentRows(lib, q)
			if len(rows) == 0 {
				msg := locales.MainNoBookmarks()
				size := float32(14)
				col := p.Muted
				if q != "" {
					msg = locales.MainNoMatchingBookmarks()
					size = 15
					col = p.Text2
				} else if len(lib.Groups) == 0 {
					msg = locales.MainNoGroups()
				}
				ui.Column(c.Key("eg-empty")).Grow(1).Center().Padding(60, 16).Children(func() {
					ui.Text(c, msg).Font(theme.TitleFont(c)).FontSize(size).TextColor(col).SingleLine()
				})
				return
			}
			widget.AutoScrollbar(c, ui.List(c.Key("eg-list"), &e.list, len(rows))).Grow(1).MinHeight(0).FillWidth().
				Padding(4, 6, 14, 6).Gap(1).
				ItemKey(func(i int) any {
					r := rows[i]
					if r.b != nil {
						return "bm-" + r.b.ID
					}
					return "shdr-" + r.sub.ID
				}).
				Rows(func(row ui.ListRow) {
					c := row.Context
					r := rows[row.Index]
					if r.b == nil {
						top := float32(10)
						if row.Index > 0 {
							top = 9
						}
						ui.Box(c.Key("eg-shdr-"+r.sub.ID)).Padding(top, 0, 3, 8).Children(func() {
							widget.Title(c, p, r.sub.Name, 13)
						})
						return
					}
					e.renderRow(c, p, r.b, menuOpen)
				})
		})
	})
}

func (e *edgeMode) currentRows(lib *model.Library, q string) []edgeRow {
	if e.cache.library == lib && e.cache.revision == lib.Revision && e.cache.groupID == e.groupID && e.cache.query == q {
		return e.cache.rows
	}
	var rows []edgeRow
	if q != "" {
		for _, bm := range searchBookmarks(e.app, q, "") {
			rows = append(rows, edgeRow{b: bm})
		}
	} else if len(lib.Groups) > 0 {
		g := activeGroup(lib, e.groupID)
		for _, sub := range g.Subs {
			items := lib.In(g.ID, sub.ID)
			if len(items) == 0 {
				continue
			}
			rows = append(rows, edgeRow{sub: sub})
			for _, bm := range items {
				rows = append(rows, edgeRow{b: bm})
			}
		}
	}
	e.cache.library = lib
	e.cache.revision = lib.Revision
	e.cache.groupID = e.groupID
	e.cache.query = q
	e.cache.rows = rows
	return rows
}

func (e *edgeMode) renderRow(c *ui.Context, p *theme.Palette, b *model.Bookmark, menuOpen *bool) {
	row := ui.ButtonBase(c.Key("eg-bm-" + b.ID)).Height(34).PaddingX(8).Gap(10).
		Radius(8).Justify(ui.Start).AlignItems(ui.Center).Label(b.Title).Tooltip(b.Title)
	if row.Hovered() {
		row.Background(p.Hover)
	}
	row.Children(func() {
		theme.Tile(c, b.Icon, e.app.Store.IconPath(b.Icon), 22).Radius(6)
		ui.Text(c, b.Title).Grow(1).TextColor(p.Text).SingleLine()
	})
	if row.Clicked() {
		e.openItem(b)
		if e.tmplID != "" {
			e.tmplInput.Focus(c)
		}
	}
	bindBookmarkPopoverMenu(c, e.app, row, b, modeMenuOptions(p), menuOpen,
		func() {
			e.openItem(b)
			if e.tmplID != "" {
				e.tmplInput.Focus(c)
			}
		},
		e.collapse,
		nil,
	)
}

func (e *edgeMode) openItem(b *model.Bookmark) {
	if b.IsTemplate() {
		e.tmplID = b.ID
		e.tmplQuery = ""
		return
	}
	e.app.Open(b, "")
}
