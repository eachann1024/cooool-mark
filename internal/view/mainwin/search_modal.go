package mainwin

import (
	"cmp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

func (s *mainState) buildSearchInline(c *ui.Context, p *theme.Palette) {
	if !s.searchOpen {
		return
	}
	a := s.app
	lib := a.Store.Lib()
	q := strings.TrimSpace(s.query)
	isURL := looksLikeURL(q)

	var hits []app.Hit
	if q == "" {
		hits = s.recentHits(lib, s.searchScope, 6)
	} else {
		hits = s.searchHits()
	}

	offset := 0
	if isURL {
		offset = 1
	}
	total := len(hits) + offset
	if total == 0 {
		s.searchIdx = 0
	} else {
		s.searchIdx = max(0, min(s.searchIdx, total-1))
	}

	bar := c.TitleBar()
	h := widget.BarH(bar)
	padR := max(float32(12), bar.Right+8)

	// Header: the search field takes over the sub-group tab bar.
	var searchIn ui.Element
	ui.Row(c.Key("si-hdr")).Height(h).Shrink(0).AlignItems(ui.Center).Gap(8).
		Padding(0, padR, 0, 16).DragWindow().Children(func() {
		field := ui.Row(c.Key("si-field")).Grow(1).MinWidth(0).Height(30).Radius(8).
			Padding(0, 6, 0, 10).Gap(8).AlignItems(ui.Center).
			Background(p.Surface).Border(1.5, p.Accent)
		field.Children(func() {
			ui.Icon(c, theme.IconSearch).FontSize(14).TextColor(p.Accent).Shrink(0)
			searchIn = ui.TextInputBase(c.Key("sm-q"), &s.query).Bind(&s.searchHandle).
				Placeholder(locales.SearchModalPlaceholder()).Label(locales.MainSearchPlaceholder()).
				FontSize(13.5).Grow(1).MinWidth(0).TextColor(p.Text).PlaceholderColor(p.Muted)

			if s.query != "" {
				clr := ui.ButtonBase(c.Key("sm-clear")).Label(locales.ActionClearSearch()).Tooltip(locales.ActionClearSearch()).
					Size(20, 20).Radius(5).Center().TextColor(p.Muted).Shrink(0)
				if clr.Hovered() {
					clr.Background(p.RowSel).TextColor(p.Text)
				}
				clr.Children(func() { ui.Icon(c, theme.IconClose).FontSize(12) })
				if clr.Clicked() {
					s.query = ""
					s.searchIdx = 0
					s.tplBookmarkID = ""
					s.searchList.ScrollTo(0, ui.Start)
					s.searchHandle.Focus(c)
				}
			}

			escBtn := ui.ButtonBase(c.Key("sm-esc")).Label(locales.ActionCloseSearch()).Tooltip(locales.SearchModalCloseTip()).
				Height(20).PaddingX(7).Radius(5).AlignItems(ui.Center).
				Background(p.Field).TextColor(p.Muted).Shrink(0)
			if escBtn.Hovered() {
				escBtn.Background(p.RowSel).TextColor(p.Text)
			}
			escBtn.Children(func() { ui.Text(c, "Esc").FontSize(11.5).SingleLine() })
			if escBtn.Clicked() {
				s.closeSearch(c)
			}
		})
	})
	ui.Box(c).Height(1).FillWidth().Background(p.Line).Shrink(0)
	if !s.searchOpen {
		return
	}

	start, end := searchIn.TextSelection()
	hasSel := start != end
	svc := c.Services()
	searchIn.OnTextKey(func(k ui.TextKey) bool {
		if s.moveOpen {
			return false
		}
		b := selectedSearchBookmark(hits, isURL, s.searchIdx)
		if b == nil {
			return false
		}
		switch {
		case k.Key == ui.KeyC && k.Mods == ui.Cmd && !hasSel:
			if b.URL != "" {
				svc.WriteClipboard(b.URL)
				a.Say(locales.ToastCopiedURL())
				return true
			}
		case k.Key == ui.KeyC && k.Mods == ui.Cmd|ui.Shift:
			if b.Title != "" {
				svc.WriteClipboard(b.Title)
				a.Say(locales.ToastCopiedTitle())
				return true
			}
		}
		return false
	})

	if searchIn.Changed() {
		s.tplBookmarkID = ""
		s.searchIdx = 0
		s.searchList.ScrollTo(0, ui.Start)
	}
	if searchIn.Shortcut(0, ui.KeyEscape) {
		if s.tplBookmarkID != "" {
			s.tplBookmarkID = ""
			s.tplQuery = ""
		} else {
			s.closeSearch(c)
			return
		}
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
		if !s.searchOpen {
			return
		}
	}

	if !s.moveOpen && !s.tplInputHandle.Focused(c) {
		if b := selectedSearchBookmark(hits, isURL, s.searchIdx); b != nil {
			if !s.searchHandle.Focused(c) {
				if c.Shortcut(ui.Cmd, ui.KeyC) && !c.ShortcutRepeated() {
					s.copyBookmarkURL(c, b.ID)
				}
				if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyC) && !c.ShortcutRepeated() {
					s.copyBookmarkTitle(c, b.ID)
				}
			}
			if c.Shortcut(ui.Cmd, ui.KeyE) && !c.ShortcutRepeated() {
				if a.Win.ShowEditor != nil {
					a.Win.ShowEditor(app.EditorRequest{ID: b.ID})
				}
			}
			if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyM) && !c.ShortcutRepeated() {
				s.openMoveModal([]string{b.ID})
			}
		}
	}

	// Scope chips strip.
	if len(lib.Groups) > 0 {
		widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("sm-scopes"))).
			ScrollbarInsets(0, 24, 0, 24).FillWidth().Height(40).Shrink(0).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Padding(10, 24, 4, 24).Children(func() {
				s.buildScopeChip(c.Key("sm-scope-all"), p, "", locales.MainAllGroups())
				for _, g := range lib.Groups {
					s.buildScopeChip(c.Key("sm-scope-"+g.ID), p, g.ID, g.Name)
				}
			})
		})
	}

	// Results fill the content area.
	body := widget.AutoScrollbar(c, ui.Scroll(c.Key("sm-scroll"))).Grow(1).MinHeight(0).FillWidth()
	body.Children(func() {
		ui.Column(c).FillWidth().Padding(6, 14, 24, 14).Gap(2).Children(func() {
			if isURL {
				s.buildPasteURLCard(c, p, lib, q, s.searchIdx == 0)
			}

			if len(hits) > 0 {
				status := locales.SearchFooterMatches(len(hits))
				if q == "" {
					status = locales.SearchRecentUpdates()
				}
				ui.Text(c, status).FontSize(12).TextColor(p.Muted).Padding(4, 10, 6, 10)
			}

			if !isURL && len(hits) == 0 {
				ui.Column(c).FillWidth().Padding(48, 16).Center().Gap(12).Children(func() {
					msg := locales.MainNoBookmarks()
					if q != "" {
						msg = locales.SearchNotFoundFor(q)
					}
					ui.Text(c, msg).FontSize(13.5).TextColor(p.Muted).SingleLine().Selectable()
					ui.Row(c).Wrap().Center().Gap(8).Children(func() {
						if s.searchScope != "" {
							allBtn := widget.Secondary(c, p, locales.MainSearchAllGroups()).
								Label(locales.MainSearchAllGroups()).Tooltip(locales.MainSearchAllGroupsTip())
							if allBtn.Clicked() {
								s.searchScope = ""
								s.searchIdx = 0
								s.searchHandle.Focus(c)
							}
						}
						if q != "" && a.Win.ShowEditor != nil {
							newBtn := widget.Secondary(c, p, locales.MenuNewBookmark()).Label(locales.MenuNewBookmark())
							if newBtn.Clicked() {
								s.closeSearch(c)
								a.Win.ShowEditor(app.EditorRequest{Title: q, GroupID: a.Group, SubID: a.Sub})
							}
						}
					})
				})
				return
			}

			for i, hit := range hits {
				idx := i + offset
				s.buildModalSearchRow(c, p, hit, idx, s.searchScrollMoved && idx == s.searchIdx)
				if s.tplBookmarkID == hit.B.ID {
					s.buildTemplatePrompt(c, p, hit.B)
				}
			}
			s.searchScrollMoved = false
		})
	})
}

func selectedSearchBookmark(hits []app.Hit, isURL bool, idx int) *model.Bookmark {
	hitIdx := idx
	if isURL {
		hitIdx--
	}
	if hitIdx >= 0 && hitIdx < len(hits) {
		return hits[hitIdx].B
	}
	return nil
}

func (s *mainState) recentHits(lib *model.Library, groupID string, limit int) []app.Hit {
	a := s.app
	live := lib.Live()
	filtered := make([]*model.Bookmark, 0, len(live))
	for _, b := range live {
		if groupID != "" && b.GroupID != groupID {
			continue
		}
		filtered = append(filtered, b)
	}
	slices.SortStableFunc(filtered, func(x, y *model.Bookmark) int {
		if !x.UpdatedAt.Equal(y.UpdatedAt) {
			if x.UpdatedAt.After(y.UpdatedAt) {
				return -1
			}
			return 1
		}
		return cmp.Compare(x.Order, y.Order)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	out := make([]app.Hit, len(filtered))
	for i, b := range filtered {
		out[i] = app.Hit{B: b, Path: a.PathOf(b)}
	}
	return out
}

func (s *mainState) buildModalSearchRow(c *ui.Context, p *theme.Palette, hit app.Hit, idx int, reveal bool) {
	a := s.app
	b := hit.B
	on := idx == s.searchIdx
	row := ui.Row(c.Key("smr-"+b.ID)).MinHeight(38).Padding(6, 10, 6, 12).Radius(9).Gap(11).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).Role(ui.RoleOption).Label(b.Title)
	hovered := row.Hovered()
	if on {
		row.Background(p.RowSel)
		if reveal {
			row.ScrollIntoView()
		}
	} else if hovered {
		row.Background(p.Hover)
	}

	path := hit.Path
	if path == "" {
		path = a.PathOf(b)
	}

	row.Children(func() {
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
		ui.Text(c, path).FontSize(12).TextColor(p.Muted).MaxWidth(140).SingleLine().Shrink(0).Tooltip(path)

		if on {
			ui.Row(c).Padding(2, 6).Radius(6).Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
				ui.Text(c, "↵").FontSize(11.5).TextColor(p.Muted).SingleLine()
			})
		}
	})

	if row.Clicked() {
		s.searchIdx = idx
		s.activateSearchBookmark(c, b)
	}
	row.PopoverMenu(func(m *ui.Menu) {
		s.searchIdx = idx
		s.buildBookmarkMenu(c, m, b)
	}, menuOptions(p))
}

func (s *mainState) activateSearchBookmark(c *ui.Context, b *model.Bookmark) {
	if b == nil {
		return
	}
	if b.IsTemplate() {
		s.activateBookmark(c, b)
		return
	}
	if s.app.Open(b, "") {
		s.closeSearch(c)
	}
}
