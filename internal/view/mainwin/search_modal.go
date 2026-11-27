package mainwin

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

func (s *mainState) resolveLockedEngine(lib *model.Library) *model.Bookmark {
	if s.lockedEngineID == "" {
		return nil
	}
	b := lib.Bookmark(s.lockedEngineID)
	if b == nil || b.InTrash() || !b.IsTemplate() {
		s.lockedEngineID = ""
		s.lockedKeyword = ""
		return nil
	}
	return b
}

func (s *mainState) lockEngine(c *ui.Context, eng *model.Bookmark, kw, rest string) {
	if eng == nil {
		return
	}
	if kw == "" && len(eng.Keywords) > 0 {
		kw = eng.Keywords[0]
	}
	s.lockedEngineID = eng.ID
	s.lockedKeyword = kw
	s.query = rest
	s.searchIdx = 0
	s.searchScrollMoved = false
	s.tplBookmarkID = ""
	s.tplQuery = ""
	s.pendingSearchCaret = len([]rune(rest))
	s.searchList.ScrollTo(0, ui.Start)
	if c != nil {
		s.searchHandle.Focus(c)
	}
}

func (s *mainState) unlockEngine(c *ui.Context, restoreKeyword bool) {
	kw := s.lockedKeyword
	s.lockedEngineID = ""
	s.lockedKeyword = ""
	s.searchIdx = 0
	s.searchScrollMoved = false
	s.searchList.ScrollTo(0, ui.Start)
	if restoreKeyword && kw != "" {
		s.query = kw
		s.pendingSearchCaret = len([]rune(kw))
	}
	if c != nil {
		s.searchHandle.Focus(c)
	}
}

func (s *mainState) handleSearchEscape(c *ui.Context) {
	switch {
	case s.lockedEngineID != "":
		s.unlockEngine(c, false)
	case s.query != "":
		s.query = ""
		s.searchIdx = 0
		s.searchScrollMoved = false
		s.searchList.ScrollTo(0, ui.Start)
		if c != nil {
			s.searchHandle.Focus(c)
		}
	default:
		s.closeSearch(c)
	}
}

func (s *mainState) buildSearchInline(c *ui.Context, p *theme.Palette) {
	if !s.searchOpen {
		return
	}
	a := s.app
	lib := a.Store.Lib()

	lockedEngine := s.resolveLockedEngine(lib)

	q := strings.TrimSpace(s.query)
	isURL := lockedEngine == nil && looksLikeURL(q)

	hits, engines, total := s.searchCandidates(lib, lockedEngine, q, isURL)
	if lockedEngine != nil || total == 0 {
		s.searchIdx = 0
	} else {
		s.searchIdx = max(0, min(s.searchIdx, total-1))
	}

	bar := c.TitleBar()
	h := widget.BarH(bar)
	padL := float32(16)
	if bar.Left > 0 {
		padL = bar.Left + 12
	}
	padR := float32(12)
	if bar.Right > 0 {
		padR = bar.Right + 8
	}

	placeholder := locales.SearchModalPlaceholder()
	if lockedEngine != nil {
		placeholder = locales.EngineLockedPlaceholder(lockedEngine.Title)
	}

	// Header: standalone full-width search bar clearing macOS traffic lights.
	var searchIn ui.Element
	ui.Row(c.Key("si-hdr")).Height(h).Shrink(0).AlignItems(ui.Center).Gap(8).
		Padding(0, padR, 0, padL).DragWindow().Children(func() {
		field := ui.Row(c.Key("si-field")).Grow(1).MinWidth(0).Height(30).Radius(8).
			Padding(0, 6, 0, 10).Gap(8).AlignItems(ui.Center).
			Background(p.Surface).Border(1.5, p.Accent)
		field.Children(func() {
			ui.Icon(c, theme.IconSearch).FontSize(14).TextColor(p.Accent).Shrink(0)
			if lockedEngine != nil {
				s.buildLockedEnginePill(c, p, lockedEngine)
			}
			searchIn = ui.TextInputBase(c.Key("sm-q"), &s.query).Bind(&s.searchHandle).
				Placeholder(placeholder).Label(locales.MainSearchPlaceholder()).
				FontSize(13.5).Grow(1).MinWidth(0).TextColor(p.Text).PlaceholderColor(p.Muted)
			if s.pendingSearchCaret >= 0 {
				searchIn.SetTextSelection(s.pendingSearchCaret, s.pendingSearchCaret)
				s.pendingSearchCaret = -1
			}
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
		if lockedEngine != nil && k.Key == ui.KeyBackspace && k.Mods == 0 && k.Text() == "" {
			kw := s.lockedKeyword
			s.unlockEngine(nil, false)
			if kw != "" {
				k.SetText(kw)
				s.pendingSearchCaret = len([]rune(kw))
			} else {
				k.SetText("")
			}
			return true
		}
		if lockedEngine == nil && k.Key == ui.KeyTab && k.Mods == 0 {
			if b := selectedSearchBookmark(hits, isURL, s.searchIdx); b != nil && b.IsTemplate() {
				s.lockEngine(nil, b, "", "")
				k.SetText("")
				return true
			}
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
		if lockedEngine == nil {
			if eng, rest, ok := a.ParseLock(s.query); ok && eng != nil {
				kw, _, _ := strings.Cut(s.query, " ")
				s.lockEngine(nil, eng, kw, rest)
				searchIn.ReplaceText(0, math.MaxInt, rest)
				s.pendingSearchCaret = -1
			}
		}
	}
	// Re-resolve in case sm-unlock or ParseLock changed the locked engine this frame.
	if nextLocked := s.resolveLockedEngine(lib); nextLocked != lockedEngine {
		lockedEngine = nextLocked
		q = strings.TrimSpace(s.query)
		isURL = lockedEngine == nil && looksLikeURL(q)
		hits, engines, total = s.searchCandidates(lib, lockedEngine, q, isURL)
		s.searchIdx = 0
	}
	offset := 0
	if isURL {
		offset = 1
	}

	if searchIn.Shortcut(0, ui.KeyEscape) {
		s.handleSearchEscape(c)
		if !s.searchOpen {
			return
		}
	}
	if lockedEngine == nil {
		if searchIn.Shortcut(0, ui.KeyDown) && total > 0 {
			s.searchIdx = min(s.searchIdx+1, total-1)
			s.searchScrollMoved = true
		}
		if searchIn.Shortcut(0, ui.KeyUp) && total > 0 {
			s.searchIdx = max(s.searchIdx-1, 0)
			s.searchScrollMoved = true
		}
	}
	if searchIn.Submitted() {
		if lockedEngine != nil {
			if a.Open(lockedEngine, q) {
				s.closeSearch(c)
				return
			}
		} else if total > 0 {
			s.activateSearchIndex(c, hits, engines, q, isURL, s.searchIdx)
			if !s.searchOpen {
				return
			}
		}
	}

	if !s.moveOpen && lockedEngine == nil {
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

	// Results fill the content area.
	body := widget.AutoScrollbar(c, ui.Scroll(c.Key("sm-scroll"))).Grow(1).MinHeight(0).FillWidth()
	body.Children(func() {
		ui.Column(c).FillWidth().Padding(6, 14, 24, 14).Gap(2).Children(func() {
			if lockedEngine != nil {
				s.buildLockedSearchCard(c, p, lockedEngine, q)
				return
			}
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

			if !isURL && len(hits) == 0 && len(engines) == 0 {
				s.buildSearchEmptyState(c, p, q)
				return
			}

			for i, hit := range hits {
				idx := i + offset
				s.buildModalSearchRow(c, p, hit, idx, s.searchScrollMoved && idx == s.searchIdx)
			}

			if len(engines) > 0 {
				topPad := float32(4)
				if len(hits) > 0 {
					topPad = 12
				}
				ui.Text(c, locales.EngineSearchSection(q)).
					FontSize(12).FontWeight(500).TextColor(p.Muted).
					Padding(topPad, 10, 6, 10)
				for i, eng := range engines {
					idx := offset + len(hits) + i
					s.buildEngineRow(c, p, eng, q, idx, s.searchScrollMoved && idx == s.searchIdx)
				}
			}
			s.searchScrollMoved = false
		})
	})
}

func (s *mainState) searchCandidates(lib *model.Library, lockedEngine *model.Bookmark, q string, isURL bool) (hits []app.Hit, engines []*model.Bookmark, total int) {
	if lockedEngine != nil {
		return nil, nil, 0
	}
	if q == "" {
		hits = s.recentHits(lib, s.searchScope, 6)
	} else {
		hits = s.searchHits()
		if !isURL {
			engines = s.app.Engines()
		}
	}
	total = len(hits) + len(engines)
	if isURL {
		total++
	}
	return hits, engines, total
}

func (s *mainState) buildLockedEnginePill(c *ui.Context, p *theme.Palette, eng *model.Bookmark) {
	pill := ui.Row(c.Key("sm-lock-pill")).Height(22).Padding(0, 4, 0, 6).Radius(999).Gap(5).
		AlignItems(ui.Center).Background(p.AccentBg).Shrink(0)
	pill.Children(func() {
		s.buildTile(c, eng, 14)
		ui.Text(c, eng.Title).FontSize(12).FontWeight(500).TextColor(p.AccentText).MaxWidth(140).SingleLine()
		unlockBtn := ui.ButtonBase(c.Key("sm-unlock")).Label(locales.EngineUnlock()).Tooltip(locales.EngineUnlock()).
			Size(16, 16).Radius(999).Center().TextColor(p.AccentText).Shrink(0)
		if unlockBtn.Hovered() {
			unlockBtn.Background(p.AccentText.Alpha(0.14))
		}
		unlockBtn.Children(func() { ui.Icon(c, theme.IconClose).FontSize(10) })
		s.releaseFocus(c, unlockBtn, nil)
		if unlockBtn.Clicked() {
			s.unlockEngine(c, false)
		}
	})
}

func (s *mainState) buildLockedSearchCard(c *ui.Context, p *theme.Palette, eng *model.Bookmark, query string) {
	a := s.app
	card := ui.Row(c.Key("sm-locked-card")).Padding(12, 14).Margin(8, 0, 6, 0).Radius(10).Gap(12).
		AlignItems(ui.Center).Background(p.RowSel).Border(1.5, p.Accent).Cursor(ui.CursorPointer)
	card.Children(func() {
		s.buildTile(c, eng, 32)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, locales.EngineLockedTitle(eng.Title, query)).
				FontSize(13.5).FontWeight(600).TextColor(p.Text).SingleLine()
			ui.RichText(c, lockedURLPreviewSpans(eng.URL, query, p)...).SingleLine()
		})
		ui.Row(c).Padding(3, 8).Radius(6).Gap(5).AlignItems(ui.Center).
			Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
			ui.Text(c, locales.EngineGo()).FontSize(12).TextColor(p.Text2).SingleLine()
			ui.Text(c, "↵").FontSize(11.5).TextColor(p.Muted).SingleLine()
		})
	})
	if card.Clicked() && a.Open(eng, query) {
		s.closeSearch(c)
	}
}

func lockedURLPreviewSpans(rawURL, query string, p *theme.Palette) []ui.Span {
	preview := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
	if !strings.Contains(preview, "{query}") {
		return []ui.Span{{Text: preview, Color: p.Muted, Size: 12}}
	}
	q := query
	if q == "" {
		q = "{query}"
	}
	parts := strings.Split(preview, "{query}")
	spans := make([]ui.Span, 0, len(parts)*2-1)
	for i, part := range parts {
		if part != "" {
			spans = append(spans, ui.Span{Text: part, Color: p.Muted, Size: 12})
		}
		if i < len(parts)-1 {
			spans = append(spans, ui.Span{
				Text:       q,
				Color:      p.AccentText,
				Background: p.AccentBg,
				Size:       12,
				Weight:     600,
				Radius:     3,
			})
		}
	}
	return spans
}

func (s *mainState) buildSearchEmptyState(c *ui.Context, p *theme.Palette, q string) {
	a := s.app
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
}

func (s *mainState) buildEngineRow(c *ui.Context, p *theme.Palette, eng *model.Bookmark, query string, idx int, reveal bool) {
	a := s.app
	on := idx == s.searchIdx
	row := ui.Row(c.Key("sme-"+eng.ID)).MinHeight(38).Padding(6, 10, 6, 12).Radius(9).Gap(11).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).Role(ui.RoleOption).Label(eng.Title)
	hovered := row.Hovered()
	if on {
		row.Background(p.RowSel)
		if reveal {
			row.ScrollIntoView()
		}
	} else if hovered {
		row.Background(p.Hover)
	}

	row.Children(func() {
		s.buildTile(c, eng, 26)
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, query).FontSize(13.5).FontWeight(500).TextColor(p.Text).SingleLine().Shrink(1).MinWidth(0)
			ui.Text(c, locales.EngineRowWith(eng.Title)).FontSize(13).TextColor(p.Muted).SingleLine().Shrink(0)
		})
		if len(eng.Keywords) > 0 {
			kwBg := p.Field
			if on {
				kwBg = p.Surface
			}
			kwPill := ui.Row(c).Padding(2, 6).Radius(5).Background(kwBg).Shrink(0)
			if on {
				kwPill.Border(1, p.Line)
			}
			kwPill.Children(func() {
				ui.Text(c, eng.Keywords[0]).FontSize(11.5).TextColor(p.Muted).SingleLine()
			})
		}
		if on {
			ui.Row(c).Padding(2, 6).Radius(6).Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
				ui.Text(c, "↵").FontSize(11.5).TextColor(p.Muted).SingleLine()
			})
		}
	})

	if row.Clicked() {
		s.searchIdx = idx
		if a.Open(eng, query) {
			s.closeSearch(c)
		}
	}
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
	pathH := ui.Local(row, "pathH", func() ui.Handle { return ui.Handle{} })
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
				if a.IsEnriching(b.ID) {
					enrichingBadge(c, p)
				} else if _, polished := a.PolishedAt[b.ID]; polished {
					polishedBadge(c, p)
				}
			})
			ui.Text(c, shortDomain(b.URL)).FontSize(12).TextColor(p.Muted).SingleLine()
		})
		pathEl := ui.Text(c, path).FontSize(12).TextColor(p.Muted).MaxWidth(140).SingleLine().Shrink(0).Bind(pathH)
		if widget.TextTruncated(c, pathH, path, 12, 0, 1) {
			pathEl.Tooltip(path)
		}

		if on {
			if b.IsTemplate() {
				ui.Row(c).Padding(2, 7).Radius(6).Gap(5).AlignItems(ui.Center).
					Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
					ui.Text(c, locales.EngineTabLock()).FontSize(11.5).TextColor(p.Muted).SingleLine()
					ui.Text(c, "↵").FontSize(11.5).TextColor(p.Muted).SingleLine()
				})
			} else {
				ui.Row(c).Padding(2, 6).Radius(6).Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
					ui.Text(c, "↵").FontSize(11.5).TextColor(p.Muted).SingleLine()
				})
			}
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
		s.lockEngine(c, b, "", "")
		return
	}
	if s.app.Open(b, "") {
		s.closeSearch(c)
	}
}
