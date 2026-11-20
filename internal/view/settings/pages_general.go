package settings

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
)

func (s *state) pageGeneral(c *ui.Context, p *theme.Palette) {
	a := s.a
	card(c, p, func() {
		segmentedRow(c, p, locales.MenuAppearance(), a.Settings.Appearance,
			[]string{locales.AppearanceAuto(), locales.AppearanceLight(), locales.AppearanceDark()},
			[]string{"auto", "light", "dark"},
			func(v string) {
				a.Settings.Appearance = v
				a.SaveSettings()
			},
		)
		segmentedRow(c, p, locales.SettingsLanguage(), localeSelection(mygo.App.ChosenLocale()),
			[]string{locales.SettingsLanguageSystem(), "简体中文", "English"},
			[]string{"", "zh-Hans", "en"},
			func(v string) {
				mygo.App.SetLocale(v)
			},
		)
		switchRow(c, p, locales.SettingsOpenAtLogin(), &s.openAtLogin, func() {
			if err := mygo.App.SetOpenAtLogin(s.openAtLogin); err != nil {
				s.openAtLogin = !s.openAtLogin
				a.SayErr(locales.SettingsOpenAtLoginErr(), "", nil)
			}
		})
		switchRow(c, p, locales.SettingsLaunchInBackground(), &a.Settings.LaunchInBackground, a.SaveSettings)
		if runtime.GOOS == "darwin" {
			switchRow(c, p, locales.SettingsHideDockIcon(), &a.Settings.HideDockWhenClosed, a.SaveSettings)
		}
		notifOn := !a.Settings.NotificationsOff
		switchRow(c, p, locales.SettingsNotifications(), &notifOn, func() {
			a.Settings.NotificationsOff = !notifOn
			a.SaveSettings()
		})
		segmentedRow(c, p, locales.SettingsDefaultView(), a.Settings.DefaultView,
			[]string{locales.SettingsViewList(), locales.SettingsViewGrid()},
			[]string{"list", "grid"},
			func(v string) {
				a.Settings.DefaultView = v
				a.View = v
				a.SaveSettings()
			},
		)
		switchRow(c, p, locales.SettingsShowNotes(), &a.Settings.ShowNote, a.SaveSettings)
		switchRow(c, p, locales.SettingsShowTags(), &a.Settings.ShowTags, a.SaveSettings)
	})

	card(c, p, func() {
		ui.Row(c).FillWidth().Padding(11, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SettingsSummonModes()).FontSize(13).FontWeight(500).TextColor(p.Text).SingleLine()
		})

		globalOn := a.Settings.GlobalSearchOn()
		switchRow(c, p, locales.SettingsGlobalSearch(), &globalOn, func() {
			a.Settings.GlobalSearchOff = !globalOn
			a.SaveSettings()
		})

		tray := a.Settings.ModeOn(model.ModeTray)
		switchRow(c, p, locales.SettingsModeTray(), &tray, func() {
			a.Settings.SetMode(model.ModeTray, tray)
			a.SaveSettings()
		})

		notch := a.Settings.ModeOn(model.ModeNotch)
		switchRow(c, p, locales.SettingsModeNotch(), &notch, func() {
			a.Settings.SetMode(model.ModeNotch, notch)
			a.SaveSettings()
		})

		edge := a.Settings.ModeOn(model.ModeEdge)
		switchRow(c, p, locales.SettingsModeEdge(), &edge, func() {
			a.Settings.SetMode(model.ModeEdge, edge)
			a.SaveSettings()
		})

		if edge {
			edgeSide := a.Settings.EdgeSide
			if edgeSide != "left" {
				edgeSide = "right"
			}
			ui.Box(c).FillWidth().Background(p.Bg.Mix(p.Surface, 0.5)).Children(func() {
				segmentedRow(c, p, locales.SettingsEdgeSide(), edgeSide,
					[]string{locales.SettingsEdgeLeft(), locales.SettingsEdgeRight()},
					[]string{"left", "right"},
					func(v string) {
						a.Settings.EdgeSide = v
						a.SaveSettings()
					},
				)
			})
		}
	})

	s.shortcutsCard(c, p)
	s.integrationCard(c, p)
	s.aboutUpdatesCard(c, p)
}

func localeSelection(tag string) string {
	low := strings.ToLower(strings.TrimSpace(tag))
	switch {
	case strings.HasPrefix(low, "zh"):
		return "zh-Hans"
	case strings.HasPrefix(low, "en"):
		return "en"
	default:
		return ""
	}
}

type shortcutItem struct {
	id       string
	label    string
	value    string
	canReset bool
	set      func(string)
	clear    func()
	reset    func()
}

func (s *state) shortcutsCard(c *ui.Context, p *theme.Palette) {
	a := s.a
	items := []shortcutItem{
		{
			id:       "search",
			label:    locales.SettingsShortcutSearch(),
			value:    a.Settings.EffectiveSearchShortcut(),
			canReset: a.Settings.GlobalSearchOff || (strings.TrimSpace(a.Settings.ShortcutSearch) != "" && !strings.EqualFold(strings.TrimSpace(a.Settings.ShortcutSearch), model.DefaultSearchShortcut)),
			set: func(acc string) {
				a.Settings.ShortcutSearch = acc
				a.Settings.GlobalSearchOff = false
			},
			clear: func() {
				a.Settings.ShortcutSearch = ""
				a.Settings.GlobalSearchOff = true
			},
			reset: func() {
				a.Settings.ShortcutSearch = ""
				a.Settings.GlobalSearchOff = false
			},
		},
		{
			id:    "quickAdd",
			label: locales.SettingsShortcutQuickAdd(),
			value: strings.TrimSpace(a.Settings.ShortcutQuickAdd),
			set:   func(acc string) { a.Settings.ShortcutQuickAdd = acc },
			clear: func() { a.Settings.ShortcutQuickAdd = "" },
		},
		{
			id:    "notch",
			label: locales.SettingsShortcutNotch(),
			value: strings.TrimSpace(a.Settings.ShortcutNotch),
			set:   func(acc string) { a.Settings.ShortcutNotch = acc },
			clear: func() { a.Settings.ShortcutNotch = "" },
		},
		{
			id:    "edge",
			label: locales.SettingsShortcutEdge(),
			value: strings.TrimSpace(a.Settings.ShortcutEdge),
			set:   func(acc string) { a.Settings.ShortcutEdge = acc },
			clear: func() { a.Settings.ShortcutEdge = "" },
		},
	}

	card(c, p, func() {
		ui.Row(c).FillWidth().Padding(11, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SettingsShortcutsTitle()).FontSize(13).FontWeight(500).TextColor(p.Text).SingleLine()
		})

		anyPending := a.HasAnyShortcutPending()
		for _, item := range items {
			errMsg := a.ShortcutErr(item.id)
			pending := a.IsShortcutPending(item.id)
			locked := pending || anyPending
			boundKeys := a.ShortcutBoundKeys(item.id)
			recording := s.recordingSlot == item.id

			ui.Column(c.Key("sc-row-"+item.id)).FillWidth().Padding(11, 16).Gap(6).Children(func() {
				ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, item.label).FontSize(13).TextColor(p.Text).Grow(1).MinWidth(0).SingleLine()

					if recording {
						rec := ui.Box(c.Key("sc-rec-"+item.id)).Height(26).PaddingX(10).Radius(7).AlignItems(ui.Center).
							Background(p.AccentBg).Border(1, p.Accent).Focusable().AutoFocus()
						rec.Children(func() {
							ui.Text(c, locales.SettingsShortcutPress()).FontSize(12).TextColor(p.AccentText).SingleLine()
						})
						rec.HandleInput(func(ev ui.InputEvent) bool {
							if ev.Kind != ui.InputKeyDown || ev.Repeat {
								return false
							}
							if ev.Key == ui.KeyEscape && ev.Mods == 0 {
								s.setRecording("")
								return true
							}
							if (ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete) && ev.Mods == 0 {
								item.clear()
								s.setRecording("")
								a.SaveSettings()
								return true
							}
							if isReservedAppShortcut(ev) {
								s.setRecording("")
								a.SetShortcutErr(item.id, locales.SettingsShortcutReserved())
								a.Invalidate()
								return true
							}
							if acc, ok := eventToAccelerator(ev); ok {
								if a.Win.SetShortcut != nil {
									// Clear recordingSlot without calling SetShortcutsPaused(false)
									// because SetShortcut resumes and tests registration atomically.
									s.recordingSlot = ""
									a.Win.SetShortcut(item.id, acc)
								} else {
									item.set(acc)
									s.setRecording("")
									a.SaveSettings()
								}
							}
							return true
						})
						if rec.FocusLeft() {
							s.setRecording("")
						}
					} else {
						pillBg, pillHover := p.Field, p.Line
						if p.Dark {
							pillBg, pillHover = p.RowSel, p.SideSel
						}
						pill := ui.ButtonBase(c.Key("sc-pill-" + item.id)).Height(26).PaddingX(10).Radius(7).AlignItems(ui.Center).
							Background(pillBg).Cursor(ui.CursorPointer).Tooltip(locales.SettingsShortcutRecordTip()).Disabled(locked)
						if !locked && pill.Hovered() {
							pill.Background(pillHover)
						}
						pill.Children(func() {
							display := item.value
							if boundKeys != "" {
								display = boundKeys
							}
							if display != "" {
								ui.Text(c, formatShortcutDisplay(display)).FontSize(12).TextColor(p.Text).SingleLine()
							} else {
								ui.Text(c, locales.SettingsShortcutUnset()).FontSize(12).TextColor(p.Muted).SingleLine()
							}
						})
						if !locked && pill.Clicked() {
							s.setRecording(item.id)
						}
					}

					if item.value != "" {
						clrBtn := btnSecondaryCompact(c, p, locales.ActionClear()).Tooltip(locales.SettingsShortcutClearTip()).Disabled(locked)
						if !locked && clrBtn.Clicked() {
							item.clear()
							s.setRecording("")
							a.SaveSettings()
						}
					}
					if item.canReset && item.reset != nil {
						rstBtn := btnSecondaryCompact(c, p, locales.SettingsShortcutResetDefault()).Disabled(locked)
						if !locked && rstBtn.Clicked() {
							if a.Win.SetShortcut != nil && item.id == "search" {
								s.recordingSlot = ""
								a.Win.SetShortcut(item.id, model.DefaultSearchShortcut)
							} else {
								item.reset()
								s.setRecording("")
								a.SaveSettings()
							}
						}
					}
				})

				if errMsg != "" {
					ui.Text(c, locales.SettingsShortcutFailed(errMsg)).FontSize(11.5).TextColor(p.Danger).Wrap().Selectable()
				} else if pending {
					ui.Text(c, locales.SettingsShortcutPending()).FontSize(11.5).TextColor(p.Muted).Wrap().Selectable()
				}
			})
		}
	})
}

func isReservedAppShortcut(ev ui.InputEvent) bool {
	if ev.Mods != ui.Cmd {
		return false
	}
	k := ev.Key
	if k == ui.KeyUnknown {
		k = ev.PhysicalKey
	}
	switch k {
	case ui.KeyQ, ui.KeyW, ui.KeyH, ui.KeyM, ui.KeyC, ui.KeyV, ui.KeyX, ui.KeyA, ui.KeyZ, ui.KeyComma, ui.KeyN, ui.KeyF, ui.KeyL:
		return true
	default:
		return false
	}
}

func eventToAccelerator(ev ui.InputEvent) (string, bool) {
	if ev.Mods&(ui.Super|ui.Ctrl|ui.Alt) == 0 {
		return "", false
	}
	k := ev.Key
	if k == ui.KeyUnknown {
		k = ev.PhysicalKey
	}
	keyName, ok := keyToAccelerator(k)
	if !ok {
		return "", false
	}

	var parts []string
	if ev.Mods&ui.Cmd != 0 {
		parts = append(parts, "CmdOrCtrl")
	}
	if runtime.GOOS == "darwin" {
		if ev.Mods&ui.Ctrl != 0 {
			parts = append(parts, "Ctrl")
		}
	} else {
		if ev.Mods&ui.Super != 0 {
			parts = append(parts, "Super")
		}
	}
	if ev.Mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if ev.Mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(append(parts, keyName), "+"), true
}

func keyToAccelerator(k ui.Key) (string, bool) {
	if k >= ui.KeyA && k <= ui.KeyZ {
		return string(rune('A' + (k - ui.KeyA))), true
	}
	if k >= ui.Key0 && k <= ui.Key9 {
		return string(rune('0' + (k - ui.Key0))), true
	}
	if k >= ui.KeyF1 && k <= ui.KeyF12 {
		return fmt.Sprintf("F%d", 1+int(k-ui.KeyF1)), true
	}
	switch k {
	case ui.KeySpace:
		return "Space", true
	case ui.KeyMinus:
		return "-", true
	case ui.KeyEqual:
		return "=", true
	case ui.KeyComma:
		return ",", true
	case ui.KeyPeriod:
		return ".", true
	case ui.KeySlash:
		return "/", true
	case ui.KeySemicolon:
		return ";", true
	case ui.KeyQuote:
		return "'", true
	case ui.KeyBracketLeft:
		return "[", true
	case ui.KeyBracketRight:
		return "]", true
	case ui.KeyBackslash:
		return "\\", true
	case ui.KeyBackquote:
		return "`", true
	default:
		return "", false
	}
}

func formatShortcutDisplay(acc string) string {
	parts := strings.Split(acc, "+")
	if len(parts) == 0 {
		return acc
	}
	if runtime.GOOS != "darwin" {
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if strings.EqualFold(p, "CmdOrCtrl") || strings.EqualFold(p, "CommandOrControl") {
				out = append(out, "Ctrl")
			} else if p != "" {
				out = append(out, p)
			}
		}
		return strings.Join(out, "+")
	}
	var mods strings.Builder
	var key string
	for i, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if i < len(parts)-1 {
			switch strings.ToLower(p) {
			case "cmdorctrl", "commandorcontrol", "cmd", "command", "super", "meta":
				mods.WriteString("⌘")
			case "ctrl", "control":
				mods.WriteString("⌃")
			case "alt", "option":
				mods.WriteString("⌥")
			case "shift":
				mods.WriteString("⇧")
			default:
				mods.WriteString(p + "+")
			}
		} else {
			key = p
		}
	}
	return mods.String() + key
}

func (s *state) integrationCard(c *ui.Context, p *theme.Palette) {
	a := s.a
	examples := []string{
		"coooolmark://open",
		"coooolmark://search?q=go",
		"coooolmark://add?url=https://example.com&title=Example",
	}
	card(c, p, func() {
		ui.Row(c).FillWidth().Padding(11, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SettingsIntegrationTitle()).FontSize(13).FontWeight(500).TextColor(p.Text).SingleLine()
		})
		for _, ex := range examples {
			ui.Row(c.Key("scheme-"+ex)).FillWidth().Padding(10, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, ex).FontSize(12.5).TextColor(p.Text).Grow(1).MinWidth(0).Wrap().Selectable()
				if btnSecondaryCompact(c, p, locales.ActionCopy()).Tooltip(locales.SettingsURLSchemeCopyTip()).Clicked() {
					c.WriteClipboardThen(ex, func(err error) {
						if err != nil {
							a.SayErr(locales.ToastCopyFailed(err.Error()), "", nil)
							return
						}
						a.Say(locales.ToastCopiedURL())
					})
				}
			})
		}
	})
}

func (s *state) aboutUpdatesCard(c *ui.Context, p *theme.Palette) {
	a := s.a
	card(c, p, func() {
		ui.Row(c).FillWidth().Padding(11, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SettingsAboutUpdates()).FontSize(13).FontWeight(500).TextColor(p.Text).SingleLine()
		})

		ui.Row(c).FillWidth().Padding(11, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			ver := strings.TrimPrefix(app.Version(), "v")
			ui.Text(c, locales.SettingsCurrentVersion(ver)).FontSize(13).TextColor(p.Text).Grow(1).MinWidth(0).Selectable()

			checkLabel := locales.MenuCheckUpdates()
			if a.Update.Checking {
				checkLabel = locales.SettingsCheckingUpdate()
			}
			checkBtn := btnSecondary(c, p, checkLabel).Disabled(a.Update.Checking)
			if checkBtn.Clicked() && !a.Update.Checking {
				a.CheckUpdate(true)
			}
			if btnSecondary(c, p, locales.SettingsOpenAbout()).Clicked() && a.Win.ShowAbout != nil {
				a.Win.ShowAbout()
			}
		})

		switch {
		case a.Update.Checking:
			ui.Row(c).FillWidth().Padding(11, 16).Children(func() {
				ui.Text(c, locales.SettingsCheckingUpdate()).FontSize(12.5).TextColor(p.Muted).Selectable()
			})
		case a.Update.Err != "":
			ui.Row(c).FillWidth().Padding(11, 16).Children(func() {
				ui.Text(c, locales.SettingsUpdateCheckFailed(a.Update.Err)).FontSize(12.5).TextColor(p.Danger).Wrap().Selectable()
			})
		case a.Update.Available && a.Update.Release != nil:
			rel := a.Update.Release
			ui.Column(c).FillWidth().Padding(12, 16).Gap(8).Children(func() {
				ver := strings.TrimPrefix(rel.Version, "v")
				ui.Text(c, locales.SettingsUpdateAvailable(ver)).FontSize(13).FontWeight(500).TextColor(p.AccentText).Selectable()
				if notes := rel.Notes(locales.IsChinese()); len(notes) > 0 {
					lines := make([]string, 0, len(notes))
					for _, n := range notes {
						if trimmed := strings.TrimSpace(n); trimmed != "" {
							lines = append(lines, "• "+trimmed)
						}
					}
					if len(lines) > 0 {
						ui.Text(c, strings.Join(lines, "\n")).FontSize(12).TextColor(p.Text2).Wrap().Selectable()
					}
				}
				if rel.DownloadURL != "" || rel.HTMLURL != "" {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if rel.DownloadURL != "" {
							if btnPrimary(c, p, locales.SettingsOpenDownload()).Clicked() {
								_ = a.OpenURL(rel.DownloadURL)
							}
						}
						if rel.HTMLURL != "" {
							if btnSecondary(c, p, locales.SettingsViewRelease()).Clicked() {
								_ = a.OpenURL(rel.HTMLURL)
							}
						}
					})
				}
			})
		case a.Update.Checked:
			ui.Row(c).FillWidth().Padding(11, 16).Children(func() {
				ui.Text(c, locales.SettingsUpToDate()).FontSize(12.5).TextColor(p.Muted).Selectable()
			})
		}
	})
}
