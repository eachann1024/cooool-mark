package mainwin

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

func (s *mainState) buildSidebar(c *ui.Context, p *theme.Palette) {
	a := s.app
	lib := a.Store.Lib()
	g := lib.Group(a.Group)

	bar := c.TitleBar()
	topH := widget.BarH(bar)

	aside := ui.Column(c).Width(184).Shrink(0).FillHeight().
		Background(p.Sidebar).BorderWidth(0, 1, 0, 0).BorderColor(p.Line)
	aside.Children(func() {
		// Top bar: macOS traffic lights live here; Windows/Linux show title.
		top := ui.Row(c).Height(topH).Shrink(0).AlignItems(ui.Center).Padding(0, 12, 0, 16).DragWindow()
		top.Children(func() {
			if runtime.GOOS != "darwin" || bar.Left == 0 {
				ui.Text(c, "Cooool Mark").Font(theme.Serif).FontSize(14).FontWeight(600).TextColor(p.Text2).SingleLine()
			}
		})

		// Sub-group list.
		nav := widget.AutoScrollbar(c, ui.Scroll(c)).Grow(1).MinHeight(0).Padding(6, 10)
		nav.Children(func() {
			if g == nil {
				s.buildSidebarEmpty(c, p, "还没有分组", "新建分组", func() { s.startAddGroup(c) })
				return
			}
			if len(g.Subs) == 0 && !s.addingSub {
				s.buildSidebarEmpty(c, p, "还没有子分组", "新建子分组", func() { s.startAddSub(c) })
				return
			}
			col := ui.Column(c).Gap(1)
			col.Children(func() {
				for i, sub := range g.Subs {
					s.buildSidebarSubItem(c, p, lib, g, i, sub)
				}
				if s.addingSub {
					s.buildNewSubInput(c, p, g.ID)
				}
			})
		})

		// Bottom bar: new sub-group button (only when the sidebar isn't showing its empty state).
		if g != nil && (len(g.Subs) > 0 || s.addingSub) {
			ui.Row(c).Shrink(0).Padding(8, 10, 12, 10).Children(func() {
				btn := ui.ButtonBase(c).Label("新建子分组").Tooltip("新建子分组").
					Height(28).FillWidth().PaddingX(10).Radius(7).Gap(6).AlignItems(ui.Center).
					TextColor(p.Muted)
				if btn.Hovered() {
					btn.Background(p.Hover.Mix(p.SideSel, 0.7)).TextColor(p.Text)
				}
				btn.Children(func() {
					ui.Icon(c, theme.IconPlus).FontSize(14).Shrink(0)
					ui.Text(c, "新建子分组").FontSize(12.5).SingleLine()
				})
				if btn.Clicked() {
					s.startAddSub(c)
				}
			})
		}
	})
}

func (s *mainState) buildSidebarEmpty(c *ui.Context, p *theme.Palette, title, btnLabel string, onClick func()) {
	ui.Column(c).FillWidth().Padding(28, 8, 12, 8).Center().Gap(10).Children(func() {
		ui.Row(c).Size(34, 34).Radius(9).Background(p.SideSel.Mix(p.Sidebar, 0.4)).Center().Children(func() {
			ui.Icon(c, theme.IconEmptyFolder).FontSize(16).TextColor(p.Muted)
		})
		ui.Text(c, title).FontSize(12.5).TextColor(p.Muted).SingleLine()
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

func (s *mainState) buildSidebarSubItem(c *ui.Context, p *theme.Palette, lib *model.Library, g *model.Group, idx int, sub model.Sub) {
	a := s.app
	keyC := c.Key("sub-" + sub.ID)

	if s.renamingSubID == sub.ID {
		box := ui.Row(keyC).Height(30).PaddingX(8).Radius(7).AlignItems(ui.Center).
			Background(p.Surface).Border(1.5, p.Accent)
		box.Children(func() {
			in := ui.TextInputBase(c, &s.renameDraft).Bind(&s.renameHandle).
				Placeholder("子分组名称").Label("重命名子分组").Grow(1).TextColor(p.Text)
			if in.Focused() {
				s.renameFocused = true
			}
			switch {
			case in.Submitted():
				if name := strings.TrimSpace(s.renameDraft); name != "" {
					a.RenameSub(g.ID, sub.ID, name)
				}
				s.renamingSubID = ""
				s.renameFocused = false
			case in.Shortcut(0, ui.KeyEscape):
				s.renamingSubID = ""
				s.renameFocused = false
			case s.renameFocused && !in.Focused():
				if name := strings.TrimSpace(s.renameDraft); name != "" {
					a.RenameSub(g.ID, sub.ID, name)
				}
				s.renamingSubID = ""
				s.renameFocused = false
			}
		})
		return
	}

	selected := sub.ID == a.Sub && strings.TrimSpace(s.query) == ""
	row := ui.Row(keyC).Height(30).Padding(0, 6, 0, 12).Radius(7).
		AlignItems(ui.Center).Gap(4).Cursor(ui.CursorPointer)
	menuOpen := ui.Local(row, "subMenuOpen", func() bool { return false })

	drag, dragOver := ui.DragOver[dragPayload](row)
	if drop, ok := ui.Drop[dragPayload](row); ok {
		a.Move(drop.IDs, g.ID, sub.ID)
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

	buildSubMenu := func(m *ui.Menu) {
		if m.Item("重命名").Chosen() {
			s.startRenameSub(c, sub)
		}
		if m.Item("新建书签到这里").Chosen() {
			if a.Win.ShowEditor != nil {
				a.Win.ShowEditor(app.EditorRequest{GroupID: g.ID, SubID: sub.ID})
			}
		}
		m.Separator()
		if m.Item("上移").Disabled(idx == 0).Chosen() {
			reorderSubByDelta(a, g, idx, -1)
		}
		if m.Item("下移").Disabled(idx >= len(g.Subs)-1).Chosen() {
			reorderSubByDelta(a, g, idx, 1)
		}
		if len(lib.Groups) > 1 {
			m.Submenu("移动到其他分组", func(sm *ui.Menu) {
				for _, targetG := range lib.Groups {
					if targetG.ID == g.ID {
						continue
					}
					if sm.Item(targetG.Name).ID("mv-" + targetG.ID).Chosen() {
						a.MoveSub(g.ID, sub.ID, targetG.ID)
						a.EnsurePlace()
					}
				}
			})
		}
		if m.Item("提升为一级分组").Chosen() {
			a.PromoteSub(g.ID, sub.ID)
			a.EnsurePlace()
		}
		m.Separator()
		if m.Item("删除子分组").Danger().Chosen() {
			a.DeleteSub(g.ID, sub.ID)
		}
	}

	var mainArea ui.Element
	row.Children(func() {
		if selected && !dragOver {
			ui.Box(c).Absolute().Left(0).Top(9).Size(3, 12).Radius(2).Background(p.Accent)
		}
		mainArea = ui.Row(c).Grow(1).MinWidth(0).FillHeight().AlignItems(ui.Center).Gap(6)
		mainArea.Children(func() {
			txt := ui.Text(c, sub.Name).Grow(1).MinWidth(0).SingleLine()
			switch {
			case dragOver:
				txt.TextColor(p.AccentText).FontWeight(600)
				ui.Text(c, fmt.Sprintf("移入 %d", len(drag.IDs))).FontSize(11.5).FontWeight(500).TextColor(p.AccentText).SingleLine()
			case selected:
				txt.TextColor(p.Text).FontWeight(500)
			default:
				txt.TextColor(p.Text2)
			}
		})

		if !dragOver && (hovered || selected || *menuOpen) {
			moreBtn := ui.ButtonBase(c).Label("子分组操作").Tooltip("子分组操作").
				Size(22, 22).Radius(6).Center().TextColor(p.Text2).Shrink(0)
			if moreBtn.Hovered() || *menuOpen {
				moreBtn.Background(p.SideSel.Mix(p.Text, 0.08)).TextColor(p.Text)
			} else if selected {
				moreBtn.Background(p.SideSel.Mix(p.Text, 0.04))
			}
			moreBtn.Children(func() { ui.Icon(c, theme.IconMore).FontSize(12) })
			moreBtn.PopoverMenuButton(buildSubMenu, menuButtonOptions(p, menuOpen, true))
		}
	})

	if mainArea.Clicked() {
		s.clearSearch()
		s.scrollToSub = sub.ID
		a.Go(g.ID, sub.ID)
	}
	row.PopoverMenu(buildSubMenu, menuOptions(p))
}

func (s *mainState) buildNewSubInput(c *ui.Context, p *theme.Palette, groupID string) {
	a := s.app
	box := ui.Row(c.Key("new-sub-input")).Height(30).PaddingX(8).Radius(7).AlignItems(ui.Center).
		Background(p.Surface).Border(1.5, p.Accent)
	box.Children(func() {
		in := ui.TextInputBase(c, &s.newSubDraft).Bind(&s.newSubHandle).
			Placeholder("子分组名称").Grow(1).TextColor(p.Text)
		if in.Focused() {
			s.newSubFocused = true
		}
		switch {
		case in.Submitted():
			if name := strings.TrimSpace(s.newSubDraft); name != "" {
				id := a.AddSub(groupID, name)
				a.Go(groupID, id)
				s.scrollToSub = id
			}
			s.addingSub = false
			s.newSubFocused = false
		case in.Shortcut(0, ui.KeyEscape):
			s.addingSub = false
			s.newSubFocused = false
		case s.newSubFocused && !in.Focused():
			if name := strings.TrimSpace(s.newSubDraft); name != "" {
				id := a.AddSub(groupID, name)
				a.Go(groupID, id)
				s.scrollToSub = id
			}
			s.addingSub = false
			s.newSubFocused = false
		}
	})
}

func (s *mainState) startRenameSub(c *ui.Context, sub model.Sub) {
	s.renamingGroupID = ""
	s.renamingSubID = sub.ID
	s.renameDraft = sub.Name
	s.renameFocused = false
	s.renameHandle.SelectAll(c)
}

func (s *mainState) startAddSub(c *ui.Context) {
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
