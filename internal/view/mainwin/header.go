package mainwin

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
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
					for i, g := range lib.Groups {
						s.buildGroupPill(c, p, lib, i, g, g.ID == a.Group)
					}
				})
			})
			if s.addingGroup {
				s.buildNewGroupInput(c, p)
			} else {
				addG := ui.ButtonBase(c.Key("add-grp-btn")).Label(locales.ActionNewGroup()).Tooltip(locales.ActionNewGroup()).
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

		// Action buttons (compact search icon frees header width for more group tabs).
		searchBtn := widget.IconButton(c.Key("open-search-btn"), p, theme.IconSearch, locales.MenuSearchBookmarks()).Shrink(0)
		if searchBtn.Clicked() {
			s.openSearch(c, "", false)
		}

		newBtn := widget.IconButton(c, p, theme.IconPlus, locales.MenuNewBookmark()).Shrink(0)
		if newBtn.Clicked() && a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
		}

		isGrid := a.View == "grid"
		viewIcon, viewLabel := theme.IconGrid, locales.ActionSwitchToGrid()
		if isGrid {
			viewIcon, viewLabel = theme.IconList, locales.ActionSwitchToList()
		}
		viewBtn := widget.IconButton(c, p, viewIcon, viewLabel).Shrink(0)
		if viewBtn.Clicked() {
			s.toggleView()
		}

		if isGrid {
			sizeIcon := theme.IconCardStandard
			switch model.NormalizeCardSize(a.Settings.CardSize) {
			case model.CardCompact:
				sizeIcon = theme.IconCardCompact
			case model.CardDetail:
				sizeIcon = theme.IconCardDetail
			}
			sizeBtn := widget.IconButton(c.Key("card-size-btn"), p, sizeIcon, locales.MenuCardSize()).Shrink(0)
			sizeBtn.PopoverMenuButton(func(m *ui.Menu) {
				s.buildCardSizeItems(m)
			}, menuButtonOptions(p, nil, true))
		}

		moreBtn := widget.IconButton(c, p, theme.IconMore, locales.MenuMore()).Shrink(0)
		moreBtn.PopoverMenuButton(func(m *ui.Menu) {
			if m.Item(locales.MenuNewBookmark()).Icon(theme.MenuIcon(theme.IconPlus)).Shortcut(ui.Cmd, ui.KeyN).Chosen() {
				if a.Win.ShowEditor != nil {
					a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
				}
			}
			m.Separator()
			if m.Item(locales.MenuImport()).Icon(theme.MenuIcon(theme.IconImport)).Chosen() {
				if a.Win.ShowImport != nil {
					a.Win.ShowImport("")
				}
			}
			m.Separator()
			if isGrid {
				s.buildCardSizeMenu(m)
			}
			m.Submenu(locales.MenuAppearance(), func(sm *ui.Menu) {
				modes := []struct {
					key   string
					label string
					icon  *ui.SVG
				}{
					{"auto", locales.AppearanceAuto(), theme.IconMonitor},
					{"light", locales.AppearanceLight(), theme.IconSun},
					{"dark", locales.AppearanceDark(), theme.IconMoon},
				}
				for _, mode := range modes {
					cur := a.Settings.Appearance == mode.key || (mode.key == "auto" && a.Settings.Appearance == "")
					if sm.Item(mode.label).Icon(theme.MenuIcon(mode.icon)).Radio(cur).Chosen() {
						a.Settings.Appearance = mode.key
						a.SaveSettings()
					}
				}
			}).Icon(theme.MenuIcon(theme.IconContrast))
			if m.Item(locales.MenuSettings()).Icon(theme.MenuIcon(theme.IconSettings)).Shortcut(ui.Cmd, ui.KeyComma).Chosen() {
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
				Placeholder(locales.MainGroupNamePlaceholder()).Label(locales.MainRenameGroupLabel()).Grow(1).TextColor(p.Text)
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
		if m.Item(locales.ActionRename()).Icon(theme.MenuIcon(theme.IconEdit)).Chosen() {
			s.startRenameGroup(c, g)
		}
		if m.Item(locales.ActionNewSub()).Icon(theme.MenuIcon(theme.IconFolderPlus)).Chosen() {
			a.Go(g.ID, "")
			s.startAddSub(c)
		}
		m.Separator()
		if m.Item(locales.ActionMoveLeft()).Icon(theme.MenuIcon(theme.IconBack)).Disabled(idx == 0).Chosen() {
			reorderGroupByDelta(a, lib, idx, -1)
		}
		if m.Item(locales.ActionMoveRight()).Icon(theme.MenuIcon(theme.IconChevron)).Disabled(idx >= len(lib.Groups)-1).Chosen() {
			reorderGroupByDelta(a, lib, idx, 1)
		}
		m.Separator()
		if m.Item(locales.ActionDeleteGroup()).Icon(theme.MenuIcon(theme.IconTrash)).IconColor(p.Danger).Danger().Chosen() {
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
			Placeholder(locales.MainGroupNamePlaceholder()).Label(locales.ActionNewGroup()).Grow(1).TextColor(p.Text)
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
		subID = a.AddSub(groupID, locales.DefaultSub())
	}
	a.Move(ids, groupID, subID)
}
