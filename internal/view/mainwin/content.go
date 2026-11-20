package mainwin

import (
	"cmp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

type browseRowKind uint8

const (
	browseRowHeader browseRowKind = iota
	browseRowEmpty
	browseRowListItem
	browseRowTail
	browseRowGridRow
)

type browseRow struct {
	kind      browseRowKind
	sub       model.Sub
	count     int
	b         *model.Bookmark
	items     []*model.Bookmark
	lastInSub bool
}

func hasLiveBookmark(lib *model.Library) bool {
	for i := range lib.Bookmarks {
		if !lib.Bookmarks[i].InTrash() {
			return true
		}
	}
	return false
}

func hasGroupBookmark(lib *model.Library, groupID string) bool {
	for i := range lib.Bookmarks {
		b := &lib.Bookmarks[i]
		if !b.InTrash() && b.GroupID == groupID {
			return true
		}
	}
	return false
}

func (s *mainState) ensureGroupCache(lib *model.Library, g *model.Group) (subs [][]*model.Bookmark, ids []string) {
	c := &s.groupCache
	if c.library == lib && c.revision == lib.Revision && c.groupID == g.ID {
		return c.subs, c.ids
	}
	subs = make([][]*model.Bookmark, len(g.Subs))
	subIdx := make(map[string]int, len(g.Subs))
	for i, sub := range g.Subs {
		subIdx[sub.ID] = i
	}
	total := 0
	for i := range lib.Bookmarks {
		b := &lib.Bookmarks[i]
		if !b.InTrash() && b.GroupID == g.ID {
			if idx, ok := subIdx[b.SubID]; ok {
				subs[idx] = append(subs[idx], b)
				total++
			}
		}
	}
	ids = make([]string, 0, total)
	for _, list := range subs {
		if len(list) > 1 {
			slices.SortStableFunc(list, func(a, b *model.Bookmark) int {
				return cmp.Compare(a.Order, b.Order)
			})
		}
		for _, b := range list {
			ids = append(ids, b.ID)
		}
	}
	c.library = lib
	c.revision = lib.Revision
	c.groupID = g.ID
	c.subs = subs
	c.ids = ids
	return subs, ids
}

func (s *mainState) ensureBrowseCache(lib *model.Library, g *model.Group, view string, cols int) (rows []browseRow, subRow, bmRow map[string]int, preserveRow int) {
	c := &s.browseCache
	if c.library == lib && c.revision == lib.Revision && c.groupID == g.ID && c.view == view && c.cols == cols {
		return c.rows, c.subRow, c.bmRow, -1
	}

	var anchorBM, anchorSub string
	if c.library != nil && c.groupID == g.ID && (c.view != view || (view == "grid" && c.cols != cols)) && s.scrollToSub == "" && !s.cursorScrollMoved {
		if first, last := s.browseList.Visible(); first >= 0 && last >= first && first < len(c.rows) {
			r := c.rows[first]
			switch {
			case r.b != nil:
				anchorBM = r.b.ID
			case len(r.items) > 0:
				anchorBM = r.items[0].ID
			default:
				anchorSub = r.sub.ID
			}
		}
	}

	subs, _ := s.ensureGroupCache(lib, g)
	subRow = make(map[string]int, len(g.Subs))
	bmRow = make(map[string]int, len(s.groupCache.ids))
	rows = rows[:0]
	for si, sub := range g.Subs {
		items := subs[si]
		subRow[sub.ID] = len(rows)
		rows = append(rows, browseRow{kind: browseRowHeader, sub: sub, count: len(items)})
		if len(items) == 0 {
			rows = append(rows, browseRow{kind: browseRowEmpty, sub: sub})
			continue
		}
		if view == "grid" {
			for first := 0; first < len(items); first += cols {
				end := min(first+cols, len(items))
				rowIdx := len(rows)
				chunk := items[first:end]
				for _, b := range chunk {
					bmRow[b.ID] = rowIdx
				}
				rows = append(rows, browseRow{
					kind:      browseRowGridRow,
					sub:       sub,
					items:     chunk,
					lastInSub: end == len(items),
				})
			}
			continue
		}
		for _, b := range items {
			bmRow[b.ID] = len(rows)
			rows = append(rows, browseRow{kind: browseRowListItem, sub: sub, b: b})
		}
		rows = append(rows, browseRow{kind: browseRowTail, sub: sub})
	}

	c.library = lib
	c.revision = lib.Revision
	c.groupID = g.ID
	c.view = view
	c.cols = cols
	c.rows = rows
	c.subRow = subRow
	c.bmRow = bmRow
	if s.subHeaderHandles != nil {
		for id := range s.subHeaderHandles {
			if _, ok := subRow[id]; !ok {
				delete(s.subHeaderHandles, id)
			}
		}
	}

	preserveRow = -1
	if anchorBM != "" {
		if idx, ok := bmRow[anchorBM]; ok {
			preserveRow = idx
		}
	} else if anchorSub != "" {
		if idx, ok := subRow[anchorSub]; ok {
			preserveRow = idx
		}
	}
	return rows, subRow, bmRow, preserveRow
}

func (s *mainState) stopBrowseFollow() {
	s.browseFollowActive = false
	s.browseFollowPending = false
}

func (s *mainState) subHeaderHandle(subID string) *ui.Handle {
	if s.subHeaderHandles == nil {
		s.subHeaderHandles = make(map[string]*ui.Handle)
	}
	h := s.subHeaderHandles[subID]
	if h == nil {
		h = &ui.Handle{}
		s.subHeaderHandles[subID] = h
	}
	return h
}

func (s *mainState) syncBrowseSubFromScroll(c *ui.Context, lib *model.Library) {
	a := s.app
	if strings.TrimSpace(s.query) != "" || !hasLiveBookmark(lib) {
		s.stopBrowseFollow()
		return
	}
	g := lib.Group(a.Group)
	if g == nil || len(g.Subs) <= 1 || !hasGroupBookmark(lib, g.ID) {
		s.stopBrowseFollow()
		return
	}

	// Read Visible() on every browse pass so MyGo rebuilds whenever scrolling
	// changes the visible row range.
	first, last := s.browseList.Visible()
	listBounds := s.browseScroll.Bounds(c)

	cache := &s.browseCache
	stable := cache.library == lib &&
		cache.revision == lib.Revision &&
		cache.groupID == g.ID &&
		cache.view == a.View &&
		s.lastBrowseGroup == g.ID &&
		s.scrollToSub == "" &&
		!s.cursorScrollMoved &&
		s.browseScrollInited &&
		listBounds.H > 0 &&
		abs32(listBounds.W-s.lastBrowseListW) < 0.5 &&
		abs32(listBounds.H-s.lastBrowseListH) < 0.5

	scrollY := s.browseScroll.Y
	deltaY := scrollY - s.lastBrowseScrollY
	moved := abs32(deltaY) > 0.5

	s.lastBrowseScrollY = scrollY
	s.lastBrowseListW = listBounds.W
	s.lastBrowseListH = listBounds.H
	if listBounds.H > 0 {
		s.browseScrollInited = true
	}

	if !stable || !s.browseFollowActive || s.browseScroll.MaxY <= 0.5 {
		s.browseFollowPending = false
		return
	}
	if !moved && !s.browseFollowPending {
		return
	}

	if moved {
		// Re-check once after layout commits the new visible range and header bounds.
		s.browseFollowPending = true
		c.AnimationFrame()
	} else {
		s.browseFollowPending = false
	}

	if subID := s.browseSubAtScroll(c, cache.rows, first, last, listBounds, scrollY, deltaY); subID != "" && subID != a.Sub {
		a.Sub = subID
	}
}

func (s *mainState) browseSubAtScroll(c *ui.Context, rows []browseRow, first, last int, listBounds ui.Rect, scrollY, deltaY float32) string {
	if len(rows) == 0 || first < 0 || last < first || first >= len(rows) {
		return ""
	}
	if last >= len(rows) {
		last = len(rows) - 1
	}
	if scrollY <= 0.5 {
		return rows[0].sub.ID
	}

	idx := first
	if rows[idx].kind == browseRowTail && idx+1 < len(rows) {
		idx++
	}
	activeSub := rows[idx].sub.ID

	if listBounds.H <= 0 {
		return activeSub
	}
	readingOffset := min(float32(72), listBounds.H*0.18)
	probeY := listBounds.Y + min(scrollY, readingOffset)

	for i := first; i <= last; i++ {
		if rows[i].kind != browseRowHeader {
			continue
		}
		h := s.subHeaderHandles[rows[i].sub.ID]
		if h == nil {
			continue
		}
		b := h.Bounds(c)
		if b.H <= 0 {
			continue
		}
		headerTop := b.Y - deltaY
		if headerTop <= probeY {
			activeSub = rows[i].sub.ID
		} else {
			if i == idx && idx > 0 {
				activeSub = rows[idx-1].sub.ID
			}
			break
		}
	}
	return activeSub
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func (s *mainState) buildBody(c *ui.Context, p *theme.Palette) {
	body := ui.Column(c).Grow(1).MinHeight(0).TabStop(false).Bind(&s.contentHandle)
	body.Children(func() {
		if strings.TrimSpace(s.query) != "" {
			s.buildSearchBody(c, p)
			return
		}
		s.buildBrowseBody(c, p)
	})
}

func (s *mainState) buildBrowseBody(c *ui.Context, p *theme.Palette) {
	a := s.app
	lib := a.Store.Lib()
	g := lib.Group(a.Group)

	if !hasLiveBookmark(lib) {
		ui.Column(c).Grow(1).Center().Padding(24).Children(func() {
			card := ui.Column(c).MaxWidth(380).Padding(28, 32).Radius(14).
				Background(p.Surface).Border(1, p.Line).Center().Gap(14)
			card.Children(func() {
				ui.Row(c).Size(52, 52).Radius(14).Background(p.AccentBg).Center().Children(func() {
					ui.Icon(c, theme.IconEmptyBookmarks).FontSize(24).TextColor(p.AccentText)
				})
				widget.Title(c, p, "还没有书签", 20)
				ui.Row(c).Wrap().Center().Gap(8).Children(func() {
					bNew := widget.Primary(c, p, "新建书签").Label("新建书签").Tooltip("新建书签")
					if bNew.Clicked() && a.Win.ShowEditor != nil {
						a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
					}
					bImp := widget.Secondary(c, p, "导入书签").Label("导入书签").Tooltip("从浏览器或文件导入书签")
					if bImp.Clicked() && a.Win.ShowImport != nil {
						a.Win.ShowImport("")
					}
					if a.Win.ShowSettings != nil {
						bCloud := widget.Secondary(c, p, "连 iCloud 目录").Label("连 iCloud 目录").Tooltip("前往存储与同步设置")
						if bCloud.Clicked() {
							a.Win.ShowSettings("data")
						}
					}
				})
			})
		})
		return
	}

	if g == nil || len(g.Subs) == 0 || (!hasGroupBookmark(lib, g.ID) && len(g.Subs) <= 1) {
		s.scrollToSub = ""
		s.cursorScrollMoved = false
		title := "还没有分组"
		if g != nil {
			title = "「" + g.Name + "」还没有书签"
		}
		ui.Column(c).Grow(1).Center().Padding(24).Children(func() {
			card := ui.Column(c).MaxWidth(380).Padding(28, 32).Radius(14).
				Background(p.Surface).Border(1, p.Line).Center().Gap(14)
			dragOver := false
			if g != nil {
				_, dragOver = ui.DragOver[dragPayload](card)
				if drop, ok := ui.Drop[dragPayload](card); ok {
					s.moveBookmarksToGroup(drop.IDs, g.ID)
				}
				if dragOver {
					card.Background(softAccentBg(p)).Border(1.5, p.Accent).BorderStyle(ui.BorderDashed)
				}
			}
			card.Children(func() {
				if dragOver && g != nil {
					ui.Text(c, "松手移入「"+g.Name+"」").FontSize(14).FontWeight(600).TextColor(p.AccentText)
					return
				}
				ui.Row(c).Size(52, 52).Radius(14).Background(p.Field).Center().Children(func() {
					ui.Icon(c, theme.IconEmptyFolder).FontSize(24).TextColor(p.Muted)
				})
				widget.Title(c, p, title, 20)
				ui.Row(c).Wrap().Center().Gap(8).Children(func() {
					if g == nil {
						bGrp := widget.Primary(c, p, "新建分组").Label("新建分组").Tooltip("新建分组")
						if bGrp.Clicked() {
							s.startAddGroup(c)
						}
					} else {
						bNew := widget.Primary(c, p, "新建书签").Label("新建书签").Tooltip("新建书签")
						if bNew.Clicked() && a.Win.ShowEditor != nil {
							a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
						}
						if len(g.Subs) == 0 {
							bSub := widget.Secondary(c, p, "新建子分组").Label("新建子分组").Tooltip("新建子分组")
							if bSub.Clicked() {
								s.startAddSub(c)
							}
						}
					}
					bImp := widget.Secondary(c, p, "导入书签").Label("导入书签").Tooltip("导入书签")
					if bImp.Clicked() && a.Win.ShowImport != nil {
						a.Win.ShowImport("")
					}
				})
			})
		})
		return
	}

	const (
		gridGap      = float32(8)
		minCardW     = float32(168)
		cardH        = float32(56)
		gridTileSize = float32(32)
		listTileSize = float32(22)
		minH         = float32(40)
		padY         = float32(6)
	)
	cols := 1
	if a.View == "grid" {
		availW := s.contentHandle.Bounds(c).W - 48
		if availW <= 0 {
			availW = 648
		}
		cols = max(1, int((availW+gridGap)/(minCardW+gridGap)))
	}

	rows, subRow, bmRow, preserveRow := s.ensureBrowseCache(lib, g, a.View, cols)
	if s.lastBrowseGroup == "" && s.scrollToSub == "" {
		s.scrollToSub = a.Sub
	}
	if s.lastBrowseGroup != "" && s.lastBrowseGroup != g.ID && s.scrollToSub == "" && !s.cursorScrollMoved {
		s.browseList.ScrollTo(0, ui.Start)
		s.stopBrowseFollow()
	}
	s.lastBrowseGroup = g.ID

	if preserveRow >= 0 {
		s.browseList.ScrollTo(preserveRow, ui.Start)
		s.stopBrowseFollow()
	}
	if s.scrollToSub != "" {
		if idx, ok := subRow[s.scrollToSub]; ok {
			s.browseList.ScrollTo(idx, ui.Start)
		}
		s.scrollToSub = ""
		s.stopBrowseFollow()
	}
	if s.cursorScrollMoved {
		if idx, ok := bmRow[s.cursorID]; ok {
			s.browseList.ScrollIntoView(idx)
		}
		s.cursorScrollMoved = false
		s.stopBrowseFollow()
	}

	listW := s.browseScroll.Bounds(c).W
	browseListEl := widget.AutoScrollbarState(c, ui.List(c.Key("browse-list"), &s.browseList, len(rows)), &s.browseScroll).Grow(1).MinHeight(0).FillWidth()
	browseListEl.
		HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputScroll && ev.DY != 0 {
				s.browseFollowActive = true
			}
			return false
		}).
		ObserveInput(func(ev ui.InputEvent) {
			switch ev.Kind {
			case ui.InputPointerDown:
				if s.browseScroll.MaxY > 0.5 && listW > 0 && ev.X >= listW-18 {
					s.browseFollowActive = true
				}
			case ui.InputKeyDown:
				switch ev.Key {
				case ui.KeyPageDown, ui.KeyPageUp, ui.KeyHome, ui.KeyEnd, ui.KeySpace:
					s.browseFollowActive = true
				}
			}
		}).
		ItemKey(func(i int) any {
			r := rows[i]
			switch r.kind {
			case browseRowHeader:
				return "hdr-" + r.sub.ID
			case browseRowEmpty:
				return "empty-" + r.sub.ID
			case browseRowTail:
				return "tail-" + r.sub.ID
			case browseRowGridRow:
				return "grid-" + r.sub.ID + "-" + r.items[0].ID
			default:
				return "bm-" + r.b.ID
			}
		}).
		Rows(func(row ui.ListRow) {
			c := row.Context
			r := rows[row.Index]
			extraBottom := float32(0)
			if row.Index == len(rows)-1 {
				extraBottom = 80
			}
			switch r.kind {
			case browseRowHeader:
				padT, padB := float32(14), float32(6)
				if row.Index == 0 {
					padT = 12
				}
				if a.View == "grid" {
					if row.Index > 0 {
						padT = 18
					}
					padB = 10
				}
				ui.Row(c).Bind(s.subHeaderHandle(r.sub.ID)).FillWidth().
					Padding(padT, 24, padB+extraBottom, 24).AlignItems(ui.Center).Gap(8).Children(func() {
					widget.Title(c, p, r.sub.Name, 15).FontWeight(600).TextColor(p.Text).MinWidth(0).Shrink(1)
					ui.Textf(c, "%d", r.count).FontSize(12).TextColor(p.Muted).SingleLine().Shrink(0)
					ui.Box(c).Grow(1).MinWidth(12).Height(1).Background(p.Line)
				})
			case browseRowEmpty:
				ui.Box(c).FillWidth().Padding(0, 24, extraBottom, 24).Children(func() {
					s.buildEmptySubCard(c, p, g, r.sub)
				})
			case browseRowListItem:
				ui.Box(c).FillWidth().Padding(0, 24, 1+extraBottom, 24).Children(func() {
					s.buildBookmarkListRow(c, p, g, r.sub, r.b, listTileSize, minH, padY)
				})
			case browseRowTail:
				ui.Box(c).FillWidth().Padding(0, 24, extraBottom, 24).Children(func() {
					tail := ui.Box(c).Height(10).FillWidth()
					_, tailOver := ui.DragOver[dragPayload](tail)
					if drop, ok := ui.Drop[dragPayload](tail); ok {
						s.dropBookmarksAt(drop, g.ID, r.sub.ID, "")
					}
					if tailOver {
						tail.Children(func() { s.drawDropLine(c, p) })
					}
				})
			case browseRowGridRow:
				bottom := gridGap
				if r.lastInSub {
					bottom = 0
				}
				bottom += extraBottom
				ui.Column(c).FillWidth().Padding(0, 24, bottom, 24).Gap(6).Children(func() {
					ui.Grid(c).Columns(cols).Gap(gridGap).Children(func() {
						for _, b := range r.items {
							s.buildBookmarkGridCard(c, p, g, r.sub, b, cardH, gridTileSize)
						}
					})
					for _, b := range r.items {
						if s.tplBookmarkID == b.ID {
							s.buildTemplatePrompt(c, p, b)
							break
						}
					}
				})
			}
		})
}

func (s *mainState) buildEmptySubCard(c *ui.Context, p *theme.Palette, g *model.Group, sub model.Sub) {
	a := s.app
	empty := ui.Row(c).FillWidth().Padding(14, 14).Radius(10).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(12)
	_, over := ui.DragOver[dragPayload](empty)
	if drop, ok := ui.Drop[dragPayload](empty); ok {
		a.Move(drop.IDs, g.ID, sub.ID)
	}
	if over {
		empty.Background(softAccentBg(p)).Border(1.5, p.Accent).BorderStyle(ui.BorderDashed)
		empty.Children(func() {
			ui.Text(c, "松手移入「"+sub.Name+"」").FontSize(12.5).FontWeight(500).TextColor(p.AccentText)
		})
		return
	}
	empty.Background(p.Surface).Border(1, p.LineSoft)
	empty.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).Size(32, 32).Radius(8).Background(p.Field).Center().Shrink(0).Children(func() {
				ui.Icon(c, theme.IconEmptyFolder).FontSize(15).TextColor(p.Muted)
			})
			ui.Text(c, "「"+sub.Name+"」还没有书签").FontSize(13).TextColor(p.Text2).SingleLine()
		})
		addBtn := ui.ButtonBase(c).Label("新建书签").Tooltip("在「" + sub.Name + "」中新建书签").
			Height(28).PaddingX(11).Radius(7).Gap(5).AlignItems(ui.Center).
			Background(p.Field).TextColor(p.Text).Shrink(0)
		if addBtn.Hovered() {
			addBtn.Background(p.Line)
		}
		addBtn.Children(func() {
			ui.Icon(c, theme.IconPlus).FontSize(13).Shrink(0)
			ui.Text(c, "新建书签").FontSize(12.5).SingleLine()
		})
		if addBtn.Clicked() && a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{GroupID: g.ID, SubID: sub.ID})
		}
	})
}

func rowIconAction(c *ui.Context, p *theme.Palette, icon *ui.SVG, label string, size, iconSize float32, danger bool) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(size, size).Radius(6).Center().TextColor(p.Text2).Shrink(0)
	if b.Hovered() {
		if danger {
			b.Background(p.DangerBg).TextColor(p.Danger)
		} else {
			b.Background(p.SideSel).TextColor(p.Text)
		}
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(iconSize) })
	return b
}

func (s *mainState) buildBookmarkHoverActions(c *ui.Context, p *theme.Palette, b *model.Bookmark, menuOpen *bool, size, iconSize float32) {
	a := s.app
	openBtn := rowIconAction(c, p, theme.IconExternal, "打开", size, iconSize, false)
	if openBtn.Clicked() {
		s.activateBookmark(c, b)
	}
	editBtn := rowIconAction(c, p, theme.IconEdit, "编辑", size, iconSize, false)
	if editBtn.Clicked() && a.Win.ShowEditor != nil {
		a.Win.ShowEditor(app.EditorRequest{ID: b.ID})
	}
	moveBtn := rowIconAction(c, p, theme.IconMove, "移动到…", size, iconSize, false)
	if moveBtn.Clicked() {
		s.openMoveModal([]string{b.ID})
	}
	delBtn := rowIconAction(c, p, theme.IconTrash, "删除", size, iconSize, true)
	if delBtn.Clicked() {
		a.Delete(b.ID)
	}
	moreBtn := rowIconAction(c, p, theme.IconMore, "更多操作", size, iconSize, false)
	moreBtn.PopoverMenuButton(func(m *ui.Menu) {
		s.buildBookmarkMenu(c, m, b)
	}, menuButtonOptions(p, menuOpen, true))
}

func (s *mainState) buildGridHoverActions(c *ui.Context, p *theme.Palette, b *model.Bookmark, menuOpen *bool, size, iconSize float32) {
	a := s.app
	openBtn := rowIconAction(c, p, theme.IconExternal, "打开", size, iconSize, false)
	if openBtn.Clicked() {
		s.activateBookmark(c, b)
	}
	editBtn := rowIconAction(c, p, theme.IconEdit, "编辑", size, iconSize, false)
	if editBtn.Clicked() && a.Win.ShowEditor != nil {
		a.Win.ShowEditor(app.EditorRequest{ID: b.ID})
	}
	moreBtn := rowIconAction(c, p, theme.IconMore, "更多操作", size, iconSize, false)
	moreBtn.PopoverMenuButton(func(m *ui.Menu) {
		s.buildBookmarkMenu(c, m, b)
	}, menuButtonOptions(p, menuOpen, true))
}

func (s *mainState) buildGridHoverToolbar(c *ui.Context, p *theme.Palette, b *model.Bookmark, menuOpen *bool) {
	shadow := ui.RGBA(0, 0, 0, 0.14)
	if p.Dark {
		shadow = ui.RGBA(0, 0, 0, 0.45)
	}
	ui.Row(c).Absolute().Top(13).Right(8).Padding(2).Radius(8).Gap(1).
		Background(p.Surface).Border(1, p.Line).
		Shadow(0, 4, 12, -4, shadow).
		Children(func() {
			s.buildGridHoverActions(c, p, b, menuOpen, 24, 13)
		})
}

func (s *mainState) buildBookmarkListRow(c *ui.Context, p *theme.Palette, g *model.Group, sub model.Sub, b *model.Bookmark, tileSize, minH, padY float32) {
	a := s.app
	keyC := c.Key("bm-" + b.ID)
	selected := s.cursorID == b.ID
	_, polished := a.PolishedAt[b.ID]

	wrap := ui.Column(keyC).FillWidth()
	_, dragOver := ui.DragOver[dragPayload](wrap)
	if drop, ok := ui.Drop[dragPayload](wrap); ok {
		s.dropBookmarksAt(drop, g.ID, sub.ID, b.ID)
	}

	wrap.Children(func() {
		if dragOver {
			s.drawDropLine(c, p)
		}

		row := ui.Row(c).MinHeight(minH).Padding(padY, 8, padY, 10).Radius(8).Gap(10).
			AlignItems(ui.Center).Role(ui.RoleOption).Label(b.Title).AccessSelected(selected)
		menuOpen := ui.Local(row, "rowMenuOpen", func() bool { return false })

		hovered := row.Hovered()
		switch {
		case selected:
			row.Background(p.RowSel)
		case polished:
			row.Background(softAccentBg(p))
		case hovered:
			row.Background(p.Hover)
		}
		if selected && !s.inputFocused(c) {
			row.Border(1.5, p.Accent)
		} else {
			row.Border(1.5, ui.Transparent)
		}

		payload := dragPayload{IDs: []string{b.ID}, SourceGroup: b.GroupID, SourceSub: b.SubID}
		row.Drag(payload)
		if row.Dragging() {
			row.Opacity(0.45)
		}

		var mainArea ui.Element
		row.Children(func() {
			mainArea = ui.Row(c).Grow(1).MinWidth(0).MinHeight(24).Gap(10).AlignItems(ui.Center)
			mainArea.Children(func() {
				s.buildTile(c, b, tileSize)

				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
						ui.Text(c, b.Title).FontSize(13.5).TextColor(p.Text).SingleLine().Shrink(1).MinWidth(0)
						if b.IsTemplate() {
							ui.Icon(c, theme.IconBraces).FontSize(13).TextColor(p.Primary).Shrink(0)
						}
						if polished {
							polishedBadge(c, p)
						}
						if a.Settings.ShowTags && len(b.Tags) > 0 {
							for _, tg := range b.Tags {
								ui.Row(c).Height(18).PaddingX(6).Radius(5).Background(p.Field).AlignItems(ui.Center).Shrink(0).Children(func() {
									ui.Text(c, tg).FontSize(11).TextColor(p.Muted).SingleLine()
								})
							}
						}
					})

					if a.Settings.ShowNote && b.Note != "" {
						ui.Text(c, b.Note).FontSize(12).TextColor(p.Muted).SingleLine()
					}
				})
			})

			if hovered || selected || *menuOpen {
				ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
					if dom := shortDomain(b.URL); dom != "" {
						ui.Text(c, dom).FontSize(12).TextColor(p.Muted).MaxWidth(150).SingleLine().Margin(0, 6, 0, 0)
					}
					s.buildBookmarkHoverActions(c, p, b, menuOpen, 24, 13.5)
				})
			}
		})

		s.bindBookmarkRowEvents(c, p, mainArea, row, b)

		if s.tplBookmarkID == b.ID {
			s.buildTemplatePrompt(c, p, b)
		}
	})
}

func (s *mainState) buildBookmarkGridCard(c *ui.Context, p *theme.Palette, g *model.Group, sub model.Sub, b *model.Bookmark, cardH, tileSize float32) {
	a := s.app
	keyC := c.Key("grid-bm-" + b.ID)
	selected := s.cursorID == b.ID
	_, polished := a.PolishedAt[b.ID]

	card := ui.Row(keyC).MinWidth(0).MinHeight(cardH).Padding(8, 12).Radius(10).
		AlignItems(ui.Center).Role(ui.RoleOption).Label(b.Title).AccessSelected(selected)
	menuOpen := ui.Local(card, "gridMenuOpen", func() bool { return false })

	_, dragOver := ui.DragOver[dragPayload](card)
	if drop, ok := ui.Drop[dragPayload](card); ok {
		s.dropBookmarksAt(drop, g.ID, sub.ID, b.ID)
	}

	hovered := card.Hovered()
	activeHover := hovered || *menuOpen
	switch {
	case dragOver:
		card.Background(softAccentBg(p)).Border(2, p.Accent).BorderStyle(ui.BorderDashed)
	case selected:
		card.Background(softAccentBg(p)).Border(1, p.Accent)
	case polished:
		card.Background(softAccentBg(p)).Border(1, p.Line)
	case activeHover:
		card.Background(p.Hover).Border(1, p.Line.Mix(p.Text, 0.06))
	default:
		card.Background(p.Surface).Border(1, p.Line)
	}

	payload := dragPayload{IDs: []string{b.ID}, SourceGroup: b.GroupID, SourceSub: b.SubID}
	card.Drag(payload)
	if card.Dragging() {
		card.Opacity(0.45)
	}

	subText := shortDomain(b.URL)
	if a.Settings.ShowNote && strings.TrimSpace(b.Note) != "" {
		subText = strings.TrimSpace(b.Note)
	}

	var mainArea ui.Element
	card.Children(func() {
		mainArea = ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Center)
		mainArea.Children(func() {
			s.buildTile(c, b, tileSize)

			textCol := ui.Column(c).Grow(1).MinWidth(0).Gap(2)
			if activeHover {
				textCol.Padding(0, 84, 0, 0)
			}
			textCol.Children(func() {
				ui.Row(c).Gap(5).AlignItems(ui.Center).FillWidth().MinWidth(0).Children(func() {
					ui.Text(c, b.Title).FontSize(13.5).FontWeight(500).TextColor(p.Text).SingleLine().Shrink(1).MinWidth(0)
					if b.IsTemplate() {
						ui.Icon(c, theme.IconBraces).FontSize(12).TextColor(p.Primary).Shrink(0)
					}
					if polished {
						polishedBadge(c, p)
					}
				})

				if subText != "" {
					ui.Text(c, subText).FontSize(12).TextColor(p.Muted).SingleLine()
				}

				if a.Settings.ShowTags && len(b.Tags) > 0 {
					ui.Row(c).Wrap().Gap(4).Margin(2, 0, 0, 0).Children(func() {
						for _, tg := range b.Tags {
							ui.Row(c).Height(18).PaddingX(6).Radius(5).Background(p.Field).AlignItems(ui.Center).Children(func() {
								ui.Text(c, tg).FontSize(11).TextColor(p.Muted).SingleLine()
							})
						}
					})
				}
			})
		})

		if activeHover {
			s.buildGridHoverToolbar(c, p, b, menuOpen)
		}
	})

	s.bindBookmarkRowEvents(c, p, mainArea, card, b)
}

func (s *mainState) bindBookmarkRowEvents(c *ui.Context, p *theme.Palette, clickEl, menuEl ui.Element, b *model.Bookmark) {
	a := s.app
	if clickEl.Clicked() {
		s.cursorID = b.ID
		if b.SubID != "" {
			a.Sub = b.SubID
		}
		s.stopBrowseFollow()
		s.contentHandle.Focus(c)
	}
	if clickEl.DoubleClicked() {
		s.activateBookmark(c, b)
	}

	menuEl.PopoverMenu(func(m *ui.Menu) {
		s.cursorID = b.ID
		s.buildBookmarkMenu(c, m, b)
	}, menuOptions(p))
}

func (s *mainState) buildTile(c *ui.Context, b *model.Bookmark, size float32) {
	a := s.app
	ui.Box(c).Size(size, size).Shrink(0).Children(func() {
		theme.Tile(c, b.Icon, a.Store.IconPath(b.Icon), size)
	})
}

func (s *mainState) drawDropLine(c *ui.Context, p *theme.Palette) {
	ui.Row(c).FillWidth().PaddingX(8).MarginY(1).Children(func() {
		ui.Box(c).Grow(1).Height(2).Radius(1).Background(p.Accent).Children(func() {
			ui.Box(c).Absolute().Left(-4).Top(-3).Size(8, 8).Radius(4).Background(p.Accent)
		})
	})
}

func (s *mainState) buildTemplatePrompt(c *ui.Context, p *theme.Palette, b *model.Bookmark) {
	a := s.app
	box := ui.Column(c.Key("tpl-"+b.ID)).Padding(10).Margin(2, 0, 4, 0).Radius(9).
		Background(p.RowSel).Gap(8)
	revealed := ui.Local(box, "revealed", func() bool { return false })
	box.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(0, 0, 0, 34).Children(func() {
			ui.Text(c, "query").FontSize(12).TextColor(p.Muted).Shrink(0)
			in := ui.TextInputBase(c, &s.tplQuery).Bind(&s.tplInputHandle).
				Placeholder("输入参数，回车打开").Label("输入参数").Grow(1).MinWidth(0).
				Height(30).PaddingX(9).Radius(7).Background(p.Surface).Border(1.5, p.Accent).TextColor(p.Text)
			ui.Text(c, "↵ 打开").FontSize(12).TextColor(p.Muted).Shrink(0)
			if !*revealed {
				in.ScrollIntoView()
				*revealed = true
			}
			if in.Submitted() && a.Open(b, s.tplQuery) {
				s.tplBookmarkID = ""
				s.tplQuery = ""
				s.contentHandle.Focus(c)
			}
			if in.Shortcut(0, ui.KeyEscape) {
				s.tplBookmarkID = ""
				s.tplQuery = ""
				s.contentHandle.Focus(c)
			}
		})
		// Live preview of URL with {query} substituted.
		ui.Row(c).Padding(0, 0, 0, 34).Children(func() {
			preview := strings.TrimPrefix(strings.TrimPrefix(b.URL, "https://"), "http://")
			before, after, found := strings.Cut(preview, "{query}")
			if !found {
				ui.Text(c, preview).FontSize(12).TextColor(p.Muted).SingleLine()
				return
			}
			q := s.tplQuery
			if q == "" {
				q = "{query}"
			}
			ui.RichText(c,
				ui.Span{Text: before, Color: p.Muted, Size: 12},
				ui.Span{Text: q, Color: p.AccentText, Size: 12, Weight: 500},
				ui.Span{Text: after, Color: p.Muted, Size: 12},
			).SingleLine()
		})
	})
}

func (s *mainState) searchHits() []app.Hit {
	lib := s.app.Store.Lib()
	query := strings.TrimSpace(s.query)
	cache := &s.searchCache
	if cache.library != lib || cache.revision != lib.Revision || cache.query != query || cache.scope != s.searchScope {
		cache.library = lib
		cache.revision = lib.Revision
		cache.query = query
		cache.scope = s.searchScope
		cache.hits = s.app.Search.Search(lib, query, s.searchScope)
	}
	return cache.hits
}

func (s *mainState) buildSearchBody(c *ui.Context, p *theme.Palette) {
	a := s.app
	lib := a.Store.Lib()
	q := strings.TrimSpace(s.query)
	isURL := looksLikeURL(q)
	hits := s.searchHits()

	total := len(hits)
	if isURL {
		total++
	}
	if total == 0 {
		s.searchIdx = 0
	} else {
		s.searchIdx = max(0, min(s.searchIdx, total-1))
	}

	// Scope chips bar.
	widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("search-scopes"))).ScrollbarInsets(0, 24, 0, 24).FillWidth().Height(39).Shrink(0).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Padding(10, 24, 4, 24).Children(func() {
			s.buildScopeChip(c, p, "", "全部")
			for _, g := range lib.Groups {
				s.buildScopeChip(c.Key("scope-"+g.ID), p, g.ID, g.Name)
			}
		})
	})

	if !isURL && len(hits) == 0 {
		s.searchList.ScrollTo(0, ui.Start)
		s.buildEmptySearch(c, p, q)
		return
	}

	cols, gap := 1, float32(2)
	if a.View == "grid" {
		gap = 8
		availW := s.contentHandle.Bounds(c).W - 48
		if availW <= 0 {
			availW = 648
		}
		cols = max(1, int((availW+gap)/(168+gap)))
	}
	offset := 0
	if isURL {
		offset = 1
	}
	if s.searchScrollMoved {
		row := 0
		if s.searchIdx >= offset {
			row = 1 + (s.searchIdx-offset)/cols
		}
		s.searchList.ScrollIntoView(row)
		s.searchScrollMoved = false
	}
	n := 1 + (len(hits)+cols-1)/cols
	widget.AutoScrollbar(c, ui.List(c.Key("search-results"), &s.searchList, n)).Grow(1).MinHeight(0).FillWidth().
		ItemKey(func(i int) any {
			if i == 0 {
				return "heading"
			}
			return "hit-" + hits[(i-1)*cols].B.ID
		}).
		Rows(func(row ui.ListRow) {
			c := row.Context
			if row.Index == 0 {
				bottom := float32(6)
				if n == 1 {
					bottom = 48
				}
				ui.Column(c).FillWidth().Padding(8, 24, bottom, 24).Children(func() {
					if isURL {
						s.buildPasteURLCard(c, p, lib, q, s.searchIdx == 0)
					} else {
						ui.Textf(c, "%d 个结果", len(hits)).FontSize(12).TextColor(p.Muted).Padding(2, 10, 4, 10)
					}
				})
				return
			}

			first := (row.Index - 1) * cols
			items := hits[first:min(first+cols, len(hits))]
			bottom := gap
			if row.Index == n-1 {
				bottom = 48
			}
			ui.Column(c).FillWidth().Padding(0, 24, bottom, 24).Gap(6).Children(func() {
				if a.View == "grid" {
					ui.Grid(c).Columns(cols).Gap(gap).Children(func() {
						for i, hit := range items {
							s.buildSearchCard(c, p, hit, first+i+offset)
						}
					})
				} else {
					s.buildSearchRow(c, p, items[0], first+offset)
				}
				for _, hit := range items {
					if s.tplBookmarkID == hit.B.ID {
						s.buildTemplatePrompt(c, p, hit.B)
						break
					}
				}
			})
		})
}

func (s *mainState) buildEmptySearch(c *ui.Context, p *theme.Palette, q string) {
	a := s.app
	widget.AutoScrollbar(c, ui.Scroll(c)).Grow(1).MinHeight(0).Padding(8, 24, 48, 24).Children(func() {
		ui.Column(c).Margin(64, 0, 0, 0).FillWidth().Center().Children(func() {
			card := ui.Column(c).MaxWidth(380).Padding(28, 32).Radius(14).
				Background(p.Surface).Border(1, p.Line).Center().Gap(14)
			card.Children(func() {
				ui.Row(c).Size(52, 52).Radius(14).Background(p.Field).Center().Children(func() {
					ui.Icon(c, theme.IconEmptySearch).FontSize(24).TextColor(p.Muted)
				})
				widget.Title(c, p, "还没有匹配的书签", 19)
				ui.Row(c).Wrap().Center().Gap(8).Children(func() {
					if s.searchScope != "" {
						allBtn := widget.Primary(c, p, "在全部分组中搜索").Label("在全部分组中搜索").Tooltip("切换到全部分组继续搜索")
						if allBtn.Clicked() {
							s.searchScope = ""
							s.searchIdx = 0
						}
						clr := widget.Secondary(c, p, "清空搜索").Label("清空搜索").Tooltip("清空搜索关键词")
						if clr.Clicked() {
							s.clearSearch()
						}
					} else {
						clr := widget.Primary(c, p, "清空搜索").Label("清空搜索").Tooltip("清空搜索关键词")
						if clr.Clicked() {
							s.clearSearch()
						}
					}
					if a.Win.ShowEditor != nil {
						newBtn := widget.Secondary(c, p, "新建书签").Label("新建书签").Tooltip("新建书签")
						if newBtn.Clicked() {
							a.Win.ShowEditor(app.EditorRequest{Title: q, GroupID: a.Group, SubID: a.Sub})
						}
					}
				})
			})
		})
	})
}

func (s *mainState) buildScopeChip(c *ui.Context, p *theme.Palette, groupID, name string) {
	on := s.searchScope == groupID
	b := ui.ButtonBase(c).Label(name).Tooltip(name).Height(25).MaxWidth(168).Shrink(0).
		PaddingX(11).Radius(999).AlignItems(ui.Center)
	switch {
	case on:
		b.Background(p.AccentBg).TextColor(p.AccentText)
	case b.Hovered():
		b.Background(p.RowSel).TextColor(p.Text)
	default:
		b.Background(p.Field).TextColor(p.Text2)
	}
	b.Children(func() {
		t := ui.Text(c, name).FontSize(12).SingleLine()
		if on {
			t.FontWeight(500)
		}
	})
	if b.Clicked() {
		s.searchScope = groupID
		s.searchIdx = 0
		s.searchList.ScrollTo(0, ui.Start)
	}
}

func (s *mainState) buildPasteURLCard(c *ui.Context, p *theme.Palette, lib *model.Library, raw string, highlighted bool) {
	a := s.app
	fullURL := ensureURLScheme(raw)
	existing := lib.FindURL(fullURL)

	card := ui.Row(c).Padding(11, 14).Margin(0, 0, 6, 0).Radius(10).Gap(12).AlignItems(ui.Center).
		Background(softAccentBg(p)).Cursor(ui.CursorPointer)
	if highlighted {
		card.Border(1.5, p.Accent)
	} else {
		card.Border(1, p.Accent.Alpha(0.45))
	}

	title := "保存这个网址"
	subtitle := raw
	if existing != nil {
		title = "编辑已保存的书签「" + existing.Title + "」"
		subtitle = raw + " · 已存在于「" + a.PathOf(existing) + "」"
	}

	card.Children(func() {
		ui.Row(c).Size(30, 30).Radius(8).Background(p.Primary).Center().Shrink(0).Children(func() {
			ui.Icon(c, theme.IconPlus).FontSize(15).TextColor(p.OnPrimary)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(600).TextColor(p.Text).SingleLine()
			ui.Text(c, subtitle).FontSize(12).TextColor(p.Muted).SingleLine()
		})
		ui.Row(c).Padding(2, 6).Radius(6).Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
			ui.Text(c, "↵").FontSize(12).TextColor(p.Muted).SingleLine()
		})
	})

	if card.Clicked() && a.Win.ShowEditor != nil {
		if existing != nil {
			a.Win.ShowEditor(app.EditorRequest{ID: existing.ID})
		} else {
			a.Win.ShowEditor(app.EditorRequest{URL: fullURL, GroupID: a.Group, SubID: a.Sub})
		}
	}
}

func (s *mainState) buildSearchRow(c *ui.Context, p *theme.Palette, hit app.Hit, idx int) {
	a := s.app
	b := hit.B
	on := idx == s.searchIdx
	keyC := c.Key("sr-" + b.ID)
	row := ui.Row(keyC).MinHeight(36).Padding(6, 10, 6, 12).Radius(9).Gap(11).AlignItems(ui.Center).
		Role(ui.RoleOption).Label(b.Title)
	menuOpen := ui.Local(row, "srMenuOpen", func() bool { return false })
	hovered := row.Hovered()
	if on {
		row.Background(p.RowSel)
	} else if hovered {
		row.Background(p.Hover)
	}

	path := hit.Path
	if path == "" {
		path = a.PathOf(b)
	}

	var mainArea ui.Element
	row.Children(func() {
		mainArea = ui.Row(c).Grow(1).MinWidth(0).Gap(11).AlignItems(ui.Center)
		mainArea.Children(func() {
			s.buildTile(c, b, 26)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Row(c).Gap(5).AlignItems(ui.Center).MinWidth(0).Children(func() {
					ui.RichText(c, highlightedTitleSpans(b.Title, hit.Ranges, p.AccentBg, p.AccentText)...).
						FontSize(13.5).FontWeight(500).TextColor(p.Text).SingleLine().Shrink(1).MinWidth(0)
					if b.IsTemplate() {
						ui.Icon(c, theme.IconBraces).FontSize(12).TextColor(p.Primary).Shrink(0)
					}
				})
				ui.Text(c, shortDomain(b.URL)).FontSize(12).TextColor(p.Muted).SingleLine()
			})
			ui.Text(c, path).FontSize(12).TextColor(p.Muted).MaxWidth(160).SingleLine().Shrink(0).Tooltip(path)
		})

		if hovered || *menuOpen {
			ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
				s.buildBookmarkHoverActions(c, p, b, menuOpen, 26, 13.5)
			})
		} else if on {
			ui.Row(c).Padding(2, 6).Radius(6).Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
				ui.Text(c, "↵").FontSize(11.5).TextColor(p.Muted).SingleLine()
			})
		}
	})

	if mainArea.Clicked() {
		s.searchIdx = idx
	}
	if mainArea.DoubleClicked() {
		s.activateBookmark(c, b)
	}
	row.PopoverMenu(func(m *ui.Menu) {
		s.buildBookmarkMenu(c, m, b)
	}, menuOptions(p))
}

func (s *mainState) buildSearchCard(c *ui.Context, p *theme.Palette, hit app.Hit, idx int) {
	a := s.app
	b := hit.B
	on := idx == s.searchIdx
	card := ui.Row(c.Key("sgrid-"+b.ID)).MinWidth(0).MinHeight(56).Padding(8, 12).Radius(10).
		AlignItems(ui.Center).Role(ui.RoleOption).Label(b.Title)
	menuOpen := ui.Local(card, "sgMenuOpen", func() bool { return false })
	hovered := card.Hovered()
	activeHover := hovered || *menuOpen
	switch {
	case on:
		card.Background(softAccentBg(p)).Border(1, p.Accent)
	case activeHover:
		card.Background(p.Hover).Border(1, p.Line.Mix(p.Text, 0.06))
	default:
		card.Background(p.Surface).Border(1, p.Line)
	}

	path := hit.Path
	if path == "" {
		path = a.PathOf(b)
	}

	var mainArea ui.Element
	card.Children(func() {
		mainArea = ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Center)
		mainArea.Children(func() {
			s.buildTile(c, b, 32)
			textCol := ui.Column(c).Grow(1).MinWidth(0).Gap(2)
			if activeHover {
				textCol.Padding(0, 84, 0, 0)
			}
			textCol.Children(func() {
				ui.Row(c).Gap(5).AlignItems(ui.Center).FillWidth().MinWidth(0).Children(func() {
					ui.RichText(c, highlightedTitleSpans(b.Title, hit.Ranges, p.AccentBg, p.AccentText)...).
						FontSize(13.5).FontWeight(500).TextColor(p.Text).SingleLine().Shrink(1).MinWidth(0)
					if b.IsTemplate() {
						ui.Icon(c, theme.IconBraces).FontSize(12).TextColor(p.Primary).Shrink(0)
					}
				})
				ui.Text(c, path).FontSize(12).TextColor(p.Muted).SingleLine()
			})
		})

		if activeHover {
			s.buildGridHoverToolbar(c, p, b, menuOpen)
		}
	})

	if mainArea.Clicked() {
		s.searchIdx = idx
	}
	if mainArea.DoubleClicked() {
		s.activateBookmark(c, b)
	}
	card.PopoverMenu(func(m *ui.Menu) {
		s.buildBookmarkMenu(c, m, b)
	}, menuOptions(p))
}

func polishedBadge(c *ui.Context, p *theme.Palette) {
	ui.Row(c).Height(18).PaddingX(6).Radius(5).Background(p.Surface).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, "已润色").FontSize(11).TextColor(p.AccentText).SingleLine()
	})
}

func highlightedTitleSpans(title string, ranges [][2]int, hiBg, hiFg ui.Color) []ui.Span {
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
