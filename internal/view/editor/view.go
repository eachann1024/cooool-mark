package editor

import (
	"fmt"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

func (s *state) view(c *ui.Context) {
	p := theme.Apply(c, s.a.Settings.Appearance)
	s.ensureValidPlace()

	c.OnWindowShortcut(ui.Cmd, ui.KeyEnter, s.save)
	c.OnWindowShortcut(0, ui.KeyEscape, func() {
		if s.addingSub {
			s.addingSub = false
			s.newSubName = ""
			s.newSubFocused = false
			return
		}
		s.close()
	})

	if s.needFocus != nil {
		h := s.needFocus
		s.needFocus = nil
		h.Focus(c)
	}

	root := ui.Column(c).Fill().Background(p.Bg)
	root.Children(func() {
		s.headerBar(c, p)
		ui.Box(c).Height(1).FillWidth().Background(p.Line)

		widget.AutoScrollbar(c, ui.Scroll(c)).Grow(1).FillWidth().Padding(16, 18, 20, 18).Gap(14).Children(func() {
			s.urlSection(c, p)
			s.cardSection(c, p)
			s.groupSection(c, p)
		})

		ui.Box(c).Height(1).FillWidth().Background(p.Line)
		s.footerBar(c, p)
	})
}

func (s *state) headerBar(c *ui.Context, p *theme.Palette) {
	bar := c.TitleBar()
	h := widget.BarH(bar)
	padLeft := max(bar.Left+12, 18)
	padRight := max(bar.Right+12, 14)

	ui.Row(c).Height(h).FillWidth().Shrink(0).AlignItems(ui.Center).
		Padding(0, padRight, 0, padLeft).Gap(8).DragWindow().Children(func() {
		widget.Title(c, p, s.windowTitle(), 15).FontWeight(600)
	})
}

func (s *state) urlSection(c *ui.Context, p *theme.Palette) {
	ui.Column(c).FillWidth().Gap(7).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			borderCol := p.Line.Mix(p.Text, 0.12)
			if s.urlHandle.Focused(c) {
				borderCol = p.Accent
			}
			box := ui.Row(c).Grow(1).MinWidth(0).Height(36).Padding(0, 8, 0, 10).
				Gap(7).AlignItems(ui.Center).Radius(9).Background(p.Surface).Border(1, borderCol)
			box.Children(func() {
				ui.Icon(c, theme.IconLink).FontSize(14).TextColor(p.Muted)

				urlIn := ui.TextInputBase(c.Key(fmt.Sprintf("url-in-%d", s.sessionSeq)), &s.url).
					Bind(&s.urlHandle).
					Grow(1).MinWidth(0).
					Placeholder("粘贴网址或含 {query} 的搜索模板").
					Label("网址").
					TextColor(p.Text).FontSize(13.5)
				urlIn.OnPaste(func(string) bool {
					s.justPasted = true
					return false
				})
				if urlIn.Changed() {
					s.scheduleDebouncedFetch()
				}
				if urlIn.Submitted() {
					s.startFetch(s.url, true, true, false)
				}

				if strings.Contains(s.url, "{query}") {
					ui.Row(c).Height(20).PaddingX(6).Radius(5).AlignItems(ui.Center).
						Background(p.AccentBg).Children(func() {
						ui.Text(c, "{query}").FontSize(11).FontWeight(500).TextColor(p.AccentText).SingleLine()
					})
				}

				canFetch := strings.TrimSpace(s.url) != "" && s.a.Fetch != nil && !s.fetching
				refBtn := ui.ButtonBase(c.Key("refetch")).Size(24, 24).Radius(6).Center().
					TextColor(p.Muted).Label("重新读取网页").Tooltip("重新读取网页").
					Busy(s.fetching && !s.fetchingIcon).Disabled(!canFetch)
				if canFetch && refBtn.Hovered() {
					refBtn.Background(p.Hover).TextColor(p.Text)
				}
				refBtn.Children(func() {
					ui.Icon(c, theme.IconRefresh).FontSize(13)
				})
				if canFetch && refBtn.Clicked() {
					s.startFetch(s.url, true, true, false)
				}
			})

			saveKey := shortcutSaveKey()
			saveBtn := ui.ButtonBase(c.Key("top-save")).Height(36).PaddingX(13).Radius(9).
				AlignItems(ui.Center).Gap(6).Label("保存书签").Tooltip("保存 (" + saveKey + ")")
			canSave := strings.TrimSpace(s.url) != ""
			keyColor := p.Muted.Alpha(0.8)
			if canSave {
				saveBtn.Background(p.Primary).TextColor(p.OnPrimary)
				keyColor = p.OnPrimary.Alpha(0.82)
				if saveBtn.Hovered() {
					saveBtn.Background(p.Primary.Mix(ui.Hex("#000000"), 0.08))
				}
			} else {
				saveBtn.Background(p.Field).TextColor(p.Muted)
			}
			saveBtn.Children(func() {
				ui.Text(c, "保存").FontWeight(500).SingleLine()
				ui.Text(c, saveKey).FontSize(11).TextColor(keyColor).SingleLine()
			})
			if !canSave {
				saveBtn.Disabled(true)
			} else if saveBtn.Clicked() {
				s.save()
			}
		})

		if s.saveErr != "" {
			ui.Text(c, s.saveErr).FontSize(12).TextColor(p.Danger).Wrap()
		}
		switch {
		case s.fetching && s.fetchingIcon:
			ui.Text(c, "正在抓取图标…").FontSize(12).TextColor(p.Muted).SingleLine()
		case s.fetching:
			ui.Text(c, "正在读取网页…").FontSize(12).TextColor(p.Muted).SingleLine()
		case s.fetchErr != "":
			ui.Text(c, s.fetchErr).FontSize(12).TextColor(p.Danger).Wrap()
		case s.aiLoading:
			ui.Text(c, "正在后台润色…").FontSize(12).TextColor(p.Muted).SingleLine()
		case s.aiErr != "":
			ui.Text(c, s.aiErr).FontSize(12).TextColor(p.Danger).Wrap()
		}

		if u := strings.TrimSpace(s.url); u != "" && model.NormalizeURL(u) != "" {
			if dup := s.a.Store.Lib().FindURL(u); dup != nil && dup.ID != s.id {
				ui.Row(c).FillWidth().Padding(8, 10).Radius(9).AlignItems(ui.Center).Gap(8).
					Background(p.AccentBg.Mix(p.Surface, 0.4)).Border(1, p.Accent.Alpha(0.35)).Children(func() {
					ui.Text(c, "此网址已保存在「"+s.a.PathOf(dup)+"」").
						FontSize(12).TextColor(p.AccentText).Grow(1).MinWidth(0)
					jump := ui.ButtonBase(c.Key("jump-dup")).Height(24).PaddingX(8).Radius(6).
						AlignItems(ui.Center).Background(p.Surface).TextColor(p.AccentText).
						Label("编辑已有书签").Tooltip("打开并编辑已保存的书签")
					if jump.Hovered() {
						jump.Background(p.Hover)
					}
					jump.Children(func() {
						ui.Text(c, "编辑已有").FontSize(12).FontWeight(500).SingleLine()
					})
					if jump.Clicked() {
						Show(s.a, app.EditorRequest{ID: dup.ID})
					}
				})
			}
		}
	})
}

func (s *state) cardSection(c *ui.Context, p *theme.Palette) {
	ui.Column(c).FillWidth().Padding(14).Gap(10).Radius(12).
		Background(p.Surface).Border(1, p.Line).Children(func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
				icon := s.previewIcon()
				theme.Tile(c, icon, s.a.Store.IconPath(icon), 48)

				canRefetchIcon := strings.TrimSpace(s.url) != "" && s.a.Fetch != nil && !s.fetching
				reText := "重新抓取图标"
				if s.fetchingIcon {
					reText = "抓取中…"
				}
				reBtn := ui.ButtonBase(c.Key("refetch-icon")).Height(22).PaddingX(7).Radius(6).
					AlignItems(ui.Center).Gap(3).Background(p.Field).TextColor(p.Text2).
					Label("重新抓取图标").Tooltip("重新从网页抓取站点图标").
					Busy(s.fetchingIcon).Disabled(!canRefetchIcon)
				if !canRefetchIcon {
					reBtn.TextColor(p.Muted)
				} else if reBtn.Hovered() {
					reBtn.Background(p.Line).TextColor(p.Text)
				}
				reBtn.Children(func() {
					ui.Text(c, reText).FontSize(11).SingleLine()
				})
				if canRefetchIcon && reBtn.Clicked() {
					s.startFetch(s.url, false, false, true)
				}
			})

			ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					ui.TextInputBase(c.Key(fmt.Sprintf("title-in-%d", s.sessionSeq)), &s.title).
						Bind(&s.titleHandle).
						Grow(1).MinWidth(0).Height(30).
						Placeholder("书签标题").
						Label("书签标题").
						FontSize(15).FontWeight(600).TextColor(p.Text)
					if s.aiReady() {
						aiLabel, aiName, aiTip := "润色", "AI 润色", "使用 AI 润色标题、简介和标签"
						if s.aiLoading {
							aiLabel, aiName, aiTip = "润色中…", "AI 润色中", "正在润色标题、简介和标签"
						}
						canAI := strings.TrimSpace(s.url) != "" && !s.aiLoading
						aiBtn := ui.ButtonBase(c.Key("ai-polish")).Height(26).PaddingX(8).Radius(6).
							AlignItems(ui.Center).Gap(4).Background(p.AccentBg).TextColor(p.AccentText).
							Label(aiName).Tooltip(aiTip).Busy(s.aiLoading).Disabled(!canAI)
						if canAI && aiBtn.Hovered() {
							aiBtn.Background(p.Accent.Alpha(0.22))
						}
						aiBtn.Children(func() {
							ui.Icon(c, theme.IconSparkle).FontSize(12)
							ui.Text(c, aiLabel).FontSize(12).SingleLine()
						})
						if canAI && aiBtn.Clicked() {
							s.startAI(true)
						}
					}
				})

				ui.TextAreaBase(c.Key(fmt.Sprintf("note-in-%d", s.sessionSeq)), &s.note).
					FillWidth().Height(52).
					Placeholder("简介或笔记").
					Label("简介或笔记").
					FontSize(12.5).TextColor(p.Text2)
			})
		})

		if s.hasAISuggestion() {
			s.aiSuggestionBox(c, p)
		}

		s.tagsRow(c, p)
	})
}

func (s *state) aiSuggestionBox(c *ui.Context, p *theme.Palette) {
	sug := s.aiSuggest
	if sug == nil {
		return
	}
	ui.Column(c).FillWidth().Padding(9, 10).Gap(6).Radius(9).
		Background(p.AccentBg.Mix(p.Surface, 0.45)).Border(1, p.Accent.Alpha(0.35)).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, theme.IconSparkle).FontSize(13).TextColor(p.AccentText)
			ui.Text(c, "润色建议").FontSize(12).FontWeight(600).TextColor(p.AccentText)
			ui.Box(c).Grow(1)

			applyBtn := ui.ButtonBase(c.Key("ai-apply")).Height(22).PaddingX(8).Radius(6).
				AlignItems(ui.Center).Background(p.Primary).TextColor(p.OnPrimary).
				Label("一键替换").Tooltip("用润色建议替换当前内容")
			if applyBtn.Hovered() {
				applyBtn.Background(p.Primary.Mix(ui.Hex("#000000"), 0.08))
			}
			applyBtn.Children(func() {
				ui.Text(c, "一键替换").FontSize(11.5).FontWeight(500).SingleLine()
			})
			if applyBtn.Clicked() {
				s.applyAI()
			}

			disBtn := ui.ButtonBase(c.Key("ai-dismiss")).Size(22, 22).Radius(6).
				Center().TextColor(p.AccentText).Label("忽略润色建议").Tooltip("忽略润色建议")
			if disBtn.Hovered() {
				disBtn.Background(p.Accent.Alpha(0.15))
			}
			disBtn.Children(func() {
				ui.Icon(c, theme.IconClose).FontSize(11)
			})
			if disBtn.Clicked() {
				s.aiDismissed = true
			}
		})

		if sug.Title != "" && sug.Title != strings.TrimSpace(s.title) {
			ui.Text(c, sug.Title).FontSize(12.5).FontWeight(500).TextColor(p.Text)
		}
		if sug.Note != "" && sug.Note != strings.TrimSpace(s.note) {
			ui.Text(c, sug.Note).FontSize(12).TextColor(p.Text2)
		}
	})
}

func (s *state) tagsRow(c *ui.Context, p *theme.Palette) {
	ui.Column(c).FillWidth().Padding(9, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(p.LineSoft).Children(func() {
		ui.Row(c).FillWidth().Wrap().Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, theme.IconTag).FontSize(12).TextColor(p.Muted)
			removeIdx := -1
			for i, tag := range s.tags {
				chip := ui.Row(c.Key(fmt.Sprintf("tag-%d-%s", i, tag))).Height(22).Padding(0, 3, 0, 7).Radius(6).
					Gap(2).AlignItems(ui.Center).Background(p.Field)
				chip.Children(func() {
					ui.Text(c, tag).FontSize(12).TextColor(p.Text).SingleLine()
					rm := ui.ButtonBase(c.Key(fmt.Sprintf("rm-tag-%d", i))).Size(16, 16).Radius(4).Center().
						TextColor(p.Muted).Label("移除标签 " + tag).Tooltip("移除标签")
					if rm.Hovered() {
						rm.Background(p.Line).TextColor(p.Text)
					}
					rm.Children(func() { ui.Icon(c, theme.IconClose).FontSize(9) })
					if rm.Clicked() {
						removeIdx = i
					}
				})
			}
			if removeIdx >= 0 {
				s.tags = slices.Delete(s.tags, removeIdx, removeIdx+1)
			}

			if s.aiReady() && s.aiSuggest != nil && !s.aiDismissed {
				for i, at := range s.aiSuggest.Tags {
					if slices.Contains(s.tags, at) {
						continue
					}
					addChip := ui.ButtonBase(c.Key(fmt.Sprintf("ai-tag-%d-%s", i, at))).Height(22).PaddingX(7).Radius(6).
						AlignItems(ui.Center).Background(p.AccentBg.Mix(p.Surface, 0.45)).
						Border(1, p.Accent.Alpha(0.4)).BorderStyle(ui.BorderDashed).TextColor(p.AccentText).
						Label("添加标签 " + at).Tooltip("点击添加推荐标签")
					if addChip.Hovered() {
						addChip.Background(p.AccentBg)
					}
					addChip.Children(func() {
						ui.Text(c, "+ "+at).FontSize(11.5).SingleLine()
					})
					if addChip.Clicked() {
						s.tags = append(s.tags, at)
					}
				}
			}

			tagIn := ui.TextInputBase(c.Key(fmt.Sprintf("tag-draft-%d", s.sessionSeq)), &s.tagDraft).
				Grow(1).MinWidth(72).Height(22).
				Placeholder("加标签…").Label("添加标签").FontSize(12).TextColor(p.Text)
			tagIn.OnTextKey(func(k ui.TextKey) bool {
				if k.Key == ui.KeyBackspace && k.Mods == 0 && k.Text() == "" && len(s.tags) > 0 {
					s.tags = s.tags[:len(s.tags)-1]
					return true
				}
				return false
			})
			if tagIn.Changed() && (strings.Contains(s.tagDraft, ",") || strings.Contains(s.tagDraft, "，")) {
				s.commitTagDraft()
			}
			if tagIn.Submitted() {
				s.commitTagDraft()
			}
		})
	})
}

func (s *state) groupSection(c *ui.Context, p *theme.Palette) {
	lib := s.a.Store.Lib()
	ui.Column(c).FillWidth().Padding(12, 14).Gap(10).Radius(12).
		Background(p.Surface).Border(1, p.Line).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Children(func() {
			ui.Text(c, "放到分组").FontSize(12).FontWeight(600).TextColor(p.Text2)

			if s.aiReady() && s.aiSuggest != nil && !s.aiDismissed &&
				s.aiSuggest.GroupID != "" && lib.Sub(s.aiSuggest.GroupID, s.aiSuggest.SubID) != nil &&
				(s.aiSuggest.GroupID != s.groupID || s.aiSuggest.SubID != s.subID) {
				recPath := s.a.PathOf(&model.Bookmark{GroupID: s.aiSuggest.GroupID, SubID: s.aiSuggest.SubID})
				recBtn := ui.ButtonBase(c.Key("ai-rec-group")).Height(22).PaddingX(8).Radius(999).
					AlignItems(ui.Center).Background(p.AccentBg).TextColor(p.AccentText).
					Label("使用推荐分组").Tooltip("切换到推荐分组")
				recBtn.Children(func() {
					ui.Text(c, "推荐："+recPath).FontSize(11.5).SingleLine()
				})
				if recBtn.Clicked() {
					s.groupID = s.aiSuggest.GroupID
					s.subID = s.aiSuggest.SubID
					s.addingSub = false
					s.newSubName = ""
					s.newSubFocused = false
				}
			}
		})

		ui.Row(c).FillWidth().Wrap().Gap(4).Padding(0, 0, 8, 0).
			BorderWidth(0, 0, 1, 0).BorderColor(p.LineSoft).Children(func() {
			for i := range lib.Groups {
				g := &lib.Groups[i]
				on := g.ID == s.groupID
				pill := widget.Pill(c.Key("grp-"+g.ID), p, g.Name, on).
					Label("分组 " + g.Name).Tooltip("选择分组 " + g.Name)
				if pill.Clicked() {
					s.groupID = g.ID
					s.subID = ""
					if len(g.Subs) > 0 {
						s.subID = g.Subs[0].ID
					}
					s.addingSub = false
					s.newSubName = ""
					s.newSubFocused = false
				}
			}
		})

		curG := lib.Group(s.groupID)
		ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
			if curG != nil {
				for i := range curG.Subs {
					sub := &curG.Subs[i]
					on := sub.ID == s.subID
					sb := ui.ButtonBase(c.Key("sub-" + sub.ID)).Height(28).PaddingX(11).Radius(8).
						AlignItems(ui.Center).Label("子分组 " + sub.Name).Tooltip("选择子分组 " + sub.Name)
					if on {
						sb.Background(p.AccentBg).Border(1, p.Accent).TextColor(p.AccentText)
					} else {
						sb.Background(p.Bg).Border(1, p.Line).TextColor(p.Text2)
						if sb.Hovered() {
							sb.Background(p.Hover)
						}
					}
					sb.Children(func() {
						t := ui.Text(c, sub.Name).FontSize(12.5).SingleLine()
						if on {
							t.FontWeight(600)
						}
					})
					if sb.Clicked() {
						s.subID = sub.ID
					}
				}
			}

			if curG != nil {
				if s.addingSub {
					subIn := ui.TextInput(c.Key(fmt.Sprintf("new-sub-in-%d", s.sessionSeq)), &s.newSubName).
						Bind(&s.newSubHandle).
						Width(132).Height(28).FontSize(12).
						Placeholder("子分组名，回车确认").Label("新建子分组名称")
					if subIn.Focused() {
						s.newSubFocused = true
					}
					switch {
					case subIn.Submitted():
						s.commitNewSub()
					case subIn.Shortcut(0, ui.KeyEscape):
						s.addingSub = false
						s.newSubName = ""
						s.newSubFocused = false
					case s.newSubFocused && !subIn.Focused():
						s.commitNewSub()
					}
				} else {
					addBtn := ui.ButtonBase(c.Key("add-sub-btn")).Height(28).PaddingX(10).Radius(8).
						AlignItems(ui.Center).Gap(4).Border(1, p.Line).BorderStyle(ui.BorderDashed).
						TextColor(p.Muted).Label("新建子分组").Tooltip("在当前分组下新建子分组")
					if addBtn.Hovered() {
						addBtn.Background(p.Hover).TextColor(p.Text)
					}
					addBtn.Children(func() {
						ui.Icon(c, theme.IconPlus).FontSize(11)
						ui.Text(c, "新建").FontSize(12).SingleLine()
					})
					if addBtn.Clicked() {
						s.addingSub = true
						s.newSubName = ""
						s.newSubFocused = false
						s.needFocus = &s.newSubHandle
					}
				}
			}
		})
	})
}

func (s *state) footerBar(c *ui.Context, p *theme.Palette) {
	saveKey := shortcutSaveKey()
	ui.Row(c).FillWidth().Shrink(0).Padding(11, 18).Gap(8).AlignItems(ui.Center).
		Background(p.Sidebar).Children(func() {
		if s.id != "" {
			del := widget.Danger(c.Key("delete-btn"), p, "删除", false).
				Label("删除").Tooltip("删除此书签")
			if del.Clicked() {
				s.deleteCurrent()
			}
		}

		ui.Box(c).Grow(1)

		cancelBtn := widget.Secondary(c.Key("cancel-btn"), p, "取消").
			Label("取消").Tooltip("取消 (Esc)")
		if cancelBtn.Clicked() {
			s.close()
		}

		saveBtn := widget.Primary(c.Key("save-btn"), p, "保存").
			Label("保存").Tooltip("保存 (" + saveKey + ")")
		if strings.TrimSpace(s.url) == "" {
			saveBtn.Disabled(true)
		} else if saveBtn.Clicked() {
			s.save()
		}
	})
}

func shortcutSaveKey() string {
	if runtime.GOOS == "darwin" {
		return "⌘↵"
	}
	return "Ctrl+↵"
}
