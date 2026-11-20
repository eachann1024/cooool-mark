package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
)

func readyGreen(p *theme.Palette) ui.Color {
	if p.Dark {
		return ui.Hex("#5fb376")
	}
	return ui.Hex("#3d7a4f")
}

func (s *state) pageAI(c *ui.Context, p *theme.Palette) {
	a := s.a

	engineVals := []string{model.AILocal, model.AIOpenAI, model.AIOff}
	engineLabels := []string{locales.AiEngineLocal(), locales.AiEngineOpenAI(), locales.AiEngineOff()}

	card(c, p, func() {
		idx := slices.Index(engineVals, a.Settings.AIEngine)
		if idx < 0 {
			idx = len(engineVals) - 1
		}
		ui.Column(c).FillWidth().Padding(11, 16).Gap(8).Children(func() {
			ui.Text(c, locales.AiEngineLabel()).FontSize(13).TextColor(p.Text).SingleLine()
			if segmented(c, p, &idx, locales.AiEngineLabel(), engineLabels, true, false).Changed() && idx >= 0 && idx < len(engineVals) {
				a.Settings.AIEngine = engineVals[idx]
				s.saveNow()
			}
		})

		var st app.AIStatus
		if a.AI != nil {
			st = a.AI.Status()
		}

		// Current status row
		ui.Row(c).FillWidth().Padding(11, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			green := readyGreen(p)
			dotColor := p.Muted
			statusText := locales.AiStatusNotReady()
			switch {
			case st.Paused && st.Err == "":
				dotColor = p.Accent
				statusText = locales.AiStatusPaused()
			case st.Downloading:
				dotColor = p.Accent
				statusText = locales.AiStatusDownloading(int(st.Progress * 100))
			case st.Ready:
				dotColor = green
				statusText = locales.AiStatusReady()
				if st.Engine == model.AIOpenAI {
					statusText = locales.AiStatusConfigured()
				}
			case st.Err != "":
				dotColor = p.Danger
				statusText = st.Err
			case a.Settings.AIEngine == model.AIOff || a.Settings.AIEngine == "":
				statusText = locales.AiStatusOff()
			}
			ui.Box(c).Size(8, 8).Radius(4).Background(dotColor).Shrink(0)
			name := st.EngineName
			if name == "" {
				name = engineDisplayName(a.Settings.AIEngine)
			}
			ui.Text(c, name).FontSize(13).TextColor(p.Text).Grow(1).MinWidth(0).SingleLine()
			txtColor := p.Muted
			if st.Err != "" {
				txtColor = p.Danger
			} else if st.Ready {
				txtColor = green
			}
			statusEl := ui.Text(c, statusText).FontSize(12).TextColor(txtColor).SingleLine().MaxWidth(320).Selectable()
			if st.Err != "" {
				statusEl.Tooltip(statusText)
			}
		})

		if a.Settings.AIEngine == model.AILocal {
			s.localModelsSection(c, p)
		}

		if a.Settings.AIEngine == model.AIOpenAI {
			s.openAISection(c, p)
		}
	})

	card(c, p, func() {
		switchRow(c, p, locales.AiAutoPolish(), &a.Settings.AIAutoPolish, s.saveNow)
	})
}

func engineDisplayName(engine string) string {
	switch engine {
	case model.AILocal:
		return locales.AiEngineLocal()
	case model.AIOpenAI:
		return locales.AiEngineOpenAI()
	case model.AIOff, "":
		return locales.AiStatusOff()
	default:
		return engine
	}
}

type localModelSpec struct {
	id, name, size string
}

var localModels = []localModelSpec{
	{id: "qwen3.5-0.8b", name: "Qwen3.5-0.8B", size: "508 MB"},
	{id: "qwen3.5-2b", name: "Qwen3.5-2B", size: "1.4 GB"},
	{id: "qwen3.5-4b", name: "Qwen3.5-4B", size: "2.6 GB"},
}

func (s *state) localModelsSection(c *ui.Context, p *theme.Palette) {
	a := s.a
	selectedModel := a.Settings.LocalModel
	if selectedModel == "" {
		selectedModel = localModels[0].id
	}

	ui.Column(c).FillWidth().Padding(12, 16).Gap(8).Children(func() {
		for _, m := range localModels {
			var st app.AIStatus
			if a.AI != nil {
				st = a.AI.LocalModelStatus(m.id)
			}
			selected := selectedModel == m.id
			isDown := st.Downloading || st.Paused
			isReady := st.Ready

			border, bg := p.Line, p.Bg
			if selected {
				border = p.Accent
				bg = p.AccentBg.Mix(p.Surface, 0.65)
			}
			ui.Column(c.Key("model-"+m.id)).FillWidth().Padding(10, 12).Gap(8).Radius(9).Background(bg).Border(1, border).Children(func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					pick := ui.ButtonBase(c).Row().Gap(8).AlignItems(ui.Center).Grow(1).Cursor(ui.CursorPointer)
					pick.Children(func() {
						dot := ui.Box(c).Size(14, 14).Radius(7).Shrink(0)
						if selected {
							dot.Border(4, p.Primary).Background(p.OnPrimary)
						} else {
							dot.Border(1.5, p.Muted).Background(p.Surface)
						}
						ui.Text(c, m.name).FontSize(13).FontWeight(600).SingleLine()
						ui.Text(c, m.size).FontSize(12).TextColor(p.Muted).SingleLine()
						if isReady {
							badgeBg, badgeFg := ui.Hex("#e8f3ec"), ui.Hex("#2f6b42")
							if p.Dark {
								badgeBg, badgeFg = ui.Hex("#1e3325"), ui.Hex("#6bc484")
							}
							ui.Box(c).Padding(1, 6).Radius(5).Background(badgeBg).Children(func() {
								ui.Text(c, locales.AiStatusReady()).FontSize(11).TextColor(badgeFg).SingleLine()
							})
						}
					})
					if pick.Clicked() && a.Settings.LocalModel != m.id {
						a.Settings.LocalModel = m.id
						s.saveNow()
					}

					if isDown {
						label, tip := locales.ActionPause(), locales.AiPauseTip(m.name)
						if st.Paused {
							if st.Err != "" {
								label, tip = locales.ActionRetry(), locales.AiRetryTip(m.name)
							} else {
								label, tip = locales.ActionResume(), locales.AiResumeTip(m.name)
							}
						}
						if btnSecondary(c, p, label).Tooltip(tip).Clicked() && a.AI != nil {
							if st.Paused {
								if a.Settings.LocalModel != m.id {
									a.Settings.LocalModel = m.id
									s.saveNow()
								}
								s.startDownload(m.id)
							} else {
								a.AI.Pause()
							}
						}
						if btnDanger(c, p, locales.ActionDelete()).Tooltip(locales.AiRemovePartialTip(m.name)).Clicked() && a.AI != nil {
							s.removeModel(m.id)
						}
					} else if isReady {
						if btnDanger(c, p, locales.ActionDelete()).Tooltip(locales.AiRemoveModelTip(m.name)).Clicked() && a.AI != nil {
							s.removeModel(m.id)
						}
					} else {
						if btnPrimary(c, p, locales.ActionDownload()).Tooltip(locales.AiDownloadModelTip(m.name)).Clicked() && a.AI != nil {
							if a.Settings.LocalModel != m.id {
								a.Settings.LocalModel = m.id
								s.saveNow()
							}
							s.startDownload(m.id)
						}
					}
				})

				if isDown {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Progress(c, st.Progress).FillWidth().Label(locales.AiDownloadLabel(m.name))
						ui.Row(c).FillWidth().Gap(8).Children(func() {
							pct := int(st.Progress * 100)
							prefixColor := p.Muted
							statusLine := fmt.Sprintf("%s · %d%%", locales.AiDownloadingShort(), pct)
							if st.Err != "" {
								prefixColor = p.Danger
								statusLine = fmt.Sprintf("%d%% · %s", pct, locales.AiDownloadFailedPrefix(st.Err))
							} else if st.Paused {
								statusLine = fmt.Sprintf("%s · %d%%", locales.AiStatusPaused(), pct)
							}
							errEl := ui.Text(c, statusLine).FontSize(11.5).TextColor(prefixColor).Grow(1).MinWidth(0).SingleLine().Selectable()
							if st.Err != "" {
								errEl.Tooltip(st.Err)
							}
							if st.Total > 0 {
								ui.Textf(c, "%s / %s", formatBytes(st.Downloaded), formatBytes(st.Total)).FontSize(11.5).TextColor(p.Muted).Shrink(0).SingleLine()
							}
						})
					})
				} else if st.Err != "" {
					ui.Text(c, locales.AiDownloadFailedPrefix(st.Err)).FontSize(11.5).TextColor(p.Danger).Wrap().Selectable().Tooltip(st.Err)
				}
			})
		}
	})
}

func (s *state) startDownload(modelID string) {
	a := s.a
	go func() {
		if err := a.AI.Download(modelID); err != nil {
			a.Post(func() { a.SayErr(locales.AiDownloadErrToast(err.Error()), "", nil) })
		}
	}()
}

func (s *state) removeModel(modelID string) {
	a := s.a
	go func() {
		if err := a.AI.Remove(modelID); err != nil {
			a.Post(func() { a.SayErr(locales.AiRemoveErrToast(err.Error()), "", nil) })
		}
	}()
}

func formatBytes(n int64) string {
	const (
		mb = 1024 * 1024
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.0f MB", float64(n)/float64(mb))
	default:
		return fmt.Sprintf("%d KB", n/1024)
	}
}

func (s *state) openAISection(c *ui.Context, p *theme.Palette) {
	a := s.a
	keyPH := "sk-..."
	if !s.openAIKeyReady {
		keyPH = locales.AiKeyLoading()
	}
	ui.Column(c).FillWidth().Padding(12, 16).Gap(10).Children(func() {
		labeledInput(c, p, "openai-url", locales.AiOpenAIURL(), &a.Settings.OpenAIURL, "https://api.openai.com/v1", false, false, s.onOpenAIChanged, s.flushSave)
		labeledInput(c, p, "openai-model", locales.AiOpenAIModel(), &a.Settings.OpenAIModel, "gpt-4o-mini", false, false, s.onOpenAIChanged, s.flushSave)
		labeledInput(c, p, "openai-key", locales.AiOpenAIKey(), &s.openAIKey, keyPH, true, !s.openAIKeyReady, s.scheduleKeySave, s.flushKeySave)
	})
}

func labeledInput(c *ui.Context, p *theme.Palette, key, label string, val *string, placeholder string, secret, disabled bool, onChanged, onSubmitted func()) {
	ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(84).FontSize(12.5).TextColor(p.Text2).SingleLine()
		inBg := p.Surface
		if p.Dark {
			inBg = p.Bg
		}
		in := ui.TextInput(c.Key(key), val).Height(30).Padding(0, 9).Radius(8).
			Background(inBg).Border(1, p.Line.Mix(p.Muted, 0.25)).
			Grow(1).Placeholder(placeholder).Label(label).Disabled(disabled)
		if secret {
			in.Password()
		}
		if in.Changed() && onChanged != nil {
			onChanged()
		}
		if in.Submitted() && onSubmitted != nil {
			onSubmitted()
		}
	})
}

func shortenPath(full string) string {
	if full == "" {
		return ""
	}
	clean := filepath.Clean(full)
	display := clean
	sep := string(filepath.Separator)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		home = filepath.Clean(home)
		if clean == home {
			return "~"
		}
		if strings.HasPrefix(clean, home+sep) {
			display = "~" + clean[len(home):]
		}
	}
	parts := strings.Split(display, sep)
	if len(parts) > 4 {
		if parts[0] == "~" {
			return "~" + sep + parts[1] + sep + "…" + sep + parts[len(parts)-1]
		}
		if parts[0] == "" && len(parts) > 5 {
			return sep + parts[1] + sep + "…" + sep + parts[len(parts)-1]
		}
		return parts[0] + sep + "…" + sep + parts[len(parts)-1]
	}
	return display
}

func (s *state) storageCard(c *ui.Context, p *theme.Palette) {
	a := s.a

	localPath, _ := a.LibraryDir(model.StorageLocal, "")
	localDisplay := shortenPath(localPath)
	if localDisplay == "" {
		localDisplay = locales.StorageDefaultDir()
	}

	icloudDir := app.ICloudDir()
	icloudFull := ""
	icloudDisplay := locales.StorageICloudMissing()
	if icloudDir != "" {
		icloudFull = filepath.Join(icloudDir, "Cooool Mark")
		icloudDisplay = locales.StorageICloud() + " › " + locales.AppName()
	}

	folderFull := a.Settings.StorageFolder
	folderDisplay := locales.StorageFolderUnset()
	if folderFull != "" {
		folderDisplay = shortenPath(folderFull)
	}

	type storageOpt struct {
		id, name, displayPath, fullPath string
		disabled                        bool
	}
	opts := []storageOpt{
		{id: model.StorageLocal, name: locales.StorageLocal(), displayPath: localDisplay, fullPath: localPath},
		{id: model.StorageICloud, name: locales.StorageICloud(), displayPath: icloudDisplay, fullPath: icloudFull, disabled: icloudDir == ""},
		{id: model.StorageFolder, name: locales.StorageFolder(), displayPath: folderDisplay, fullPath: folderFull},
	}

	card(c, p, func() {
		ui.Column(c).FillWidth().Padding(12, 16).Gap(8).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.StorageLocationTitle()).FontSize(13).FontWeight(500).Grow(1).SingleLine()
				if btnSecondaryCompact(c, p, locales.StorageOpenFolder()).Clicked() {
					target := filepath.Join(a.Store.Dir(), "bookmarks.json")
					if _, err := os.Stat(target); err != nil {
						target = a.Store.Dir()
					}
					mygo.Shell.ShowItemInFolder(target)
				}
			})

			curStorage := a.Settings.Storage
			if curStorage == "" {
				curStorage = model.StorageLocal
			}
			for _, opt := range opts {
				on := curStorage == opt.id
				border, bg := p.Line, p.Bg
				if on {
					border, bg = p.Accent, p.AccentBg.Mix(p.Surface, 0.55)
				}
				row := ui.Row(c.Key("storage-"+opt.id)).FillWidth().Padding(9, 12).Gap(10).
					Radius(9).Background(bg).Border(1, border).AlignItems(ui.Center)
				if opt.fullPath != "" {
					row.Tooltip(opt.fullPath)
				}
				row.Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						pick := ui.ButtonBase(c).Row().FillWidth().Gap(10).AlignItems(ui.Center)
						if !opt.disabled {
							pick.Cursor(ui.CursorPointer)
						} else {
							pick.TextColor(p.Muted)
						}
						pick.Children(func() {
							dot := ui.Box(c).Size(14, 14).Radius(7).Shrink(0)
							if on {
								dot.Border(4, p.Primary).Background(p.OnPrimary)
							} else {
								dot.Border(1.5, p.Muted).Background(p.Surface)
							}
							ui.Text(c, opt.name).FontSize(13).FontWeight(500).SingleLine()
						})
						if pick.Clicked() {
							switch opt.id {
							case model.StorageLocal:
								if !on {
									if err := a.SetStorage(model.StorageLocal, ""); err != nil {
										a.SayErr(locales.StorageSwitchFailed(err.Error()), "", nil)
									} else {
										a.Say(locales.StorageSwitchedLocal())
									}
								}
							case model.StorageICloud:
								if opt.disabled {
									a.SayErr(locales.StorageICloudErr(), "", nil)
								} else if !on {
									if err := a.SetStorage(model.StorageICloud, ""); err != nil {
										a.SayErr(locales.StorageSwitchFailed(err.Error()), "", nil)
									} else {
										a.Say(locales.StorageSwitchedICloud())
									}
								}
							case model.StorageFolder:
								if a.Settings.StorageFolder == "" {
									s.pickStorageFolder()
								} else if !on {
									if err := a.SetStorage(model.StorageFolder, a.Settings.StorageFolder); err != nil {
										a.SayErr(locales.StorageSwitchFailed(err.Error()), "", nil)
									} else {
										a.Say(locales.StorageSwitchedFolder())
									}
								}
							}
						}
						ui.Box(c).FillWidth().Padding(0, 0, 0, 24).Children(func() {
							ui.Text(c, opt.displayPath).FontSize(12).TextColor(p.Muted).SingleLine().TruncateMiddle().Selectable()
						})
					})
					if opt.id == model.StorageFolder {
						if btnSecondaryCompact(c, p, locales.ActionSelectEllipsis()).Clicked() {
							s.pickStorageFolder()
						}
					}
				})
			}
		})
	})
}

func (s *state) pickStorageFolder() {
	a, win := s.a, s.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            win,
			Title:             locales.StoragePickFolderTitle(),
			Directory:         true,
			CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		dir := paths[0]
		a.Post(func() {
			if err := a.SetStorage(model.StorageFolder, dir); err != nil {
				a.SayErr(locales.StorageSwitchFailed(err.Error()), "", nil)
			} else {
				a.Say(locales.StorageSwitchedFolder())
			}
		})
	}()
}

func (s *state) pageData(c *ui.Context, p *theme.Palette) {
	s.storageCard(c, p)

	a := s.a
	card(c, p, func() {
		switchRow(c, p, locales.DataAutoBackup(), &a.Settings.AutoBackup, a.SaveSettings)

		settingRow(c, p, locales.DataImportRow(), func() {
			if btnSecondary(c, p, locales.ActionImportEllipsis()).Clicked() && a.Win.ShowImport != nil {
				a.Win.ShowImport("")
			}
		})

		settingRow(c, p, locales.DataExportRow(), func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if btnSecondary(c, p, "HTML").Clicked() {
					Export(a, "html")
				}
				if btnSecondary(c, p, "JSON").Clicked() {
					Export(a, "json")
				}
			})
		})

		settingRow(c, p, locales.DataBackupRow(), func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if btnSecondary(c, p, locales.DataBackupNow()).Clicked() {
					if _, err := a.Store.Backup("manual"); err != nil {
						a.SayErr(locales.DataBackupFailed(err.Error()), "", nil)
					} else {
						a.Say(locales.DataBackupDone())
					}
				}
				if btnSecondary(c, p, locales.DataOpenBackups()).Clicked() {
					s.openBackupsFolder()
				}
			})
		})
	})
}

func (s *state) openBackupsFolder() {
	dir := filepath.Join(s.a.Store.Dir(), "backups")
	_ = os.MkdirAll(dir, 0o755)
	target := dir
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		target = filepath.Join(dir, entries[len(entries)-1].Name())
	}
	mygo.Shell.ShowItemInFolder(target)
}
