package mainwin

import (
	"context"
	"slices"
	"strings"
	"time"

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

func softAccentBg(p *theme.Palette) ui.Color {
	if p.Dark {
		return p.AccentBg.Mix(p.Surface, 0.35)
	}
	return p.AccentBg.Mix(p.Surface, 0.65)
}

func (s *mainState) buildBookmarkMenu(c *ui.Context, m *ui.Menu, b *model.Bookmark) {
	a := s.app
	icon := theme.MenuIcon

	if m.Item(locales.ActionOpen()).Icon(icon(theme.IconExternal)).Shortcut(0, ui.KeyEnter).Chosen() {
		s.activateBookmark(c, b)
	}

	if m.Item(locales.ActionCopyURL()).Icon(icon(theme.IconLink)).Shortcut(ui.Cmd, ui.KeyC).Chosen() {
		s.copyBookmarkURL(c, b.ID)
	}
	if m.Item(locales.ActionCopyTitle()).Icon(icon(theme.IconCopy)).Shortcut(ui.Cmd|ui.Shift, ui.KeyC).Chosen() {
		s.copyBookmarkTitle(c, b.ID)
	}
	if m.Item(locales.ActionCopyMarkdown()).Icon(icon(theme.IconMarkdown)).Chosen() {
		s.copyText(c, "["+b.Title+"]("+b.URL+")", locales.ToastCopiedMarkdown())
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
	aiReady := a.AI != nil && a.AI.Status().Ready
	if m.Item(locales.ActionAIRePolish()).Icon(icon(theme.IconSparkle)).Disabled(!aiReady).Chosen() {
		a.Polish(b.ID, true)
	}
	if m.Item(locales.ActionRefreshIcon()).Icon(icon(theme.IconRefresh)).Disabled(a.Fetch == nil).Chosen() {
		s.refreshBookmarkIcon(b.ID)
	}

	if a.View == "grid" {
		m.Separator()
		s.buildCardSizeMenu(m)
	}

	m.Separator()

	if m.Item(locales.ActionDelete()).Icon(icon(theme.IconTrash)).IconColor(s.pal.Danger).Danger().Shortcut(0, ui.KeyBackspace).Chosen() {
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

func (s *mainState) copyBookmarkTitle(c *ui.Context, id string) {
	if b := s.app.Store.Lib().Bookmark(id); b != nil && b.Title != "" {
		s.copyText(c, b.Title, locales.ToastCopiedTitle())
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

func (s *mainState) refreshBookmarkIcon(id string) {
	a := s.app
	if a.Fetch == nil {
		return
	}
	b := a.Store.Lib().Bookmark(id)
	if b == nil {
		return
	}
	targetURL := b.URL
	libraryDir := a.Store.Dir()
	app.Background(20*time.Second, func(ctx context.Context) {
		meta, err := a.Fetch.Fetch(ctx, targetURL)
		a.Post(func() {
			if current := a.Store.Lib().Bookmark(id); current == nil || current.URL != targetURL || a.Store.Dir() != libraryDir {
				return
			}
			if err != nil || len(meta.Icon) == 0 {
				a.Say(locales.ToastNoIconFetched())
				return
			}
			imgPath, err := a.Store.SaveIcon(meta.Icon, meta.IconExt)
			if err != nil {
				a.Say(locales.ToastSaveIconFailed())
				return
			}
			a.Store.Update(func(l *model.Library) {
				if cur := l.Bookmark(id); cur != nil {
					cur.Icon.Image = imgPath
					cur.UpdatedAt = time.Now()
				}
			})
			a.Say(locales.ToastIconUpdated())
		})
	})
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
