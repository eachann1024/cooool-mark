package mainwin

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

func (s *mainState) buildSidebar(c *ui.Context, p *theme.Palette) {
	a := s.app
	lib := a.Store.Lib()

	bar := c.TitleBar()
	topH := widget.BarH(bar)

	aside := ui.Column(c).Width(184).Shrink(0).FillHeight().
		Background(p.Sidebar).BorderWidth(0, 1, 0, 0).BorderColor(p.Line)
	aside.Children(func() {
		// Top bar: macOS traffic lights live here; Windows/Linux show title.
		top := ui.Row(c).Height(topH).Shrink(0).AlignItems(ui.Center).Padding(0, 12, 0, 16).DragWindow()
		top.Children(func() {
			if runtime.GOOS != "darwin" || bar.Left == 0 {
				ui.Text(c, locales.AppName()).Font(theme.TitleFont(c)).FontSize(14).FontWeight(600).TextColor(p.Text2).SingleLine()
			}
		})

		// Primary group list.
		nav := widget.AutoScrollbar(c, ui.Scroll(c.Key("sidebar-groups"))).Grow(1).MinHeight(0).Padding(6, 10)
		nav.Children(func() {
			if len(lib.Groups) == 0 && !s.addingGroup {
				s.buildSidebarEmpty(c, p, locales.MainNoGroups(), locales.ActionNewGroup(), func() { s.startAddGroup(c) })
				return
			}
			col := ui.Column(c.Key("sidebar-groups-col")).Gap(2)
			lastSelected := ui.Local(col, "selected-group", func() string { return "" })
			revealGroup := ""
			if strings.TrimSpace(s.query) == "" && *lastSelected != a.Group {
				revealGroup = a.Group
				*lastSelected = a.Group
			}
			counts := countBookmarksByGroup(lib)
			ids := make([]string, len(lib.Groups))
			index := make(map[string]int, len(lib.Groups))
			for i, g := range lib.Groups {
				ids[i], index[g.ID] = g.ID, i
			}
			col.Children(func() {
				for _, id := range s.groupSort.Order(ids) {
					i := index[id]
					g := lib.Groups[i]
					s.buildSidebarGroupItem(c, p, lib, i, g, counts[g.ID], revealGroup == g.ID)
				}
				if s.addingGroup {
					s.buildNewGroupInput(c, p)
				}
			})
			if order, ok := s.groupSort.Dropped(); ok {
				a.ReorderGroups(order)
			}
		})

		// Bottom bar: new group button (only when the sidebar isn't showing its empty state).
		if len(lib.Groups) > 0 || s.addingGroup {
			ui.Row(c).Shrink(0).Padding(8, 10, 12, 10).Children(func() {
				btn := ui.ButtonBase(c).Label(locales.ActionNewGroup()).Tooltip(locales.ActionNewGroup()).
					Height(28).FillWidth().PaddingX(10).Radius(7).Gap(6).AlignItems(ui.Center).
					TextColor(p.Muted)
				if btn.Hovered() {
					btn.Background(p.Hover.Mix(p.SideSel, 0.7)).TextColor(p.Text)
				}
				btn.Children(func() {
					ui.Icon(c, theme.IconPlus).FontSize(14).Shrink(0)
					ui.Text(c, locales.ActionNewGroup()).FontSize(12.5).SingleLine()
				})
				if btn.Clicked() {
					s.startAddGroup(c)
				}
			})
		}
	})
}

func countBookmarksByGroup(lib *model.Library) map[string]int {
	counts := make(map[string]int, len(lib.Groups))
	for i := range lib.Bookmarks {
		b := &lib.Bookmarks[i]
		if !b.InTrash() {
			counts[b.GroupID]++
		}
	}
	return counts
}

func (s *mainState) buildSidebarEmpty(c *ui.Context, p *theme.Palette, title, btnLabel string, onClick func()) {
	ui.Column(c).FillWidth().Padding(28, 8, 12, 8).Center().Gap(10).Children(func() {
		ui.Row(c).Size(34, 34).Radius(9).Background(p.SideSel.Mix(p.Sidebar, 0.4)).Center().Children(func() {
			ui.Icon(c, theme.IconEmptyFolder).FontSize(16).TextColor(p.Muted)
		})
		ui.Text(c, title).FontSize(12.5).TextColor(p.Muted).SingleLine().Selectable()
		btn := ui.ButtonBase(c).Label(btnLabel).Tooltip(btnLabel).
			Height(28).PaddingX(12).Radius(7).Gap(5).AlignItems(ui.Center).
			Background(p.SideSel).TextColor(p.Text)
		if btn.Hovered() {
			btn.Background(p.Line)
		}
		btn.Children(func() {
			ui.Icon(c, theme.IconPlus).FontSize(13).Shrink(0)
			ui.Text(c, btnLabel).FontSize(12.5).FontWeight(500).SingleLine()
		})
		if btn.Clicked() && onClick != nil {
			onClick()
		}
	})
}

func (s *mainState) buildSidebarGroupItem(c *ui.Context, p *theme.Palette, lib *model.Library, idx int, g model.Group, count int, reveal bool) {
	a := s.app
	keyC := c.Key("grp-" + g.ID)

	if s.renamingGroupID == g.ID {
		box := ui.Row(keyC).Height(30).PaddingX(8).Radius(7).AlignItems(ui.Center).
			Background(p.Surface).Border(1.5, p.Accent)
		box.Children(func() {
			in := ui.TextInputBase(c, &s.renameDraft).Bind(&s.renameHandle).
				Placeholder(locales.MainGroupNamePlaceholder()).Label(locales.MainRenameGroupLabel()).Grow(1).TextColor(p.Text)
			handleInlineEditInput(in, &s.renameFocused, func() {
				if name := strings.TrimSpace(s.renameDraft); name != "" {
					a.RenameGroup(g.ID, name)
				}
				s.renamingGroupID = ""
			}, func() {
				s.renamingGroupID = ""
			})
		})
		return
	}

	selected := g.ID == a.Group && strings.TrimSpace(s.query) == ""
	row := ui.Row(keyC).Height(30).Padding(0, 6, 0, 12).Radius(7).
		AlignItems(ui.Center).Gap(4).Cursor(ui.CursorPointer)
	if reveal {
		row.ScrollIntoView()
	}
	menuOpen := ui.Local(row, "grpMenuOpen", func() bool { return false })

	drag, dragOver := ui.DragOver[dragPayload](row)
	if drop, ok := ui.Drop[dragPayload](row); ok {
		s.moveBookmarksToGroup(drop.IDs, g.ID)
	}

	hovered := row.Hovered()
	switch {
	case dragOver:
		row.Background(softAccentBg(p)).Border(2, p.Accent)
	case selected:
		row.Background(p.SideSel)
	case hovered:
		row.Background(p.Hover.Mix(p.SideSel, 0.7))
	}

	buildGroupMenu := func(m *ui.Menu) {
		if m.Item(locales.ActionRename()).Icon(theme.MenuIcon(theme.IconEdit)).Chosen() {
			s.startRenameGroup(c, g)
		}
		if m.Item(locales.ActionNewSub()).Icon(theme.MenuIcon(theme.IconFolderPlus)).Chosen() {
			a.Go(g.ID, "")
			s.startAddSub(c)
		}
		m.Separator()
		if m.Item(locales.ActionMoveUp()).Icon(theme.MenuIcon(theme.IconChevronUp)).Disabled(idx == 0).Chosen() {
			reorderGroupByDelta(a, lib, idx, -1)
		}
		if m.Item(locales.ActionMoveDown()).Icon(theme.MenuIcon(theme.IconChevronDown)).Disabled(idx >= len(lib.Groups)-1).Chosen() {
			reorderGroupByDelta(a, lib, idx, 1)
		}
		m.Separator()
		if m.Item(locales.ActionDeleteGroup()).Icon(theme.MenuIcon(theme.IconTrash)).IconColor(p.Danger).Danger().Chosen() {
			a.DeleteGroup(g.ID)
		}
	}

	var mainArea ui.Element
	row.Children(func() {
		if selected && !dragOver {
			ui.Box(c).Absolute().Left(0).Top(9).Size(3, 12).Radius(2).Background(p.Accent)
		}
		mainArea = ui.Row(c).Grow(1).MinWidth(0).FillHeight().AlignItems(ui.Center).Gap(6)
		mainArea.Children(func() {
			txt := ui.Text(c, g.Name).Grow(1).MinWidth(0).SingleLine()
			switch {
			case dragOver:
				txt.TextColor(p.AccentText).FontWeight(600)
				ui.Text(c, locales.MainDropMoveCount(len(drag.IDs))).FontSize(11.5).FontWeight(500).TextColor(p.AccentText).SingleLine()
			case selected:
				txt.TextColor(p.Text).FontWeight(500)
			default:
				txt.TextColor(p.Text2)
			}
			if !dragOver && !hovered && !*menuOpen {
				ui.Textf(c, "%d", count).FontSize(12).TextColor(p.Muted).SingleLine().Shrink(0).Padding(0, 4, 0, 0)
			}
		})

		if !dragOver && (hovered || *menuOpen) {
			moreBtn := ui.ButtonBase(c).Label(locales.MenuMoreActions()).Tooltip(locales.MenuMoreActions()).
				Size(22, 22).Radius(6).Center().TextColor(p.Text2).Shrink(0)
			if moreBtn.Hovered() || *menuOpen {
				moreBtn.Background(p.SideSel.Mix(p.Text, 0.08)).TextColor(p.Text)
			} else if selected {
				moreBtn.Background(p.SideSel.Mix(p.Text, 0.04))
			}
			moreBtn.Children(func() { ui.Icon(c, theme.IconMore).FontSize(12) })
			moreBtn.PopoverMenuButton(buildGroupMenu, menuButtonOptions(p, menuOpen, true))
		}
	})

	quiet := s.groupSort.Item(g.ID, row, mainArea)
	if s.groupSort.Active(g.ID) {
		row.Background(p.SideSel).Shadow(0, 6, 16, -6, sortLiftShadow(p))
	}
	if mainArea.Clicked() && !quiet {
		s.closeSearch(c)
		s.cursorID = ""
		s.subTabsScroll.X = 0
		s.browseList.ScrollTo(0, ui.Start)
		s.stopBrowseFollow()
		a.Go(g.ID, "")
	}
	row.PopoverMenu(buildGroupMenu, menuOptions(p))
}

func (s *mainState) buildNewGroupInput(c *ui.Context, p *theme.Palette) {
	a := s.app
	box := ui.Row(c.Key("new-grp-input")).Height(30).PaddingX(8).Radius(7).AlignItems(ui.Center).
		Background(p.Surface).Border(1.5, p.Accent)
	box.Children(func() {
		in := ui.TextInputBase(c, &s.newGroupDraft).Bind(&s.newGroupHandle).
			Placeholder(locales.MainGroupNamePlaceholder()).Label(locales.ActionNewGroup()).Grow(1).TextColor(p.Text)
		handleInlineEditInput(in, &s.newGroupFocused, func() {
			if name := strings.TrimSpace(s.newGroupDraft); name != "" {
				a.AddGroup(name)
				s.cursorID = ""
				s.browseList.ScrollTo(0, ui.Start)
				s.stopBrowseFollow()
			}
			s.addingGroup = false
		}, func() {
			s.addingGroup = false
		})
	})
}

func (s *mainState) startRenameGroup(c *ui.Context, g model.Group) {
	s.addingGroup = false
	s.addingSub = false
	s.renamingSubID = ""
	s.renamingGroupID = g.ID
	s.renameDraft = g.Name
	s.renameFocused = false
	s.renameHandle.SelectAll(c)
}

func (s *mainState) startAddGroup(c *ui.Context) {
	s.renamingGroupID = ""
	s.renamingSubID = ""
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
		subID = a.AddSub(groupID, locales.DefaultSub())
	}
	a.Move(ids, groupID, subID)
}
