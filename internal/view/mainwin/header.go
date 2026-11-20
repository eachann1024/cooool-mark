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
	header.Children(func() {
		if showSubBar {
			// Sub-group pills zone: scrollable tabs + pinned "+" button.
			ui.Row(c.Key("main-tabs-zone")).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(4).Children(func() {
				pills := widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("main-tabs"))).
					ScrollbarInsets(0, 4, 0, 4).Grow(1).MinWidth(0).Height(32).Padding(2, 6, 4, 0)
				pills.Children(func() {
					row := ui.Row(c.Key("main-tabs-row")).AlignItems(ui.Center).Gap(4)
					row.Children(func() {
						for i, sub := range g.Subs {
							selected := sub.ID == a.Sub && strings.TrimSpace(s.query) == ""
							s.buildSubPill(c, p, lib, g, i, sub, selected)
						}
					})
				})
				if s.addingSub {
					s.buildNewSubInput(c, p, g.ID)
				} else {
					addSub := ui.ButtonBase(c.Key("add-sub-btn")).Label(locales.ActionNewSub()).Tooltip(locales.ActionNewSub()).
						Size(26, 26).Radius(999).Center().TextColor(p.Muted).Shrink(0)
					if addSub.Hovered() {
						addSub.Background(p.RowSel).TextColor(p.Text)
					}
					addSub.Children(func() { ui.Icon(c, theme.IconPlus).FontSize(14) })
					if addSub.Clicked() {
						s.startAddSub(c)
					}
				}
			})

			ui.Box(c).Size(1, 16).Background(p.Line).Shrink(0)
		} else {
			ui.Box(c).Grow(1).MinWidth(0)
		}

		// Action buttons (compact search icon frees header width for sub-group tabs).
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
	if pill.Clicked() {
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
