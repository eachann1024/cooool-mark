package mainwin

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

func styleMenuPanel(p *theme.Palette, panel ui.Element) {
	shadow := ui.RGBA(25, 15, 9, 0.22)
	if p.Dark {
		shadow = ui.RGBA(0, 0, 0, 0.5)
	}
	panel.Background(p.Surface).Border(1, p.Line).Radius(10).Padding(5).
		NoShadow().Shadow(0, 12, 32, -10, shadow)
}

func sortLiftShadow(p *theme.Palette) ui.Color {
	if p.Dark {
		return ui.RGBA(0, 0, 0, 0.45)
	}
	return ui.RGBA(25, 15, 9, 0.18)
}

func styleMenuItem(item ui.Element) {
	item.MinHeight(29).PaddingX(10).Radius(6)
}

func menuOptions(p *theme.Palette) ui.PopoverMenuOptions {
	return ui.PopoverMenuOptions{
		Panel: func(panel ui.Element) { styleMenuPanel(p, panel) },
		Item:  styleMenuItem,
	}
}

func menuButtonOptions(p *theme.Palette, open *bool, bottomEnd bool) ui.PopoverMenuButtonOptions {
	opts := ui.PopoverMenuButtonOptions{
		Open:  open,
		Panel: func(panel ui.Element) { styleMenuPanel(p, panel) },
		Item:  styleMenuItem,
	}
	if bottomEnd {
		opts.At = ui.AnchorBottomEnd
		opts.Self = ui.AnchorTopEnd
	}
	return opts
}

// releaseFocus hands the keyboard focus back to the bookmark area when a
// button keeps it after a mouse click, so arrow keys, Enter and Space keep
// driving the bookmark cursor instead of the button (a menu button reopens
// its menu on Up/Down). A focus reached by Tab stays put. For a menu
// button, open is its PopoverMenuButtonOptions.Open: the focus the menu
// gives back as it closes is released too, even after keyboard use inside
// the menu.
func (s *mainState) releaseFocus(c *ui.Context, b ui.Element, open *bool) {
	if !b.Valid() {
		return
	}
	pending := ui.Local(b, "releaseFocusPending", func() bool { return false })
	if open != nil {
		was := ui.Local(b, "releaseFocusWasOpen", func() bool { return false })
		if *was && !*open {
			*pending = true
		}
		*was = *open
		if *open {
			return
		}
	}
	switch {
	case b.Focused() && (*pending || !b.FocusVisible()):
		*pending = false
		s.focusHome(c)
	case *pending && s.inputFocused(c):
		*pending = false // a menu item moved the focus into a text field
	}
}

func softAccentBg(p *theme.Palette) ui.Color {
	if p.Dark {
		return p.AccentBg.Mix(p.Surface, 0.35)
	}
	return p.AccentBg.Mix(p.Surface, 0.65)
}

func (s *mainState) buildBookmarkMenu(c *ui.Context, m *ui.Menu, b *model.Bookmark) {
	a := s.app
	icon := theme.MenuIcon

	if !s.searchOpen {
		if ids := s.actionBookmarkIDs(b.ID); len(ids) > 1 {
			n := len(ids)
			if m.Item(locales.ActionCopyURLs(n)).Icon(icon(theme.IconLink)).Shortcut(ui.Cmd, ui.KeyC).Chosen() {
				s.copyBookmarkURLs(c, ids)
			}
			if m.Item(locales.ActionCopyTitles(n)).Icon(icon(theme.IconCopy)).Shortcut(ui.Cmd|ui.Shift, ui.KeyC).Chosen() {
				s.copyBookmarkTitles(c, ids)
			}

			m.Separator()

			if m.Item(locales.ActionOpenCount(n)).Icon(icon(theme.IconExternal)).Shortcut(0, ui.KeyEnter).Chosen() {
				s.openBookmarks(c, ids)
			}
			if m.Item(locales.ActionMoveCount(n)).Icon(icon(theme.IconMove)).Shortcut(ui.Cmd|ui.Shift, ui.KeyM).Chosen() {
				s.openMoveModal(ids)
			}

			m.Separator()

			if m.Item(locales.ActionDeleteCount(n)).Icon(icon(theme.IconTrash)).IconColor(s.pal.Danger).Danger().Shortcut(0, ui.KeyBackspace).Chosen() {
				s.clearSelection()
				a.Delete(ids...)
			}
			return
		}
	}

	if m.Item(locales.ActionOpen()).Icon(icon(theme.IconExternal)).Shortcut(0, ui.KeyEnter).Chosen() {
		if s.searchOpen {
			s.activateSearchBookmark(c, b)
		} else {
			s.activateBookmark(c, b)
		}
	}

	if m.Item(locales.ActionCopyURL()).Icon(icon(theme.IconLink)).Shortcut(ui.Cmd, ui.KeyC).Chosen() {
		s.copyBookmarkURL(c, b.ID)
	}
	if m.Item(locales.ActionCopyTitle()).Icon(icon(theme.IconCopy)).Shortcut(ui.Cmd|ui.Shift, ui.KeyC).Chosen() {
		s.copyBookmarkTitle(c, b.ID)
	}

	m.Separator()

	if m.Item(locales.ActionEdit()).Icon(icon(theme.IconEdit)).Shortcut(ui.Cmd, ui.KeyE).Chosen() {
		if a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{ID: b.ID})
		}
	}
	if m.Item(locales.ActionMoveTo()).Icon(icon(theme.IconMove)).Shortcut(ui.Cmd|ui.Shift, ui.KeyM).Chosen() {
		s.openMoveModal([]string{b.ID})
	}
	busy := a.IsEnriching(b.ID)
	if a.AI != nil && a.AI.Status().Ready {
		if m.Item(locales.ActionAIRePolish()).ID("re-polish").Icon(icon(theme.IconSparkle)).Disabled(busy).Chosen() {
			a.Polish(b.ID, true)
		}
	}
	if a.Fetch != nil {
		if m.Item(locales.ActionRefreshIcon()).ID("refresh-icon").Icon(icon(theme.IconRefresh)).Disabled(busy).Chosen() {
			a.RefreshBookmarkIcon(b.ID)
		}
	}

	m.Separator()

	deleteItem := m.Item(locales.ActionDelete()).Icon(icon(theme.IconTrash)).IconColor(s.pal.Danger).Danger()
	if !s.searchOpen {
		deleteItem.Shortcut(0, ui.KeyBackspace)
	}
	if deleteItem.Chosen() {
		a.Delete(b.ID)
	}
}

// buildCardSizeMenu adds the "Card Size" submenu (compact / standard /
// detailed) for the grid view.
func (s *mainState) buildCardSizeMenu(m *ui.Menu) {
	m.Submenu(locales.MenuCardSize(), func(sm *ui.Menu) {
		s.buildCardSizeItems(sm)
	}).Icon(theme.MenuIcon(theme.IconCardStandard))
}

// buildCardSizeItems adds the three card-size radio items to m.
func (s *mainState) buildCardSizeItems(sm *ui.Menu) {
	a := s.app
	cur := model.NormalizeCardSize(a.Settings.CardSize)
	{
		opts := []struct {
			id, label string
			icon      *ui.SVG
		}{
			{model.CardCompact, locales.CardSizeCompact(), theme.IconCardCompact},
			{model.CardStandard, locales.CardSizeStandard(), theme.IconCardStandard},
			{model.CardDetail, locales.CardSizeDetail(), theme.IconCardDetail},
		}
		for _, o := range opts {
			if sm.Item(o.label).Icon(theme.MenuIcon(o.icon)).Radio(cur == o.id).Chosen() {
				a.SetCardSize(o.id)
			}
		}
	}
}

func (s *mainState) buildMoveModal(c *ui.Context, p *theme.Palette) {
	if !s.moveOpen {
		return
	}
	a := s.app
	choices := a.Choices()
	q := strings.ToLower(strings.TrimSpace(s.moveFilter))
	var filtered []app.GroupChoice
	for _, ch := range choices {
		if q == "" || strings.Contains(strings.ToLower(ch.Path), q) {
			filtered = append(filtered, ch)
		}
	}

	rawFilter := strings.TrimSpace(s.moveFilter)
	canCreate := rawFilter != "" && a.Group != ""
	if canCreate {
		if g := a.Store.Lib().Group(a.Group); g != nil {
			for _, sub := range g.Subs {
				if strings.EqualFold(sub.Name, rawFilter) {
					canCreate = false
					break
				}
			}
		} else {
			canCreate = false
		}
	}

	total := len(filtered)
	if canCreate {
		total++
	}
	if total == 0 {
		s.moveIdx = 0
	} else {
		s.moveIdx = max(0, min(s.moveIdx, total-1))
	}

	ui.DialogBase(c, &s.moveOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(p.Overlay)
		panel.Width(320).MaxWidthPercent(90).Padding(8).Radius(10).
			Background(p.Surface).Border(1, p.Line).Gap(6).
			Shadow(0, 12, 32, -10, ui.RGBA(25, 15, 9, 0.22))

		panel.Children(func() {
			searchBox := ui.Row(c).Height(30).PaddingX(8).Radius(7).Gap(6).AlignItems(ui.Center).
				Background(p.Field)
			var in ui.Element
			searchBox.Children(func() {
				ui.Icon(c, theme.IconSearch).FontSize(13).TextColor(p.Muted).Shrink(0)
				in = ui.TextInputBase(c, &s.moveFilter).Bind(&s.moveFilterHandle).
					Placeholder(locales.MainMoveFilterPlaceholder()).Label(locales.MainMoveFilterLabel()).Grow(1).MinWidth(0).TextColor(p.Text)
			})
			reveal := false
			if in.Changed() {
				s.moveIdx = 0
				reveal = true
			}
			if in.Shortcut(0, ui.KeyDown) && total > 0 {
				s.moveIdx = min(s.moveIdx+1, total-1)
				reveal = true
			}
			if in.Shortcut(0, ui.KeyUp) && total > 0 {
				s.moveIdx = max(s.moveIdx-1, 0)
				reveal = true
			}
			if in.Submitted() && total > 0 {
				s.commitMoveChoice(filtered, canCreate, rawFilter, s.moveIdx)
			}

			list := widget.AutoScrollbar(c, ui.Scroll(c)).MaxHeight(260)
			list.Children(func() {
				ui.Column(c).Gap(1).Children(func() {
					for i, ch := range filtered {
						on := i == s.moveIdx
						row := ui.Row(c.Key("mv-" + ch.SubID)).Height(29).PaddingX(10).Radius(6).
							AlignItems(ui.Center).Justify(ui.SpaceBetween).Cursor(ui.CursorPointer)
						if on {
							row.Background(p.RowSel)
							if reveal {
								row.ScrollIntoView()
							}
						} else if row.Hovered() {
							row.Background(p.Hover)
						}
						row.Children(func() {
							ui.Text(c, ch.Path).TextColor(p.Text).SingleLine().Grow(1).MinWidth(0)
							if on {
								ui.Text(c, "↵").FontSize(12).TextColor(p.Muted).Shrink(0)
							}
						})
						if row.Clicked() {
							s.commitMoveChoice(filtered, canCreate, rawFilter, i)
						}
					}

					if canCreate {
						idx := len(filtered)
						on := idx == s.moveIdx
						row := ui.Row(c.Key("mv-create")).Height(29).PaddingX(10).Radius(6).
							AlignItems(ui.Center).Justify(ui.SpaceBetween).Cursor(ui.CursorPointer)
						if on {
							row.Background(p.RowSel)
							if reveal {
								row.ScrollIntoView()
							}
						} else if row.Hovered() {
							row.Background(p.Hover)
						}
						row.Children(func() {
							ui.Text(c, locales.MainMoveCreateSub(rawFilter)).TextColor(p.AccentText).SingleLine().Grow(1).MinWidth(0)
							if on {
								ui.Text(c, "↵").FontSize(12).TextColor(p.Muted).Shrink(0)
							}
						})
						if row.Clicked() {
							s.commitMoveChoice(filtered, canCreate, rawFilter, idx)
						}
					}

					if total == 0 {
						ui.Text(c, locales.MainMoveNoMatches()).FontSize(12).TextColor(p.Muted).Padding(10, 10).Selectable()
					}
				})
			})
		})
	})
}

func (s *mainState) commitMoveChoice(filtered []app.GroupChoice, canCreate bool, rawFilter string, idx int) {
	a := s.app
	if idx >= 0 && idx < len(filtered) {
		ch := filtered[idx]
		a.Move(s.moveIDs, ch.GroupID, ch.SubID)
		s.moveOpen = false
		return
	}
	if canCreate && idx == len(filtered) {
		subID := a.AddSub(a.Group, rawFilter)
		a.Move(s.moveIDs, a.Group, subID)
		s.moveOpen = false
	}
}

func (s *mainState) openMoveModal(ids []string) {
	if len(ids) == 0 {
		return
	}
	s.moveIDs = slices.Clone(ids)
	s.moveFilter = ""
	s.moveIdx = 0
	s.moveOpen = true
	s.moveFilterHandle.Focus()
}

func (s *mainState) activateBookmark(c *ui.Context, b *model.Bookmark) {
	if b == nil {
		return
	}
	if b.IsTemplate() {
		if s.tplBookmarkID == b.ID {
			s.tplBookmarkID = ""
			s.tplQuery = ""
		} else {
			s.tplBookmarkID = b.ID
			s.tplQuery = ""
			s.tplInputHandle.Focus(c)
		}
		return
	}
	s.app.Open(b, "")
}

func (s *mainState) activateSearchIndex(c *ui.Context, hits []app.Hit, isURL bool, idx int) {
	a := s.app
	if isURL && idx == 0 {
		if a.Win.ShowEditor != nil {
			fullURL := ensureURLScheme(strings.TrimSpace(s.query))
			s.closeSearch(c)
			if existing := a.Store.Lib().FindURL(fullURL); existing != nil {
				a.Win.ShowEditor(app.EditorRequest{ID: existing.ID})
			} else {
				a.Win.ShowEditor(app.EditorRequest{URL: fullURL, GroupID: a.Group, SubID: a.Sub})
			}
		}
		return
	}
	hitIdx := idx
	if isURL {
		hitIdx--
	}
	if hitIdx >= 0 && hitIdx < len(hits) {
		s.activateSearchBookmark(c, hits[hitIdx].B)
	}
}

func (s *mainState) copyBookmarkURL(c *ui.Context, id string) {
	if b := s.app.Store.Lib().Bookmark(id); b != nil && b.URL != "" {
		s.copyText(c, b.URL, locales.ToastCopiedURL())
	}
}

func (s *mainState) copyBookmarkURLs(c *ui.Context, ids []string) {
	lib := s.app.Store.Lib()
	urls := make([]string, 0, len(ids))
	for _, id := range ids {
		if b := lib.Bookmark(id); b != nil && b.URL != "" {
			urls = append(urls, b.URL)
		}
	}
	if len(urls) == 0 {
		return
	}
	if len(urls) == 1 {
		s.copyText(c, urls[0], locales.ToastCopiedURL())
		return
	}
	s.copyText(c, strings.Join(urls, "\n"), locales.ToastCopiedURLs(len(urls)))
}

func (s *mainState) copyBookmarkTitle(c *ui.Context, id string) {
	if b := s.app.Store.Lib().Bookmark(id); b != nil && b.Title != "" {
		s.copyText(c, b.Title, locales.ToastCopiedTitle())
	}
}

func (s *mainState) copyBookmarkTitles(c *ui.Context, ids []string) {
	lib := s.app.Store.Lib()
	titles := make([]string, 0, len(ids))
	for _, id := range ids {
		if b := lib.Bookmark(id); b != nil && b.Title != "" {
			titles = append(titles, b.Title)
		}
	}
	if len(titles) == 0 {
		return
	}
	if len(titles) == 1 {
		s.copyText(c, titles[0], locales.ToastCopiedTitle())
		return
	}
	s.copyText(c, strings.Join(titles, "\n"), locales.ToastCopiedTitles(len(titles)))
}

func formatBookmarkMarkdown(b *model.Bookmark) string {
	url := strings.TrimSpace(b.URL)
	title := strings.TrimSpace(b.Title)
	if title == "" {
		title = url
	}
	line := "- [" + title + "](" + url + ")"
	if note := strings.Join(strings.Fields(b.Note), " "); note != "" {
		line += " — " + note
	}
	return line
}

func (s *mainState) copyGroupMarkdown(c *ui.Context, groupID string) {
	lib := s.app.Store.Lib()
	g := lib.Group(groupID)
	if g == nil {
		return
	}
	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(g.Name)
	total := 0
	for _, sub := range g.Subs {
		bms := lib.In(g.ID, sub.ID)
		if len(bms) == 0 {
			continue
		}
		sb.WriteString("\n\n## ")
		sb.WriteString(sub.Name)
		for _, b := range bms {
			sb.WriteByte('\n')
			sb.WriteString(formatBookmarkMarkdown(b))
			total++
		}
	}
	if total == 0 {
		return
	}
	s.copyText(c, sb.String(), locales.ToastCopiedMarkdownCount(total))
}

func (s *mainState) copySubMarkdown(c *ui.Context, groupID, subID string) {
	lib := s.app.Store.Lib()
	g := lib.Group(groupID)
	sub := lib.Sub(groupID, subID)
	if g == nil || sub == nil {
		return
	}
	bms := lib.In(g.ID, sub.ID)
	if len(bms) == 0 {
		return
	}
	var sb strings.Builder
	sb.WriteString("## ")
	sb.WriteString(g.Name)
	sb.WriteString(" / ")
	sb.WriteString(sub.Name)
	for _, b := range bms {
		sb.WriteByte('\n')
		sb.WriteString(formatBookmarkMarkdown(b))
	}
	s.copyText(c, sb.String(), locales.ToastCopiedMarkdownCount(len(bms)))
}

func (s *mainState) openBookmarks(c *ui.Context, ids []string) {
	lib := s.app.Store.Lib()
	for _, id := range ids {
		b := lib.Bookmark(id)
		if b == nil || b.IsTemplate() {
			continue
		}
		s.app.Open(b, "")
	}
}

func (s *mainState) copyText(c *ui.Context, value, success string) {
	a := s.app
	c.WriteClipboardThen(value, func(err error) {
		if err != nil {
			a.SayErr(locales.ToastCopyFailed(err.Error()), "", nil)
			return
		}
		a.Say(success)
	})
}

func (s *mainState) dropBookmarksAt(drop dragPayload, groupID, subID, beforeID string) {
	a := s.app
	if len(drop.IDs) == 0 {
		return
	}
	if drop.SourceGroup != groupID || drop.SourceSub != subID {
		a.Move(drop.IDs, groupID, subID)
		if beforeID == "" {
			return
		}
	}
	a.Reorder(drop.IDs, groupID, subID, beforeID)
}

func (s *mainState) visibleBookmarkIDs() []string {
	lib := s.app.Store.Lib()
	g := lib.Group(s.app.Group)
	if g == nil {
		return nil
	}
	_, ids := s.ensureGroupCache(lib, g)
	return ids
}

func looksLikeURL(q string) bool {
	return widget.LooksLikeURL(q)
}

func ensureURLScheme(q string) string {
	return widget.EnsureURLScheme(q)
}

func shortDomain(raw string) string {
	return widget.ShortDomain(raw)
}
