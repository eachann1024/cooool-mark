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
	g := lib.Group(a.Group)
	bar := c.TitleBar()
	h := widget.BarH(bar)
	padR := max(float32(12), bar.Right+8)
	showSubBar := g != nil && (len(g.Subs) > 1 || s.addingSub)

	header := ui.Row(c).Height(h).Shrink(0).AlignItems(ui.Center).Gap(8).
		Padding(0, padR, 0, 16).DragWindow()
	var pills ui.Element
	header.Children(func() {
		if showSubBar {
			// Sub-group pills zone: full-height horizontally scrollable tabs.
			ui.Row(c.Key("main-tabs-zone")).Grow(1).MinWidth(0).FillHeight().AlignItems(ui.Center).Gap(4).Children(func() {
				pills = widget.AutoScrollbarSlimState(c, ui.ScrollHorizontal(c.Key("main-tabs")), &s.subTabsScroll).
					ScrollbarInsets(0, 4, 2, 4).Grow(1).MinWidth(0).FillHeight().AlignItems(ui.Center).Padding(0, 6, 0, 0)
				pills.Children(func() {
					row := ui.Row(c.Key("main-tabs-row")).AlignItems(ui.Center).Gap(4)
					s.subSort.Axis = ui.Horizontal
					subs := append([]model.Sub(nil), g.Subs...)
					ids := make([]string, len(subs))
					index := make(map[string]int, len(subs))
					for i, sub := range subs {
						ids[i], index[sub.ID] = sub.ID, i
					}
					row.Children(func() {
						for _, id := range s.subSort.Order(ids) {
							i := index[id]
							sub := subs[i]
							selected := sub.ID == a.Sub && strings.TrimSpace(s.query) == ""
							s.buildSubPill(c, p, lib, g, i, sub, selected)
						}
					})
					if order, ok := s.subSort.Dropped(); ok {
						a.ReorderSubs(g.ID, order)
					}
				})
				if s.addingSub {
					s.buildNewSubInput(c, p, g.ID)
				}
			})
		} else {
			ui.Box(c).Grow(1).MinWidth(0)
		}

		newBtn := widget.IconButton(c.Key("new-bookmark-btn"), p, theme.IconPlus, locales.MenuNewBookmark()).Shrink(0)
		if newBtn.Clicked() && a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
		}

		isGrid := a.View == "grid"
		viewIcon, viewLabel := theme.IconGrid, locales.ActionSwitchToGrid()
		if isGrid {
			viewIcon, viewLabel = theme.IconList, locales.ActionSwitchToList()
		}

		moreBtn := widget.IconButton(c.Key("more-btn"), p, theme.IconMore, locales.MenuMore()).Shrink(0)
		moreBtn.PopoverMenuButton(func(m *ui.Menu) {
			if m.Item(locales.MenuSearchBookmarks()).Icon(theme.MenuIcon(theme.IconSearch)).Shortcut(ui.Cmd, ui.KeyF).Chosen() {
				s.openSearch(c, "", false)
			}
			if g != nil {
				if m.Item(locales.ActionNewSub()).Icon(theme.MenuIcon(theme.IconFolderPlus)).Chosen() {
					s.startAddSub(c)
				}
			}
			m.Separator()
			if m.Item(viewLabel).Icon(theme.MenuIcon(viewIcon)).Shortcut(ui.Cmd, ui.KeyL).Chosen() {
				s.toggleView()
			}
			if isGrid {
				s.buildCardSizeMenu(m)
			}
			m.Separator()
			if m.Item(locales.MenuSettings()).Icon(theme.MenuIcon(theme.IconSettings)).Shortcut(ui.Cmd, ui.KeyComma).Chosen() {
				if a.Win.ShowSettings != nil {
					a.Win.ShowSettings("")
				}
			}
		}, menuButtonOptions(p, nil, true))
	})

	if showSubBar {
		widget.ScrollHorizontalWheel(header, &s.subTabsScroll, pills)
	}
}

func handleInlineEditInput(in ui.Element, focused *bool, onCommit, onCancel func()) {
	if in.Focused() {
		*focused = true
	}
	switch {
	case in.Submitted():
		onCommit()
		*focused = false
	case in.Shortcut(0, ui.KeyEscape):
		onCancel()
		*focused = false
	case *focused && !in.Focused():
		onCommit()
		*focused = false
	}
}

func (s *mainState) buildSubPill(c *ui.Context, p *theme.Palette, lib *model.Library, g *model.Group, idx int, sub model.Sub, selected bool) {
	a := s.app
	keyC := c.Key("sub-" + sub.ID)

	if s.renamingSubID == sub.ID {
		box := ui.Row(keyC).Height(26).Width(92).PaddingX(10).Radius(999).AlignItems(ui.Center).
			Background(p.Surface).Border(1.5, p.Accent).Shrink(0)
		box.Children(func() {
			in := ui.TextInputBase(c, &s.renameDraft).Bind(&s.renameHandle).
				Placeholder(locales.MainSubNamePlaceholder()).Label(locales.MainRenameSubLabel()).Grow(1).TextColor(p.Text)
			handleInlineEditInput(in, &s.renameFocused, func() {
				if name := strings.TrimSpace(s.renameDraft); name != "" {
					a.RenameSub(g.ID, sub.ID, name)
				}
				s.renamingSubID = ""
			}, func() {
				s.renamingSubID = ""
			})
		})
		return
	}

	pill := widget.Pill(keyC, p, sub.Name, selected).Shrink(0)
	widget.RevealSelectedTab(pill, selected, idx)
	if _, over := ui.DragOver[dragPayload](pill); over {
		pill.Background(softAccentBg(p)).TextColor(p.AccentText).Border(1, p.Accent).BorderStyle(ui.BorderDashed)
	}
	if drop, ok := ui.Drop[dragPayload](pill); ok {
		a.Move(drop.IDs, g.ID, sub.ID)
	}
	quiet := s.subSort.Item(sub.ID, pill, pill)
	if s.subSort.Active(sub.ID) {
		if !selected {
			pill.Background(p.RowSel).TextColor(p.Text)
		}
		pill.Shadow(0, 4, 12, -4, sortLiftShadow(p))
	}
	if pill.Clicked() && !quiet {
		s.clearSearch()
		s.scrollToSub = sub.ID
		s.stopBrowseFollow()
		a.Go(g.ID, sub.ID)
	}

	pill.PopoverMenu(func(m *ui.Menu) {
		if m.Item(locales.ActionRename()).Icon(theme.MenuIcon(theme.IconEdit)).Chosen() {
			s.startRenameSub(c, sub)
		}
		if m.Item(locales.ActionNewBookmarkHere()).Icon(theme.MenuIcon(theme.IconPlus)).Chosen() {
			if a.Win.ShowEditor != nil {
				a.Win.ShowEditor(app.EditorRequest{GroupID: g.ID, SubID: sub.ID})
			}
		}
		m.Separator()
		if m.Item(locales.ActionMoveLeft()).Icon(theme.MenuIcon(theme.IconBack)).Disabled(idx == 0).Chosen() {
			reorderSubByDelta(a, g, idx, -1)
		}
		if m.Item(locales.ActionMoveRight()).Icon(theme.MenuIcon(theme.IconChevron)).Disabled(idx >= len(g.Subs)-1).Chosen() {
			reorderSubByDelta(a, g, idx, 1)
		}
		if len(lib.Groups) > 1 {
			m.Submenu(locales.ActionMoveToOtherGroup(), func(sm *ui.Menu) {
				for _, targetG := range lib.Groups {
					if targetG.ID == g.ID {
						continue
					}
					if sm.Item(targetG.Name).Icon(theme.MenuIcon(theme.IconFolder)).ID("mv-" + targetG.ID).Chosen() {
						a.MoveSub(g.ID, sub.ID, targetG.ID)
						a.EnsurePlace()
					}
				}
			}).Icon(theme.MenuIcon(theme.IconMove))
		}
		if m.Item(locales.ActionPromoteToGroup()).Icon(theme.MenuIcon(theme.IconPromote)).Chosen() {
			a.PromoteSub(g.ID, sub.ID)
			a.EnsurePlace()
		}
		m.Separator()
		if m.Item(locales.ActionDeleteSub()).Icon(theme.MenuIcon(theme.IconTrash)).IconColor(p.Danger).Danger().Chosen() {
			a.DeleteSub(g.ID, sub.ID)
		}
	}, menuOptions(p))
}

func (s *mainState) buildNewSubInput(c *ui.Context, p *theme.Palette, groupID string) {
	a := s.app
	box := ui.Row(c.Key("new-sub-input")).Height(26).Width(92).PaddingX(10).Radius(999).AlignItems(ui.Center).
		Background(p.Surface).Border(1.5, p.Accent).Shrink(0)
	box.Children(func() {
		in := ui.TextInputBase(c, &s.newSubDraft).Bind(&s.newSubHandle).
			Placeholder(locales.MainSubNamePlaceholder()).Label(locales.ActionNewSub()).Grow(1).TextColor(p.Text)
		handleInlineEditInput(in, &s.newSubFocused, func() {
			if name := strings.TrimSpace(s.newSubDraft); name != "" {
				id := a.AddSub(groupID, name)
				a.Go(groupID, id)
				s.scrollToSub = id
				s.stopBrowseFollow()
			}
			s.addingSub = false
		}, func() {
			s.addingSub = false
		})
	})
}

func (s *mainState) startRenameSub(c *ui.Context, sub model.Sub) {
	s.addingGroup = false
	s.addingSub = false
	s.renamingGroupID = ""
	s.renamingSubID = sub.ID
	s.renameDraft = sub.Name
	s.renameFocused = false
	s.renameHandle.SelectAll(c)
}

func (s *mainState) startAddSub(c *ui.Context) {
	s.renamingGroupID = ""
	s.renamingSubID = ""
	s.addingGroup = false
	s.addingSub = true
	s.newSubDraft = ""
	s.newSubFocused = false
	s.newSubHandle.Focus(c)
}

func reorderSubByDelta(a *app.App, g *model.Group, idx, delta int) {
	next := idx + delta
	if next < 0 || next >= len(g.Subs) {
		return
	}
	ids := make([]string, len(g.Subs))
	for i, sub := range g.Subs {
		ids[i] = sub.ID
	}
	ids[idx], ids[next] = ids[next], ids[idx]
	a.ReorderSubs(g.ID, ids)
}
