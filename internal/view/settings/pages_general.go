package settings

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/update"
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
	})

	s.compactCardSection(c, p)

	card(c, p, func() {
		ui.Row(c).FillWidth().Padding(11, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SettingsSummonModes()).FontSize(13).FontWeight(500).TextColor(p.Text).SingleLine()
		})

		globalOn := a.Settings.GlobalSearchOn()
		switchRow(c, p, locales.SettingsGlobalSearch(), &globalOn, func() {
			a.Settings.GlobalSearchOff = !globalOn
			if !globalOn {
				if s.recordingSlot == "search" {
					s.setRecording("")
				}
				if a.Win.CancelPendingShortcut != nil {
					a.Win.CancelPendingShortcut("search")
				}
			}
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

func (s *state) compactCardSection(c *ui.Context, p *theme.Palette) {
	a := s.a
	cc := a.Settings.CompactCard()
	apply := func(next model.CompactCardLayout) {
		next = next.Clamp()
		a.Settings.SetCompactCard(next)
		s.syncCompactSliders()
		a.Invalidate()
		s.scheduleSave()
	}

	presets := []struct {
		id     string
		label  string
		layout model.CompactCardLayout
	}{
		{"title", locales.SettingsCompactPresetTitle(), model.CompactPresetTitle},
		{"two", locales.SettingsCompactPresetTwo(), model.CompactPresetTwo},
		{"airy", locales.SettingsCompactPresetAiry(), model.CompactPresetAiry},
	}

	ui.Column(c).FillWidth().Radius(12).Background(p.Surface).Border(1, p.Line).Children(func() {
		settingRow(c, p, locales.SettingsCompactCard(), func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Padding(2).Gap(2).Radius(8).Background(p.Field).AlignItems(ui.Center).Children(func() {
					for _, pr := range presets {
						active := cc == pr.layout
						btn := ui.ButtonBase(c.Key("cc-preset-"+pr.id)).Height(24).PaddingX(10).Radius(6).
							AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(pr.label)
						if active {
							bg := p.Surface
							if p.Dark {
								bg = p.SideSel
							}
							btn.Background(bg).TextColor(p.AccentText).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
						} else {
							btn.TextColor(p.Muted)
							if btn.Hovered() {
								btn.TextColor(p.Text)
							}
						}
						btn.Children(func() {
							t := ui.Text(c, pr.label).FontSize(12).SingleLine()
							if active {
								t.FontWeight(500)
							}
						})
						if btn.Clicked() {
							apply(pr.layout)
						}
					}
				})

				if cc != model.DefaultCompactCardLayout() {
					rst := ui.ButtonBase(c.Key("cc-reset")).Height(24).PaddingX(8).Radius(6).
						AlignItems(ui.Center).Border(1, p.Line).TextColor(p.Muted).Cursor(ui.CursorPointer).
						Label(locales.SettingsCompactReset())
					if rst.Hovered() {
						rst.Background(p.Hover).TextColor(p.Text)
					}
					rst.Children(func() {
						ui.Text(c, locales.SettingsCompactReset()).FontSize(11.5).SingleLine()
					})
					if rst.Clicked() {
						apply(model.DefaultCompactCardLayout())
					}
				}
			})
		})

		ui.Column(c).FillWidth().Padding(0, 16, 16, 16).Children(func() {
			ui.Column(c).FillWidth().Padding(14).Radius(10).Background(p.Bg).Border(1, p.Line).Gap(12).Children(func() {
				meta := locales.SettingsCompactPreviewNotes(cc.Width, int(cc.Height()), cc.NoteLines)
				if cc.NoteLines == 0 {
					meta = locales.SettingsCompactPreviewTitleOnly(cc.Width, int(cc.Height()))
				}
				ui.Row(c).FillWidth().Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, locales.SettingsCompactPreview()).FontSize(12).FontWeight(500).TextColor(p.Text).SingleLine()
					ui.Text(c, meta).FontSize(11.5).TextColor(p.Muted).SingleLine()
				})

				s.compactPreviewGrid(c, p, cc)

				ui.Box(c).FillWidth().Height(1).Background(p.Line)

				linesLabel := locales.SettingsCompactNoteLinesVal(cc.NoteLines)
				if cc.NoteLines == 0 {
					linesLabel = locales.SettingsCompactNoteLinesZero()
				}
				changed := false
				if s.compactSlider(c, p, "lines", locales.SettingsCompactNoteLines(), linesLabel, &s.compactLines, 0, 4, 1) {
					changed = true
				}
				ui.Grid(c).Columns(2).Gap(14).FillWidth().Children(func() {
					if s.compactSlider(c, p, "width", locales.SettingsCompactWidth(), fmt.Sprintf("%d px", cc.Width), &s.compactWidth, 140, 240, 4) {
						changed = true
					}
					if s.compactSlider(c, p, "gap", locales.SettingsCompactGap(), fmt.Sprintf("%d px", cc.Gap), &s.compactGap, 4, 16, 2) {
						changed = true
					}
					if s.compactSlider(c, p, "pad", locales.SettingsCompactPad(), fmt.Sprintf("%d px", cc.Pad), &s.compactPad, 6, 16, 1) {
						changed = true
					}
					if s.compactSlider(c, p, "radius", locales.SettingsCompactRadius(), fmt.Sprintf("%d px", cc.Radius), &s.compactRadius, 4, 16, 1) {
						changed = true
					}
				})
				if changed {
					next := model.CompactCardLayout{
						NoteLines: int(math.Round(s.compactLines)),
						Width:     int(math.Round(s.compactWidth/4)) * 4,
						Gap:       int(math.Round(s.compactGap/2)) * 2,
						Pad:       int(math.Round(s.compactPad)),
						Radius:    int(math.Round(s.compactRadius)),
					}.Clamp()
					if next != cc {
						a.Settings.SetCompactCard(next)
						a.Invalidate()
						s.scheduleSave()
					}
				}
			})
		})
	})
}

func (s *state) compactPreviewGrid(c *ui.Context, p *theme.Palette, cc model.CompactCardLayout) {
	type sample struct {
		badge, color, title, note string
	}
	var items []sample
	if locales.IsChinese() {
		items = []sample{
			{"IP", "#3d3c38", "IP 查询 – 评分/地理位置", "查纯净度、ASN 归属、家宽风险评分与流媒体原生解锁情况，排查代理节点和出口线路前先跑一遍。"},
			{"C", "#1f1e1d", "Claude AIP 检测", "验证当前出口 IP 是否支持原生访问。"},
			{"M", "#4a6fd8", "Mark 控制台", ""},
		}
	} else {
		items = []sample{
			{"IP", "#3d3c38", "IP Score & Geo Lookup", "Check purity, ASN ownership, residential risk score and streaming unlock before testing exit nodes."},
			{"C", "#1f1e1d", "Claude Access Check", "Verify whether the current exit IP has native access."},
			{"M", "#4a6fd8", "Mark Console", ""},
		}
	}

	const previewW = float32(548)
	gap := float32(cc.Gap)
	cols := max(1, min(3, int((previewW+gap)/(float32(cc.Width)+gap))))
	cardW := (previewW - float32(cols-1)*gap) / float32(cols)
	cardH := cc.Height()
	padX := float32(cc.Pad)
	padY := cc.PadY()
	if cc.NoteLines <= 0 {
		padY = 0
	}
	lineH := cc.NoteLineHeight()
	noteBoxH := cc.NoteBoxHeight()

	ui.Grid(c).Columns(cols).Gap(gap).FillWidth().Children(func() {
		for i, it := range items {
			cardEl := ui.Row(c.Key(fmt.Sprintf("cc-prev-%d", i))).MinWidth(0).Height(cardH).
				Padding(padY, padX).Radius(float32(cc.Radius)).Background(p.Surface).Border(1, p.Line)
			if cc.NoteLines > 0 {
				cardEl.AlignItems(ui.Start)
			} else {
				cardEl.AlignItems(ui.Center)
			}
			cardEl.Children(func() {
				tile := func() {
					ui.Row(c).Size(22, 22).Radius(6).Background(ui.Hex(it.color)).Center().Shrink(0).Children(func() {
						ui.Text(c, it.badge).FontSize(10).FontWeight(700).TextColor(ui.Hex("#ffffff")).SingleLine()
					})
				}
				if cc.NoteLines <= 0 {
					ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
						tile()
						ui.Text(c, it.title).FontSize(12.5).FontWeight(500).TextColor(p.Text).SingleLine().Shrink(1).MinWidth(0)
					})
					return
				}
				ui.Column(c).Grow(1).MinWidth(0).FillHeight().Gap(6).Children(func() {
					ui.Row(c).FillWidth().MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
						tile()
						ui.Text(c, it.title).FontSize(12.5).FontWeight(500).TextColor(p.Text).SingleLine().Shrink(1).MinWidth(0)
					})
					ui.Box(c).FillWidth().Height(noteBoxH).Clip().Children(func() {
						if it.note != "" {
							ui.Text(c, it.note).FontSize(11.5).LineHeight(lineH / 11.5).TextColor(p.Muted).MaxLines(cc.NoteLines)
							if cc.NoteOverflows(it.note, cardW) {
								ui.Box(c).Absolute().Right(0).Bottom(0).Width(44).Height(lineH).PassThrough().
									LinearGradient(ui.LinearGradient{
										Angle: 90,
										Stops: []ui.GradientStop{
											{Color: p.Surface.Alpha(0), At: 0},
											{Color: p.Surface.Alpha(0.88), At: 0.68},
											{Color: p.Surface, At: 1},
										},
									})
							}
						}
					})
				})
			})
		}
	})
}

func (s *state) compactSlider(c *ui.Context, p *theme.Palette, key, label, valText string, val *float64, lo, hi, step float64) bool {
	changed := false
	ui.Column(c.Key("cc-sl-wrap-"+key)).FillWidth().Gap(4).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, label).FontSize(12).TextColor(p.Text).SingleLine()
			ui.Text(c, valText).FontSize(11.5).FontWeight(500).TextColor(p.AccentText).SingleLine()
		})
		sl := ui.SliderBase(c.Key("cc-sl-"+key), val, lo, hi).FillWidth().Height(20).PaddingX(7).
			Step(step).Label(label).Cursor(ui.CursorPointer)
		sl.Draw(func(pt *ui.Painter, r ui.Rect) {
			t := float32((*val - lo) / (hi - lo))
			if t < 0 {
				t = 0
			} else if t > 1 {
				t = 1
			}
			trackX := r.X + 7
			trackW := max(float32(0), r.W-14)
			trackY := r.Y + (r.H-4)/2
			pt.Fill(ui.Rect{X: trackX, Y: trackY, W: trackW, H: 4}, p.Line, 2)
			if fillW := trackW * t; fillW > 0 {
				pt.Fill(ui.Rect{X: trackX, Y: trackY, W: fillW, H: 4}, p.Accent, 2)
			}
			cx := trackX + trackW*t
			cy := r.Y + r.H/2
			pt.Fill(ui.Rect{X: cx - 7, Y: cy - 7, W: 14, H: 14}, ui.Hex("#ffffff"), 7)
			pt.Fill(ui.Rect{X: cx - 5, Y: cy - 5, W: 10, H: 10}, p.Accent, 5)
		})
		if sl.Changed() {
			changed = true
		}
	})
	return changed
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
		if anyPending && s.recordingSlot != "" {
			s.setRecording("")
		}
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
								if a.Win.SetShortcut != nil {
									s.recordingSlot = ""
									a.Win.SetShortcut(item.id, "")
								} else {
									item.clear()
									s.setRecording("")
									a.SaveSettings()
								}
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
							if a.Win.SetShortcut != nil {
								s.recordingSlot = ""
								a.Win.SetShortcut(item.id, "")
							} else {
								item.clear()
								s.setRecording("")
								a.SaveSettings()
							}
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
	snap := a.Updater.Snapshot()
	busy := snap.Status == update.StatusChecking || snap.Status == update.StatusDownloading || snap.Status == update.StatusInstalling
	card(c, p, func() {
		ui.Row(c).FillWidth().Padding(11, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SettingsAboutUpdates()).FontSize(13).FontWeight(500).TextColor(p.Text).SingleLine()
		})

		ui.Row(c).FillWidth().Padding(11, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			ver := strings.TrimPrefix(snap.CurrentVersion, "v")
			ui.Text(c, locales.SettingsCurrentVersion(ver)).FontSize(13).TextColor(p.Text).Grow(1).MinWidth(0).Selectable()

			checkLabel := locales.MenuCheckUpdates()
			if snap.Status == update.StatusChecking {
				checkLabel = locales.SettingsCheckingUpdate()
			}
			if btnSecondary(c, p, checkLabel).Disabled(busy).Clicked() && !busy {
				a.CheckUpdate(true)
			}
			if btnSecondary(c, p, locales.SettingsOpenAbout()).Clicked() && a.Win.ShowAbout != nil {
				a.Win.ShowAbout()
			}
		})

		switch snap.Status {
		case update.StatusChecking:
			ui.Row(c).FillWidth().Padding(11, 16).Children(func() {
				ui.Text(c, locales.SettingsCheckingUpdate()).FontSize(12.5).TextColor(p.Muted).Selectable()
			})
		case update.StatusError:
			ui.Row(c).FillWidth().Padding(11, 16).Children(func() {
				ui.Text(c, locales.SettingsUpdateCheckFailed(snap.ErrorMessage)).FontSize(12.5).TextColor(p.Danger).Wrap().Selectable()
			})
		case update.StatusUpToDate:
			ui.Row(c).FillWidth().Padding(11, 16).Children(func() {
				ui.Text(c, locales.SettingsUpToDate()).FontSize(12.5).TextColor(p.Muted).Selectable()
			})
		case update.StatusAvailable, update.StatusDownloading, update.StatusReady, update.StatusInstalling:
			s.updateAvailableSection(c, p, snap)
		}

		segmentedRow(c, p, locales.SettingsUpdateSource(), string(snap.SourceMode),
			[]string{locales.SettingsUpdateSourceAuto(), locales.SettingsUpdateSourceMirror(), locales.SettingsUpdateSourceGitHub()},
			[]string{string(update.SourceAuto), string(update.SourceMirror), string(update.SourceGitHub)},
			func(val string) { a.Updater.SetSourceMode(update.SourceMode(val)) })
	})
}

func (s *state) updateAvailableSection(c *ui.Context, p *theme.Palette, snap update.Snapshot) {
	a := s.a
	ui.Column(c).FillWidth().Padding(12, 16).Gap(8).Children(func() {
		ver := strings.TrimPrefix(snap.LatestVersion, "v")
		heading := locales.SettingsUpdateAvailable(ver)
		if snap.Status == update.StatusReady {
			heading = locales.SettingsUpdateReady(ver)
		}
		ui.Text(c, heading).FontSize(13).FontWeight(500).TextColor(p.AccentText).Selectable()
		if lines := snap.Notes.ForLanguage(locales.IsChinese()).Lines(); len(lines) > 0 {
			bullets := make([]string, len(lines))
			for i, line := range lines {
				bullets[i] = "• " + line
			}
			ui.Text(c, strings.Join(bullets, "\n")).FontSize(12).TextColor(p.Text2).Wrap().Selectable()
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			switch snap.Status {
			case update.StatusAvailable:
				if snap.InPlace {
					if btnPrimary(c, p, locales.SettingsUpdateDownload()).Clicked() {
						a.Updater.StartDownload()
					}
				} else if btnPrimary(c, p, locales.SettingsOpenDownload()).Clicked() {
					_ = a.OpenURL(snap.ReleaseURL)
				}
				if btnSecondary(c, p, locales.SettingsViewRelease()).Clicked() {
					_ = a.OpenURL(snap.ReleaseURL)
				}
			case update.StatusDownloading:
				label := locales.SettingsUpdateDownloading(strconv.Itoa(snap.Percent))
				if snap.Preparing {
					label = locales.SettingsUpdatePreparing()
				}
				ui.Text(c, label).FontSize(12.5).TextColor(p.Muted).SingleLine()
				if btnSecondary(c, p, locales.ActionCancel()).Clicked() {
					a.Updater.CancelDownload()
				}
			case update.StatusReady:
				if btnPrimary(c, p, locales.SettingsUpdateRestart()).Clicked() {
					a.Updater.InstallNow()
				}
				if btnSecondary(c, p, locales.SettingsViewRelease()).Clicked() {
					_ = a.OpenURL(snap.ReleaseURL)
				}
			case update.StatusInstalling:
				ui.Text(c, locales.SettingsUpdateInstalling()).FontSize(12.5).TextColor(p.Muted).SingleLine()
			}
		})
	})
}
