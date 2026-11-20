package mainwin

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

func (s *mainState) buildSearchModal(c *ui.Context, p *theme.Palette) {
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

	wasOpen := s.searchOpen
	ui.DialogBase(c.Key("search-modal"), &s.searchOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(p.Overlay).AlignItems(ui.Center).Justify(ui.Start).Padding(52, 24, 24, 24)
		panel.Width(580).MaxWidthPercent(94).Radius(14).
			Background(p.Surface).Border(1, p.Line).Clip().
			Shadow(0, 22, 48, -12, ui.RGBA(25, 15, 9, 0.28))

		panel.Children(func() {
			// Top search input row.
			var searchIn ui.Element
			ui.Row(c.Key("sm-hdr")).Height(50).Padding(0, 12, 0, 16).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, theme.IconSearch).FontSize(16).TextColor(p.Accent).Shrink(0)
				searchIn = ui.TextInputBase(c.Key("sm-q"), &s.query).Bind(&s.searchHandle).
					Placeholder("搜标题、拼音、网址，或粘贴网址保存…").Label("搜索书签").
					FontSize(15).Grow(1).MinWidth(0).TextColor(p.Text).PlaceholderColor(p.Muted)

				if s.query != "" {
					clr := ui.ButtonBase(c.Key("sm-clear")).Label("清空搜索").Tooltip("清空搜索").
						Size(24, 24).Radius(6).Center().TextColor(p.Muted).Shrink(0)
					if clr.Hovered() {
						clr.Background(p.RowSel).TextColor(p.Text)
					}
					clr.Children(func() { ui.Icon(c, theme.IconClose).FontSize(13) })
					if clr.Clicked() {
						s.query = ""
						s.searchIdx = 0
						s.tplBookmarkID = ""
						s.searchList.ScrollTo(0, ui.Start)
						s.searchHandle.Focus(c)
					}
				}

				escBtn := ui.ButtonBase(c.Key("sm-esc")).Label("关闭搜索").Tooltip("返回主界面 (Esc)").
					Height(24).PaddingX(8).Radius(6).AlignItems(ui.Center).
					Background(p.Field).TextColor(p.Muted).Shrink(0)
				if escBtn.Hovered() {
					escBtn.Background(p.RowSel).TextColor(p.Text)
				}
				escBtn.Children(func() { ui.Text(c, "Esc").FontSize(11.5).SingleLine() })
				if escBtn.Clicked() {
					s.closeSearch(c)
				}
			})
			ui.Box(c).Height(1).FillWidth().Background(p.Line).Shrink(0)

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
			}

			// Scope chips strip.
			if len(lib.Groups) > 0 {
				widget.AutoScrollbarSlim(c, ui.ScrollHorizontal(c.Key("sm-scopes"))).
					ScrollbarInsets(0, 14, 0, 14).FillWidth().Height(36).Shrink(0).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Padding(8, 14, 3, 14).Children(func() {
						s.buildScopeChip(c.Key("sm-scope-all"), p, "", "全部")
						for _, g := range lib.Groups {
							s.buildScopeChip(c.Key("sm-scope-"+g.ID), p, g.ID, g.Name)
						}
					})
				})
			}

			// Results area.
			body := widget.AutoScrollbar(c, ui.Scroll(c.Key("sm-scroll"))).MaxHeight(340).FillWidth()
			body.Children(func() {
				ui.Column(c).FillWidth().Padding(6, 10, 10, 10).Gap(2).Children(func() {
					if isURL {
						s.buildPasteURLCard(c, p, lib, q, s.searchIdx == 0)
					}

					if q == "" && len(hits) > 0 {
						ui.Text(c, "最近更新").FontSize(11.5).TextColor(p.Muted).Padding(2, 10, 4, 10)
					}

					if !isURL && len(hits) == 0 {
						ui.Column(c).FillWidth().Padding(28, 16).Center().Gap(12).Children(func() {
							msg := "还没有书签"
							if q != "" {
								msg = "没有找到「" + q + "」"
							}
							ui.Text(c, msg).FontSize(13.5).TextColor(p.Muted).SingleLine()
							ui.Row(c).Wrap().Center().Gap(8).Children(func() {
								if s.searchScope != "" {
									allBtn := widget.Secondary(c, p, "在全部分组中搜索")
									if allBtn.Clicked() {
										s.searchScope = ""
										s.searchIdx = 0
										s.searchHandle.Focus(c)
									}
								}
								if q != "" && a.Win.ShowEditor != nil {
									newBtn := widget.Secondary(c, p, "新建书签")
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

			// Footer status bar.
			ui.Box(c).Height(1).FillWidth().Background(p.Line).Shrink(0)
			ui.Row(c.Key("sm-foot")).Height(34).Padding(0, 14).Background(p.Bg).
				AlignItems(ui.Center).Justify(ui.SpaceBetween).Shrink(0).Children(func() {
				status := "直接输入关键词、拼音或网址"
				switch {
				case isURL:
					status = "识别为网址 · 按回车保存"
				case q != "":
					status = fmt.Sprintf("%d 个匹配结果", len(hits))
				case len(hits) > 0:
					status = fmt.Sprintf("最近 %d 条书签", len(hits))
				}
				ui.Text(c, status).FontSize(11.5).TextColor(p.Muted).SingleLine()
				ui.Text(c, "↑↓ 选择 · ↵ 打开 · Esc 返回").FontSize(11.5).TextColor(p.Muted).SingleLine()
			})
		})
	})

	if wasOpen && !s.searchOpen {
		s.clearSearch()
		s.contentHandle.Focus(c)
	}
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
	menuOpen := ui.Local(row, "smrMenuOpen", func() bool { return false })
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

	var mainArea ui.Element
	row.Children(func() {
		mainArea = ui.Row(c).Grow(1).MinWidth(0).Gap(11).AlignItems(ui.Center)
		mainArea.Children(func() {
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
		})

		if hovered || *menuOpen {
			ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
				s.buildBookmarkHoverActions(c, p, b, menuOpen, 26, 13.5)
			})
		} else if on {
			ui.Row(c).Padding(2, 6).Radius(6).Background(p.Surface).Border(1, p.Line).Shrink(0).Children(func() {
				ui.Text(c, "↵").FontSize(11.5).TextColor(p.Muted).SingleLine()
			})
		}
	})

	if mainArea.Clicked() {
		s.searchIdx = idx
		s.activateSearchBookmark(c, b)
	}
	row.PopoverMenu(func(m *ui.Menu) {
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
