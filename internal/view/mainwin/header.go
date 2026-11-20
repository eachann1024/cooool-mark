package mainwin

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

func (s *mainState) buildHeader(c *ui.Context, p *theme.Palette) {
	a := s.app
	lib := a.Store.Lib()
	bar := c.TitleBar()
	h := widget.BarH(bar)
	padR := max(float32(12), bar.Right+8)

	header := ui.Row(c).Height(h).Shrink(0).AlignItems(ui.Center).Gap(8).
		Padding(0, padR, 0, 16).DragWindow()
	header.Children(func() {
		// Group pills zone: scrollable tabs + pinned "+" button.
		ui.Row(c.Key("main-tabs-zone")).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(4).Children(func() {
			pills := widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("main-tabs"))).
				ScrollbarInsets(0, 4, 0, 4).Grow(1).MinWidth(0).Height(32).Padding(2, 6, 4, 0)
			pills.Children(func() {
				row := ui.Row(c.Key("main-tabs-row")).AlignItems(ui.Center).Gap(4)
				row.Children(func() {
					searching := strings.TrimSpace(s.query) != ""
					for i, g := range lib.Groups {
						s.buildGroupPill(c, p, lib, i, g, !searching && g.ID == a.Group)
					}
				})
			})
			if s.addingGroup {
				s.buildNewGroupInput(c, p)
			} else {
				addG := ui.ButtonBase(c.Key("add-grp-btn")).Label("新建分组").Tooltip("新建分组").
					Size(26, 26).Radius(999).Center().TextColor(p.Muted).Shrink(0)
				if addG.Hovered() {
					addG.Background(p.RowSel).TextColor(p.Text)
				}
				addG.Children(func() { ui.Icon(c, theme.IconPlus).FontSize(14) })
				if addG.Clicked() {
					s.startAddGroup(c)
				}
			}
		})

		ui.Box(c).Size(1, 16).Background(p.Line).Shrink(0)

		// Search field.
		s.buildSearchField(c, p)

		// Action buttons.
		newBtn := widget.IconButton(c, p, theme.IconPlus, "新建书签").Shrink(0)
		if newBtn.Clicked() && a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
		}

		isGrid := a.View == "grid"
		viewIcon, viewLabel := theme.IconGrid, "切换到网格"
		if isGrid {
			viewIcon, viewLabel = theme.IconList, "切换到列表"
		}
		viewBtn := widget.IconButton(c, p, viewIcon, viewLabel).Shrink(0)
		if viewBtn.Clicked() {
			s.toggleView()
		}

		moreBtn := widget.IconButton(c, p, theme.IconMore, "菜单").Shrink(0)
		moreBtn.PopoverMenuButton(func(m *ui.Menu) {
			if m.Item("新建书签").Shortcut(ui.Cmd, ui.KeyN).Chosen() {
				if a.Win.ShowEditor != nil {
					a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
				}
			}
			m.Separator()
			if m.Item("导入书签…").Chosen() {
				if a.Win.ShowImport != nil {
					a.Win.ShowImport("")
				}
			}
			m.Separator()
			m.Submenu("外观", func(sm *ui.Menu) {
				modes := [][2]string{{"auto", "自动"}, {"light", "浅色"}, {"dark", "深色"}}
				for _, mode := range modes {
					cur := a.Settings.Appearance == mode[0] || (mode[0] == "auto" && a.Settings.Appearance == "")
					if sm.Item(mode[1]).Radio(cur).Chosen() {
						a.Settings.Appearance = mode[0]
						a.SaveSettings()
					}
				}
			})
			if m.Item("设置…").Shortcut(ui.Cmd, ui.KeyComma).Chosen() {
				if a.Win.ShowSettings != nil {
					a.Win.ShowSettings("")
				}
			}
		}, menuButtonOptions(p, nil, true))
	})
}

func (s *mainState) buildGroupPill(c *ui.Context, p *theme.Palette, lib *model.Library, idx int, g model.Group, selected bool) {
	a := s.app
	keyC := c.Key("grp-" + g.ID)

	if s.renamingGroupID == g.ID {
		box := ui.Row(keyC).Height(26).Width(92).PaddingX(10).Radius(999).AlignItems(ui.Center).
			Background(p.Surface).Border(1.5, p.Accent).Shrink(0)
		box.Children(func() {
			in := ui.TextInputBase(c, &s.renameDraft).Bind(&s.renameHandle).
				Placeholder("分组名称").Label("重命名分组").Grow(1).TextColor(p.Text)
			if in.Focused() {
				s.renameFocused = true
			}
			switch {
			case in.Submitted():
				if name := strings.TrimSpace(s.renameDraft); name != "" {
					a.RenameGroup(g.ID, name)
				}
				s.renamingGroupID = ""
				s.renameFocused = false
			case in.Shortcut(0, ui.KeyEscape):
				s.renamingGroupID = ""
				s.renameFocused = false
			case s.renameFocused && !in.Focused():
				if name := strings.TrimSpace(s.renameDraft); name != "" {
					a.RenameGroup(g.ID, name)
				}
				s.renamingGroupID = ""
				s.renameFocused = false
			}
		})
		return
	}

	pill := widget.Pill(keyC, p, g.Name, selected).Shrink(0)
	widget.RevealSelectedTab(pill, selected, idx)
	if _, over := ui.DragOver[dragPayload](pill); over {
		pill.Background(softAccentBg(p)).TextColor(p.AccentText).Border(1, p.Accent).BorderStyle(ui.BorderDashed)
	}
	if drop, ok := ui.Drop[dragPayload](pill); ok {
		s.moveBookmarksToGroup(drop.IDs, g.ID)
	}
	if pill.Clicked() {
		s.clearSearch()
		s.cursorID = ""
		s.browseList.ScrollTo(0, ui.Start)
		s.stopBrowseFollow()
		a.Go(g.ID, "")
	}

	pill.PopoverMenu(func(m *ui.Menu) {
		if m.Item("重命名").Chosen() {
			s.startRenameGroup(c, g)
		}
		if m.Item("新建子分组").Chosen() {
			a.Go(g.ID, "")
			s.startAddSub(c)
		}
		m.Separator()
		if m.Item("左移").Disabled(idx == 0).Chosen() {
			reorderGroupByDelta(a, lib, idx, -1)
		}
		if m.Item("右移").Disabled(idx >= len(lib.Groups)-1).Chosen() {
			reorderGroupByDelta(a, lib, idx, 1)
		}
		m.Separator()
		if m.Item("删除分组").Danger().Chosen() {
			a.DeleteGroup(g.ID)
		}
	}, menuOptions(p))
}

func (s *mainState) buildNewGroupInput(c *ui.Context, p *theme.Palette) {
	a := s.app
	box := ui.Row(c.Key("new-grp-input")).Height(26).Width(92).PaddingX(10).Radius(999).AlignItems(ui.Center).
		Background(p.Surface).Border(1.5, p.Accent).Shrink(0)
	box.Children(func() {
		in := ui.TextInputBase(c, &s.newGroupDraft).Bind(&s.newGroupHandle).
			Placeholder("分组名称").Grow(1).TextColor(p.Text)
		if in.Focused() {
			s.newGroupFocused = true
		}
		switch {
		case in.Submitted():
			if name := strings.TrimSpace(s.newGroupDraft); name != "" {
				a.AddGroup(name)
			}
			s.addingGroup = false
			s.newGroupFocused = false
		case in.Shortcut(0, ui.KeyEscape):
			s.addingGroup = false
			s.newGroupFocused = false
		case s.newGroupFocused && !in.Focused():
			if name := strings.TrimSpace(s.newGroupDraft); name != "" {
				a.AddGroup(name)
			}
			s.addingGroup = false
			s.newGroupFocused = false
		}
	})
}

func (s *mainState) buildSearchField(c *ui.Context, p *theme.Palette) {
	focused := s.searchHandle.Focused(c)
	hasQ := s.query != ""

	box := ui.Row(c).Height(28).MinWidth(110).Width(188).Shrink(1).
		Padding(0, 6, 0, 10).Radius(7).Gap(7).AlignItems(ui.Center)
	if focused || hasQ {
		box.Background(p.Surface).Border(1.5, p.Accent)
	} else {
		box.Background(p.Field).Border(1.5, ui.Transparent)
	}

	var searchIn ui.Element
	box.Children(func() {
		ui.Icon(c, theme.IconSearch).FontSize(14).TextColor(p.Muted).Shrink(0)
		searchIn = ui.TextInputBase(c, &s.query).Bind(&s.searchHandle).
			Placeholder("搜索书签").Label("搜索书签").Grow(1).MinWidth(0).TextColor(p.Text)
		if hasQ {
			clr := ui.ButtonBase(c).Label("清空搜索").Tooltip("清空搜索").Height(20).PaddingX(7).Radius(5).
				AlignItems(ui.Center).Background(p.Field).TextColor(p.Muted).Shrink(0)
			if clr.Hovered() {
				clr.Background(p.RowSel).TextColor(p.Text)
			}
			clr.Children(func() { ui.Text(c, "Esc").FontSize(11.5).SingleLine() })
			if clr.Clicked() {
				s.clearSearch()
				s.searchHandle.Focus(c)
			}
		}
	})

	if searchIn.Changed() {
		s.tplBookmarkID = ""
		if strings.TrimSpace(s.query) == "" {
			s.searchScope = ""
		}
		s.searchIdx = 0
		s.searchList.ScrollTo(0, ui.Start)
	}
	if searchIn.Shortcut(0, ui.KeyEscape) {
		if s.query != "" {
			s.clearSearch()
		} else {
			s.contentHandle.Focus(c)
		}
	}

	if strings.TrimSpace(s.query) != "" {
		hits := s.searchHits()
		isURL := looksLikeURL(strings.TrimSpace(s.query))
		total := len(hits)
		if isURL {
			total++
		}
		if total > 0 {
			s.searchIdx = min(s.searchIdx, total-1)
		}
		if searchIn.Shortcut(0, ui.KeyDown) && total > 0 {
			s.searchIdx = min(s.searchIdx+1, total-1)
			s.searchScrollMoved = true
		}
		if searchIn.Shortcut(0, ui.KeyUp) && total > 0 {
			s.searchIdx = max(s.searchIdx-1, 0)
			s.searchScrollMoved = true
		}
		if searchIn.Submitted() && total > 0 {
			s.activateSearchIndex(c, hits, isURL, s.searchIdx)
		}
	}
}

func (s *mainState) startRenameGroup(c *ui.Context, g model.Group) {
	s.renamingSubID = ""
	s.renamingGroupID = g.ID
	s.renameDraft = g.Name
	s.renameFocused = false
	s.renameHandle.SelectAll(c)
}

func (s *mainState) startAddGroup(c *ui.Context) {
	s.addingSub = false
	s.addingGroup = true
	s.newGroupDraft = ""
	s.newGroupFocused = false
	s.newGroupHandle.Focus(c)
}

func reorderGroupByDelta(a *app.App, lib *model.Library, idx, delta int) {
	next := idx + delta
	if next < 0 || next >= len(lib.Groups) {
		return
	}
	ids := make([]string, len(lib.Groups))
	for i, g := range lib.Groups {
		ids[i] = g.ID
	}
	ids[idx], ids[next] = ids[next], ids[idx]
	a.ReorderGroups(ids)
}

func (s *mainState) moveBookmarksToGroup(ids []string, groupID string) {
	a := s.app
	lib := a.Store.Lib()
	g := lib.Group(groupID)
	if g == nil {
		return
	}
	subID := ""
	if len(g.Subs) > 0 {
		subID = g.Subs[0].ID
	} else {
		subID = a.AddSub(groupID, "常用")
	}
	a.Move(ids, groupID, subID)
}
