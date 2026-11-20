package mode

import (
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

type notchState uint8

const (
	notchClosed notchState = iota
	notchOpen
	notchDrop
)

const (
	notchClosedWinW = 236
	notchClosedWinH = 40
	notchDropWinW   = 480
	notchDropWinH   = 180
	notchOpenWinW   = 740
	notchGridRowH   = 80
)

type notchMode struct {
	app          *app.App
	win          *mygo.Window
	state        notchState
	groupID      string
	searching    bool
	query        string
	searchInput  ui.Handle
	hoverSince   time.Time
	hoverBlocked bool
	dragLeftAt   time.Time
	openedAt     time.Time
	tmplID       string
	tmplQuery    string
	tmplInput    ui.Handle
	disp         mygo.Display
	unsubDisp    func()
	list         ui.ListState
	cache        struct {
		library   *model.Library
		revision  int64
		groupID   string
		searching bool
		query     string
		items     []*model.Bookmark
	}
}

func syncNotch(a *app.App, on bool) {
	if !on {
		if curNotch != nil {
			curNotch.destroy()
			curNotch = nil
		}
		return
	}
	if curNotch == nil {
		curNotch = newNotchMode(a)
		return
	}
	curNotch.app = a
	if curNotch.state == notchClosed {
		curNotch.placeClosed(cursorDisplay())
	}
	if curNotch.win != nil {
		curNotch.win.Invalidate()
	}
}

func newNotchMode(a *app.App) *notchMode {
	n := &notchMode{
		app:  a,
		disp: cursorDisplay(),
	}
	x, y, w, h := n.boundsFor(notchClosed, 1)
	n.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Cooool Mark 刘海",
		X:                 x,
		Y:                 y,
		Width:             w,
		Height:            h,
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
		Content:           ui.View(n.view),
	})
	prepareNotchWindow(n.win)
	n.win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: w, Height: h})
	a.AddWindow(n.win)
	n.win.OnBlur(func() {
		if n.state != notchClosed && time.Since(n.openedAt) > 150*time.Millisecond {
			n.collapse()
		}
	})
	n.win.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		n.collapse()
	})
	n.unsubDisp = mygo.Screen.OnDisplaysChanged(func() {
		if curNotch == n && n.state == notchClosed {
			n.placeClosed(cursorDisplay())
		}
	})
	return n
}

func (n *notchMode) destroy() {
	if n.unsubDisp != nil {
		n.unsubDisp()
		n.unsubDisp = nil
	}
	if n.win != nil {
		w := n.win
		n.win = nil
		w.Destroy()
	}
}

func (n *notchMode) boundsFor(st notchState, rows int) (x, y, w, h int) {
	d := n.disp
	if d.Bounds.Width <= 0 {
		d = cursorDisplay()
		n.disp = d
	}
	switch st {
	case notchOpen:
		w = notchOpenWinW
		h = n.panelHeight(rows) + 52
	case notchDrop:
		w = notchDropWinW
		h = notchDropWinH
	default:
		w = notchClosedWinW
		h = notchClosedWinH
	}
	x = d.Bounds.X + (d.Bounds.Width-w)/2
	y = d.Bounds.Y
	return x, y, w, h
}

func (n *notchMode) panelHeight(rows int) int {
	height := 92 + rows*notchGridRowH
	if n.tmplID != "" {
		height += 48
	}
	return height
}

func (n *notchMode) placeClosed(d mygo.Display) {
	if n.win == nil {
		return
	}
	n.disp = d
	x, y, w, h := n.boundsFor(notchClosed, 1)
	n.win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: w, Height: h})
}

func (n *notchMode) currentItems(lib *model.Library) []*model.Bookmark {
	q := strings.TrimSpace(n.query)
	if n.cache.library == lib && n.cache.revision == lib.Revision && n.cache.groupID == n.groupID && n.cache.searching == n.searching && n.cache.query == q {
		return n.cache.items
	}
	var items []*model.Bookmark
	if n.searching && q != "" {
		items = searchBookmarks(n.app, q, "")
	} else if len(lib.Groups) > 0 {
		items = groupBookmarks(lib, activeGroup(lib, n.groupID))
	}
	n.cache.library = lib
	n.cache.revision = lib.Revision
	n.cache.groupID = n.groupID
	n.cache.searching = n.searching
	n.cache.query = q
	n.cache.items = items
	return items
}

func (n *notchMode) currentRows() int {
	lib := n.app.Store.Lib()
	count := len(n.currentItems(lib))
	rows := int(math.Ceil(float64(max(count, 1)) / 6.0))
	return min(max(rows, 1), 3)
}

func (n *notchMode) setState(st notchState) {
	if n.state == st {
		return
	}
	n.state = st
	n.hoverSince = time.Time{}
	n.dragLeftAt = time.Time{}
	if st == notchClosed {
		n.searching = false
		n.query = ""
		n.tmplID = ""
		n.tmplQuery = ""
		n.list.ScrollTo(0, ui.Start)
	} else {
		n.disp = cursorDisplay()
		n.openedAt = time.Now()
	}
	if n.win == nil {
		return
	}
	rows := n.currentRows()
	x, y, w, h := n.boundsFor(st, rows)
	n.win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: w, Height: h})
	if st == notchOpen {
		n.win.Show()
		n.win.Focus()
	}
	n.win.Invalidate()
}

func (n *notchMode) expand() {
	n.setState(notchOpen)
}

func (n *notchMode) collapse() {
	n.hoverBlocked = true
	n.setState(notchClosed)
}

func (n *notchMode) syncOpenHeight() {
	if n.win == nil || n.state != notchOpen {
		return
	}
	x, y, w, h := n.boundsFor(notchOpen, n.currentRows())
	b := n.win.Bounds()
	if b.Width != w || b.Height != h || b.X != x || b.Y != y {
		n.win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: w, Height: h})
	}
}

func (n *notchMode) view(c *ui.Context) {
	p := theme.Apply(c, n.app.Settings.Appearance)
	// Keep Notch's window theme dark so no widget or scrollbar inherits light surfaces.
	base := c.Theme()
	nt := *ui.DarkTheme()
	nt.Font = base.Font
	nt.FontSize = base.FontSize
	nt.Spacing = base.Spacing
	nt.Radius = 9
	nt.Background = p.Notch
	nt.Surface = ui.RGBA(255, 255, 255, 0.12)
	nt.SurfaceHover = ui.RGBA(255, 255, 255, 0.16)
	nt.SurfacePressed = ui.RGBA(255, 255, 255, 0.20)
	nt.Border = ui.RGBA(255, 255, 255, 0.10)
	nt.Text = ui.Hex("#eae8e1")
	nt.TextMuted = ui.RGBA(234, 232, 225, 0.55)
	nt.Focus = ui.Hex("#e0875f")
	nt.Selection = ui.Hex("#e0875f").Alpha(0.28)
	sb := theme.ScrollbarStyle(true)
	nt.ScrollbarWidth = sb.ThumbWidth
	nt.Scrollbar = sb.Thumb
	nt.ScrollbarStyle = sb
	nt.Inverse = ui.Hex("#1f1e1d")
	nt.InverseText = ui.Hex("#eae8e1")
	c.SetTheme(&nt)
	c.Root().Background(ui.Transparent)

	lib := n.app.Store.Lib()
	if n.tmplID != "" && templateBookmark(lib, n.tmplID) == nil {
		n.tmplID = ""
	}

	if n.state == notchOpen && c.Shortcut(0, ui.KeyEscape) {
		if n.tmplID != "" {
			n.tmplID = ""
			n.tmplQuery = ""
			n.syncOpenHeight()
		} else if n.searching {
			n.searching = false
			n.query = ""
			n.list.ScrollTo(0, ui.Start)
			n.syncOpenHeight()
		} else {
			n.collapse()
			return
		}
	}

	if g := activeGroup(lib, n.groupID); g != nil {
		n.groupID = g.ID
	} else {
		n.groupID = ""
	}

	var targetW, targetH, targetR float32
	switch n.state {
	case notchOpen:
		targetW = 640
		targetH = float32(n.panelHeight(n.currentRows()))
		targetR = 30
	case notchDrop:
		targetW = 420
		targetH = 128
		targetR = 26
	default:
		targetW = 200
		targetH = 32
		targetR = 14
	}

	// Root drop receiver so dragging a URL anywhere onto the notch triggers drop mode.
	dropOpts := urlDropOptions()
	root := ui.Column(c).Fill().AlignItems(ui.Center)
	_, dragOver := ui.DataDragOver(root, dropOpts)
	if drop, ok := ui.DropData(root, dropOpts); ok {
		if raw := extractDropURL(drop.Data); raw != "" {
			var gid, sid string
			if len(lib.Groups) > 0 {
				g := activeGroup(lib, n.groupID)
				gid, sid = g.ID, firstSubID(g)
			}
			saveDroppedURL(n.app, raw, gid, sid)
		}
		n.collapse()
	}

	if dragOver {
		n.dragLeftAt = time.Time{}
		if n.state != notchDrop {
			n.setState(notchDrop)
		}
	} else if n.state == notchDrop {
		if n.dragLeftAt.IsZero() {
			n.dragLeftAt = time.Now()
			c.After(140 * time.Millisecond)
		} else if time.Since(n.dragLeftAt) >= 120*time.Millisecond {
			n.collapse()
		} else {
			c.After(60 * time.Millisecond)
		}
	}

	var animW float32
	// Draw concave circular ears on top-left and top-right of the black capsule.
	root.DrawOver(func(pt *ui.Painter, r ui.Rect) {
		left := (r.W - animW) / 2
		right := left + animW
		const ear = float32(14)
		const k = 0.55228475 * ear
		var lp ui.Path
		lp.MoveTo(left-ear, 0).LineTo(left+1, 0).LineTo(left+1, ear).LineTo(left, ear).
			CubeTo(left, ear-k, left-ear+k, 0, left-ear, 0).Close()
		pt.FillPath(&lp, p.Notch)
		var rp ui.Path
		rp.MoveTo(right+ear, 0).LineTo(right-1, 0).LineTo(right-1, ear).LineTo(right, ear).
			CubeTo(right, ear-k, right+ear-k, 0, right+ear, 0).Close()
		pt.FillPath(&rp, p.Notch)
	})

	// Hover-to-open on the collapsed capsule (300ms dwell) and click toggle.
	root.Children(func() {
		// Animated black capsule container
		spring := ui.Animation{Spring: ui.SpringOptions{Duration: 380 * time.Millisecond, Bounce: 0.16}}
		panel := ui.Column(c.Key("notch-panel"))
		animW = panel.AnimateTo("w", targetW, spring)
		animH := panel.AnimateTo("h", targetH, spring)
		animR := panel.AnimateTo("r", targetR, spring)

		panel.Size(animW, animH).Shrink(0).Background(p.Notch).Radius(0, 0, animR, animR)
		if math.Abs(float64(animW-targetW)) > 0.5 || math.Abs(float64(animH-targetH)) > 0.5 {
			panel.Clip()
		}
		if n.state != notchClosed {
			panel.Shadow(0, 30, 70, -24, ui.RGBA(0, 0, 0, 0.55))
		}

		panel.Children(func() {
			// Top notch trigger strip (200x32 centered at top)
			trigLabel := "展开书签"
			if n.state == notchOpen {
				trigLabel = "收起书签"
			}
			trigger := ui.ButtonBase(c.Key("notch-trigger")).Absolute().
				Left((animW-200)/2).Top(0).Size(200, 32).FocusRing(false)
			trigger.Label(trigLabel).Tooltip(trigLabel)
			if trigger.Clicked() {
				if n.state == notchOpen {
					n.collapse()
				} else {
					n.expand()
				}
			}
			if n.state == notchClosed {
				hovered := trigger.Hovered() || panel.Hovered()
				if !hovered {
					n.hoverBlocked = false
					n.hoverSince = time.Time{}
				} else if !n.hoverBlocked {
					if n.hoverSince.IsZero() {
						n.hoverSince = time.Now()
						c.After(300 * time.Millisecond)
					} else if time.Since(n.hoverSince) >= 300*time.Millisecond {
						n.expand()
					} else {
						c.After(50 * time.Millisecond)
					}
				}
			}

			switch n.state {
			case notchDrop:
				n.renderDrop(c, lib)
			case notchOpen:
				n.renderOpen(c, p, lib)
			}
		})
	})
}

func (n *notchMode) renderDrop(c *ui.Context, lib *model.Library) {
	groupName := "默认分组"
	if len(lib.Groups) > 0 {
		groupName = activeGroup(lib, n.groupID).Name
	}
	zone := ui.Row(c.Key("notch-drop")).Absolute().Left(14).Right(14).Top(40).Bottom(14).
		Radius(18).Border(1.5, ui.RGBA(224, 135, 95, 0.75)).BorderStyle(ui.BorderDashed).
		Background(ui.RGBA(224, 135, 95, 0.10)).Center().Gap(10)
	zone.Children(func() {
		ui.Icon(c, theme.IconPlus).FontSize(18).TextColor(ui.Hex("#f0a07c"))
		ui.Text(c, "存到「"+groupName+"」").FontSize(14).FontWeight(500).TextColor(ui.Hex("#f0a07c")).SingleLine()
	})
}

func (n *notchMode) renderOpen(c *ui.Context, p *theme.Palette, lib *model.Library) {
	body := ui.Column(c.Key("notch-body")).Fill().Padding(40, 20, 18, 20).Gap(14)
	body.Children(func() {
		defer widget.Toast(c, n.app, p, 12)
		// Header: group pills (or search input) + Search button + Open Main button
		ui.Row(c.Key("notch-hdr")).Height(30).AlignItems(ui.Center).Gap(2).Children(func() {
			if n.searching {
				sbox := ui.Row(c.Key("notch-sbox")).Grow(1).Height(30).PaddingX(10).Gap(7).Radius(9).
					Background(ui.RGBA(255, 255, 255, 0.12)).AlignItems(ui.Center)
				sbox.Children(func() {
					ui.Icon(c, theme.IconSearch).FontSize(14).TextColor(ui.RGBA(234, 232, 225, 0.65))
					in := ui.TextInputBase(c.Key("notch-q"), &n.query).Bind(&n.searchInput).Grow(1).
						Label("搜索书签").Placeholder("搜索书签").TextColor(ui.Hex("#ffffff")).
						PlaceholderColor(ui.RGBA(234, 232, 225, 0.60))
					if in.Changed() {
						n.list.ScrollTo(0, ui.Start)
						n.syncOpenHeight()
					}
					if in.Submitted() {
						hits := searchBookmarks(n.app, n.query, "")
						if len(hits) > 0 {
							n.openItem(hits[0])
							if n.tmplID != "" {
								n.tmplInput.Focus(c)
							}
						}
					}
				})
				closeBtn := notchIconBtn(c, "notch-sclose", theme.IconClose, "关闭搜索")
				if closeBtn.Clicked() {
					n.searching = false
					n.query = ""
					n.list.ScrollTo(0, ui.Start)
					n.syncOpenHeight()
				}
			} else {
				scroll := widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("notch-tabs"))).MinWidth(0).Shrink(0).ScrollbarInsets(0, 2, 0, 2).Padding(0, 0, 4, 0).Grow(1).Height(30)
				scroll.Children(func() {
					ui.Row(c.Key("notch-tabs-row")).Gap(4).AlignItems(ui.Center).Children(func() {
						for i := range lib.Groups {
							g := &lib.Groups[i]
							sel := g.ID == n.groupID
							pill := ui.ButtonBase(c.Key(g.ID)).Height(26).PaddingX(10).Radius(999).
								MaxWidth(156).Shrink(0).Label(g.Name).Tooltip(g.Name).AlignItems(ui.Center)
							widget.RevealSelectedTab(pill, sel, i)
							if sel {
								pill.Background(ui.RGBA(255, 255, 255, 0.16)).TextColor(ui.Hex("#ffffff"))
							} else if pill.Hovered() {
								pill.Background(ui.RGBA(255, 255, 255, 0.10)).TextColor(ui.Hex("#eae8e1"))
							} else {
								pill.TextColor(ui.RGBA(234, 232, 225, 0.60))
							}
							pill.Children(func() {
								t := ui.Text(c, g.Name).SingleLine()
								if sel {
									t.FontWeight(500)
								}
							})
							if pill.Clicked() {
								n.groupID = g.ID
								n.tmplID = ""
								n.list.ScrollTo(0, ui.Start)
								n.syncOpenHeight()
							}
						}
					})
				})
				searchBtn := notchIconBtn(c, "notch-sbtn", theme.IconSearch, "搜索")
				if searchBtn.Clicked() {
					n.searching = true
					n.searchInput.Focus(c)
					n.list.ScrollTo(0, ui.Start)
					n.syncOpenHeight()
				}
			}
			mainBtn := notchIconBtn(c, "notch-main", theme.IconExternal, "打开主窗口")
			if mainBtn.Clicked() {
				n.collapse()
				if n.app.Win.ShowMain != nil {
					n.app.Win.ShowMain()
				}
			}
		})

		if b := templateBookmark(lib, n.tmplID); b != nil {
			renderTemplateBar(c, n.app, b, &n.tmplQuery, &n.tmplInput, true, func() {
				n.tmplID = ""
				n.tmplQuery = ""
				n.syncOpenHeight()
			})
		}

		q := strings.TrimSpace(n.query)
		items := n.currentItems(lib)

		if len(items) == 0 {
			msg := "还没有书签"
			if n.searching && q != "" {
				msg = "还没有匹配的书签"
			} else if len(lib.Groups) == 0 {
				msg = "还没有分组"
			}
			ui.Column(c.Key("notch-empty")).FillWidth().Grow(1).Center().Children(func() {
				ui.Text(c, msg).Font(theme.Serif).FontSize(14).TextColor(ui.RGBA(234, 232, 225, 0.72)).SingleLine()
			})
			return
		}

		const cols = 6
		numRows := (len(items) + cols - 1) / cols
		widget.AutoScrollbar(c, ui.List(c.Key("notch-grid-list"), &n.list, numRows)).Grow(1).MinHeight(0).FillWidth().Gap(14).
			ItemKey(func(i int) any { return "row-" + items[i*cols].ID }).
			Rows(func(row ui.ListRow) {
				c := row.Context
				first := row.Index * cols
				rowItems := items[first:min(first+cols, len(items))]
				ui.Grid(c).Columns(cols).GapX(6).Children(func() {
					for _, bm := range rowItems {
						b := bm
						tile := ui.ButtonBase(c.Key("nt-" + b.ID)).Padding(0).Radius(12).
							AlignItems(ui.Center).Label(b.Title).Tooltip(b.Title)
						if tile.Hovered() {
							tile.Background(ui.RGBA(255, 255, 255, 0.08))
						}
						tile.Children(func() {
							ui.Column(c).AlignItems(ui.Center).Gap(7).FillWidth().Children(func() {
								sq := theme.Tile(c, b.Icon, n.app.Store.IconPath(b.Icon), 44).Radius(12).
									Border(1, ui.RGBA(255, 255, 255, 0.08))
								if tile.Hovered() {
									sq.Translate(0, -2).Scale(1.04, 1.04)
								}
								ui.Text(c, b.Title).FontSize(12).TextColor(ui.Hex("#e8e6df")).SingleLine()
							})
						})
						if tile.Clicked() {
							n.openItem(b)
							if n.tmplID != "" {
								n.tmplInput.Focus(c)
							}
						}
					}
				})
			})
	})
}

func (n *notchMode) openItem(b *model.Bookmark) {
	if b.IsTemplate() {
		n.tmplID = b.ID
		n.tmplQuery = ""
		n.syncOpenHeight()
		return
	}
	n.app.Open(b, "")
}

func notchIconBtn(c *ui.Context, key string, icon *ui.SVG, label string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(30, 30).Radius(9).Center().
		TextColor(ui.Hex("#d9d7d0")).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(15) })
	return b
}
