package editor

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

func (s *state) view(c *ui.Context) {
	p := theme.Apply(c, s.a.Settings.Appearance)
	s.ensureValidPlace()

	c.OnWindowShortcut(ui.Cmd, ui.KeyEnter, s.save)
	c.OnWindowShortcut(0, ui.KeyEscape, func() {
		if s.confirmSwitch {
			s.confirmSwitch = false
			s.pendingReqs = nil
			return
		}
		if s.confirmClose {
			s.confirmClose = false
			s.quitAfterClose = false
			return
		}
		if s.addingSub {
			s.addingSub = false
			s.newSubName = ""
			return
		}
		s.requestClose()
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
		widget.Toast(c, s.a, p, 64)
		s.unsavedSwitchModal(c, p)
		s.unsavedCloseModal(c, p)
	})
}

func (s *state) unsavedCloseModal(c *ui.Context, p *theme.Palette) {
	if !s.confirmClose {
		return
	}
	wasOpen := s.confirmClose
	chosen := -1
	winW, _ := c.Size()
	ui.DialogBase(c.Key("unsaved-close-modal"), &s.confirmClose, func(back, panel ui.Element) {
		back.Background(p.Overlay)
		panel.Width(404).MaxWidth(max(winW-32, 240)).Padding(20, 20, 16, 20).Gap(10).Radius(12).
			Background(p.Surface).Border(1, p.Line).
			Shadow(0, 24, 64, -16, ui.RGBA(0, 0, 0, 0.32))
		widget.Title(c, p, locales.EditorUnsavedTitle(), 17)
		ui.Text(c, locales.EditorUnsavedCloseMsg()).TextColor(p.Text2).Wrap().Selectable()
		ui.Row(c).Gap(8).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
			if widget.Secondary(c.Key("close-cancel"), p, locales.ActionCancel()).Clicked() {
				chosen = 0
			}
			if widget.Danger(c.Key("close-discard"), p, locales.EditorDiscardAndClose(), false).Clicked() {
				chosen = 1
			}
			if widget.Primary(c.Key("close-save"), p, locales.EditorSaveAndClose()).AutoFocus().Clicked() {
				chosen = 2
			}
		})
	})
	switch chosen {
	case 0:
		s.confirmClose = false
		s.quitAfterClose = false
	case 1:
		s.confirmClose = false
		s.finishClose()
	case 2:
		s.confirmClose = false
		if s.doSave() {
			s.pendingReqs = nil
			s.finishClose()
		} else {
			s.quitAfterClose = false
		}
	default:
		if wasOpen && !s.confirmClose {
			s.quitAfterClose = false
		}
	}
}

func (s *state) unsavedSwitchModal(c *ui.Context, p *theme.Palette) {
	if !s.confirmSwitch || len(s.pendingReqs) == 0 {
		s.confirmSwitch = false
		return
	}
	wasOpen := s.confirmSwitch
	chosen := -1
	winW, _ := c.Size()
	ui.DialogBase(c.Key("unsaved-switch-modal"), &s.confirmSwitch, func(back, panel ui.Element) {
		back.Background(p.Overlay)
		panel.Width(404).MaxWidth(max(winW-32, 240)).Padding(20, 20, 16, 20).Gap(10).Radius(12).
			Background(p.Surface).Border(1, p.Line).
			Shadow(0, 24, 64, -16, ui.RGBA(0, 0, 0, 0.32))
		widget.Title(c, p, locales.EditorUnsavedTitle(), 17)
		ui.Text(c, locales.EditorUnsavedMsg()).TextColor(p.Text2).Wrap().Selectable()
		ui.Row(c).Gap(8).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
			if widget.Secondary(c.Key("switch-cancel"), p, locales.ActionCancel()).Clicked() {
				chosen = 0
			}
			if widget.Danger(c.Key("switch-discard"), p, locales.EditorDiscardAndSwitch(), false).Clicked() {
				chosen = 1
			}
			if widget.Primary(c.Key("switch-save"), p, locales.EditorSaveAndSwitch()).AutoFocus().Clicked() {
				chosen = 2
			}
		})
	})
	switch chosen {
	case 0:
		s.confirmSwitch = false
		s.pendingReqs = nil
	case 1:
		s.advancePending()
	case 2:
		s.confirmSwitch = false
		if s.doSave() {
			s.advancePending()
		}
	default:
		if wasOpen && !s.confirmSwitch {
			s.pendingReqs = nil
		}
	}
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
					Placeholder(locales.EditorURLPlaceholder()).
					Label(locales.EditorURLLabel()).
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
					TextColor(p.Muted).Label(locales.EditorRefetchPage()).Tooltip(locales.EditorRefetchPage()).
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
				AlignItems(ui.Center).Gap(6).Label(locales.EditorSaveBookmark()).Tooltip(locales.EditorSaveTip(saveKey))
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
				ui.Text(c, locales.ActionSave()).FontWeight(500).SingleLine()
				ui.Text(c, saveKey).FontSize(11).TextColor(keyColor).SingleLine()
			})
			if !canSave {
				saveBtn.Disabled(true)
			} else if saveBtn.Clicked() {
				s.save()
			}
		})

		if s.saveErr != "" {
			ui.Text(c, s.saveErr).FontSize(12).TextColor(p.Danger).Wrap().Selectable()
		}
		switch {
		case s.fetching && s.fetchingIcon:
			ui.Text(c, locales.EditorFetchingIcon()).FontSize(12).TextColor(p.Muted).SingleLine()
		case s.fetching:
			ui.Text(c, locales.EditorFetchingPage()).FontSize(12).TextColor(p.Muted).SingleLine()
		case s.fetchErr != "":
			ui.Text(c, s.fetchErr).FontSize(12).TextColor(p.Danger).Wrap().Selectable()
		case s.aiLoading:
			ui.Text(c, locales.EditorAIPolishing()).FontSize(12).TextColor(p.Muted).SingleLine()
		case s.aiErr != "":
			ui.Text(c, s.aiErr).FontSize(12).TextColor(p.Danger).Wrap().Selectable()
		}

		if u := strings.TrimSpace(s.url); u != "" && model.NormalizeURL(u) != "" {
			if dup := s.a.Store.Lib().FindURL(u); dup != nil && dup.ID != s.id {
				ui.Row(c).FillWidth().Padding(8, 10).Radius(9).AlignItems(ui.Center).Gap(8).
					Background(p.AccentBg.Mix(p.Surface, 0.4)).Border(1, p.Accent.Alpha(0.35)).Children(func() {
					ui.Text(c, locales.EditorDuplicateBanner(s.a.PathOf(dup))).
						FontSize(12).TextColor(p.AccentText).Grow(1).MinWidth(0).Selectable()
					jump := ui.ButtonBase(c.Key("jump-dup")).Height(24).PaddingX(8).Radius(6).
						AlignItems(ui.Center).Background(p.Surface).TextColor(p.AccentText).
						Label(locales.ActionEditExisting()).Tooltip(locales.EditorEditExistingTip())
					if jump.Hovered() {
						jump.Background(p.Hover)
					}
					jump.Children(func() {
						ui.Text(c, locales.ActionEditExisting()).FontSize(12).FontWeight(500).SingleLine()
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
				reText := locales.EditorRefetchIcon()
				if s.fetchingIcon {
					reText = locales.EditorFetchingShort()
				}
				reBtn := ui.ButtonBase(c.Key("refetch-icon")).Height(22).PaddingX(7).Radius(6).
					AlignItems(ui.Center).Gap(3).Background(p.Field).TextColor(p.Text2).
					Label(locales.EditorRefetchIcon()).Tooltip(locales.EditorRefetchIconTip()).
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
					titleIn := ui.TextInputBase(c.Key(fmt.Sprintf("title-in-%d", s.sessionSeq)), &s.title).
						Bind(&s.titleHandle).
						Grow(1).MinWidth(0).Height(30).
						Placeholder(locales.EditorTitlePlaceholder()).
						Label(locales.EditorTitleLabel()).
						FontSize(15).FontWeight(600).TextColor(p.Text)
					if titleIn.Changed() {
						s.titleEdited = strings.TrimSpace(s.title) != ""
					}
					if s.aiReady() {
						aiLabel, aiName, aiTip := locales.EditorPolishShort(), locales.ActionAIPolish(), locales.EditorPolishTip()
						if s.aiLoading {
							aiLabel, aiName, aiTip = locales.EditorPolishingShort(), locales.EditorPolishingShort(), locales.EditorPolishingTip()
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

				noteIn := ui.TextAreaBase(c.Key(fmt.Sprintf("note-in-%d", s.sessionSeq)), &s.note).
					FillWidth().Height(52).
					Placeholder(locales.EditorNotePlaceholder()).
					Label(locales.EditorNoteLabel()).
					FontSize(12.5).TextColor(p.Text2)
				if noteIn.Changed() {
					s.noteEdited = strings.TrimSpace(s.note) != ""
				}
			})
		})

		if s.hasAISuggestion() {
			s.aiSuggestionBox(c, p)
		}
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
			ui.Text(c, locales.EditorAISuggestion()).FontSize(12).FontWeight(600).TextColor(p.AccentText)
			ui.Box(c).Grow(1)

			applyBtn := ui.ButtonBase(c.Key("ai-apply")).Height(22).PaddingX(8).Radius(6).
				AlignItems(ui.Center).Background(p.Primary).TextColor(p.OnPrimary).
				Label(locales.EditorApplySuggestion()).Tooltip(locales.EditorApplySuggestionTip())
			if applyBtn.Hovered() {
				applyBtn.Background(p.Primary.Mix(ui.Hex("#000000"), 0.08))
			}
			applyBtn.Children(func() {
				ui.Text(c, locales.EditorApplySuggestion()).FontSize(11.5).FontWeight(500).SingleLine()
			})
			if applyBtn.Clicked() {
				s.applyAI()
			}

			disBtn := ui.ButtonBase(c.Key("ai-dismiss")).Size(22, 22).Radius(6).
				Center().TextColor(p.AccentText).Label(locales.EditorDismissSuggestion()).Tooltip(locales.EditorDismissSuggestion())
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
			ui.Text(c, sug.Title).FontSize(12.5).FontWeight(500).TextColor(p.Text).Wrap().Selectable()
		}
		if sug.Note != "" && sug.Note != strings.TrimSpace(s.note) {
			ui.Text(c, sug.Note).FontSize(12).TextColor(p.Text2).Wrap().Selectable()
		}
	})
}

func (s *state) groupSection(c *ui.Context, p *theme.Palette) {
	lib := s.a.Store.Lib()
	ui.Column(c).FillWidth().Padding(12, 14).Gap(10).Radius(12).
		Background(p.Surface).Border(1, p.Line).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Children(func() {
			ui.Text(c, locales.EditorPlaceInGroup()).FontSize(12).FontWeight(600).TextColor(p.Text2)

			if s.aiReady() && s.aiSuggest != nil && !s.aiDismissed &&
				s.aiSuggest.GroupID != "" && lib.Sub(s.aiSuggest.GroupID, s.aiSuggest.SubID) != nil &&
				(s.aiSuggest.GroupID != s.groupID || s.aiSuggest.SubID != s.subID) {
				recPath := s.a.PathOf(&model.Bookmark{GroupID: s.aiSuggest.GroupID, SubID: s.aiSuggest.SubID})
				recBtn := ui.ButtonBase(c.Key("ai-rec-group")).Height(22).PaddingX(8).Radius(999).
					AlignItems(ui.Center).Background(p.AccentBg).TextColor(p.AccentText).
					Label(locales.EditorUseRecommendedGroup()).Tooltip(locales.EditorUseRecommendedGroup())
				recBtn.Children(func() {
					ui.Text(c, locales.EditorRecommendedGroup(recPath)).FontSize(11.5).SingleLine()
				})
				if recBtn.Clicked() {
					s.groupID = s.aiSuggest.GroupID
					s.subID = s.aiSuggest.SubID
					s.addingSub = false
					s.newSubName = ""
				}
			}
		})

		ui.Row(c).FillWidth().Wrap().Gap(4).Padding(0, 0, 8, 0).
			BorderWidth(0, 0, 1, 0).BorderColor(p.LineSoft).Children(func() {
			for i := range lib.Groups {
				g := &lib.Groups[i]
				on := g.ID == s.groupID
				pill := widget.Pill(c.Key("grp-"+g.ID), p, g.Name, on).
					Label(locales.EditorGroupItemLabel(g.Name)).Tooltip(locales.EditorGroupItemTip(g.Name))
				if pill.Clicked() {
					s.groupID = g.ID
					s.subID = ""
					if len(g.Subs) > 0 {
						s.subID = g.Subs[0].ID
					}
					s.addingSub = false
					s.newSubName = ""
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
						AlignItems(ui.Center).Label(locales.EditorSubItemLabel(sub.Name)).Tooltip(locales.EditorSubItemTip(sub.Name))
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
						Placeholder(locales.EditorNewSubPlaceholder()).Label(locales.EditorNewSubLabel())
					// Commit only on explicit Enter or validated Save (never on
					// blur) so clicking Save with an invalid URL or discarding
					// a draft never writes a phantom sub-group to the library.
					switch {
					case subIn.Submitted():
						s.commitNewSub()
					case subIn.Shortcut(0, ui.KeyEscape):
						s.addingSub = false
						s.newSubName = ""
					}
				} else {
					addBtn := ui.ButtonBase(c.Key("add-sub-btn")).Height(28).PaddingX(10).Radius(8).
						AlignItems(ui.Center).Gap(4).Border(1, p.Line).BorderStyle(ui.BorderDashed).
						TextColor(p.Muted).Label(locales.ActionNewSub()).Tooltip(locales.EditorNewSubTip())
					if addBtn.Hovered() {
						addBtn.Background(p.Hover).TextColor(p.Text)
					}
					addBtn.Children(func() {
						ui.Icon(c, theme.IconPlus).FontSize(11)
						ui.Text(c, locales.ActionNew()).FontSize(12).SingleLine()
					})
					if addBtn.Clicked() {
						s.addingSub = true
						s.newSubName = ""
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
			del := widget.Danger(c.Key("delete-btn"), p, locales.ActionDelete(), false).
				Label(locales.ActionDelete()).Tooltip(locales.EditorDeleteTip())
			if del.Clicked() {
				s.deleteCurrent()
			}
		}

		ui.Box(c).Grow(1)

		cancelBtn := widget.Secondary(c.Key("cancel-btn"), p, locales.ActionCancel()).
			Label(locales.ActionCancel()).Tooltip(locales.EditorCancelTip())
		if cancelBtn.Clicked() {
			s.requestClose()
		}

		saveBtn := widget.Primary(c.Key("save-btn"), p, locales.ActionSave()).
			Label(locales.ActionSave()).Tooltip(locales.EditorSaveTip(saveKey))
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
