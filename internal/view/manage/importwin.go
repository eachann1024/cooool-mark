package manage

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/importer"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

type importState struct {
	app            *app.App
	win            *mygo.Window
	closed         bool
	plan           *app.ImportPlan
	planRev        int64
	loading        bool
	loadingFile    string
	loadingPath    string
	queue          []string
	picking        bool
	loadSeq        int
	err            string
	confirmRestore bool
}

var (
	importWin   *mygo.Window
	importModel *importState
)

// RefreshChrome updates the import window title and requests a redraw when open.
func RefreshChrome() {
	if importWin != nil && !importWin.IsDestroyed() {
		importWin.SetTitle(locales.ImportWindowTitle())
		importWin.Invalidate()
	}
}

func showImport(a *app.App, path string) {
	a.PrepareRegularWindow()
	if importWin != nil && !importWin.IsDestroyed() {
		importWin.SetTitle(locales.ImportWindowTitle())
		importWin.Show()
		importWin.Focus()
		importWin.Invalidate()
		if path != "" && importModel != nil {
			importModel.enqueue(path)
		} else if importModel != nil && importModel.plan == nil && !importModel.loading && !importModel.picking && importModel.err == "" {
			importModel.pickFile()
		}
		return
	}
	st := &importState{app: a}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: locales.ImportWindowTitle(), Width: 540, Height: 420, MinWidth: 440, MinHeight: 340,
		StateKey: "manage-import", TitleBarStyle: mygo.TitleBarHiddenInset,
		TrafficLightPosition: widget.Lights(), TitleBarHeight: widget.HeaderH,
		Content: ui.View(st.view),
	})
	st.win, importWin, importModel = w, w, st
	a.AddRegularWindow(w)
	w.OnClosed(func() {
		st.closed = true
		st.loadSeq++
		st.loading = false
		st.loadingFile = ""
		st.loadingPath = ""
		st.queue = nil
		st.picking = false
		st.confirmRestore = false
		st.win = nil
		if importWin == w {
			importWin, importModel = nil, nil
		}
	})
	if path != "" {
		st.loadFile(path)
	} else {
		st.pickFile()
	}
}

func (s *importState) invalidate() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

func (s *importState) imp() app.Importer {
	if s.app.Import != nil {
		return s.app.Import
	}
	return importer.New()
}

func (s *importState) pickFile() {
	if s.picking || s.loading {
		return
	}
	s.picking = true
	s.err = ""
	startSeq := s.loadSeq
	s.invalidate()
	win := s.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: win, Title: locales.ImportPickTitle(),
			Filters: []mygo.FileFilter{{Name: locales.ImportPickFilter(), Extensions: []string{"html", "htm", "json"}}},
		})
		s.app.Post(func() {
			if s.closed || (importModel != nil && importModel != s) {
				return
			}
			s.picking = false
			if err != nil {
				s.err = locales.ImportPickFailed(err.Error())
			} else if len(paths) > 0 {
				if s.loadSeq != startSeq || s.loading {
					// Other files arrived (e.g. via drag-drop) while the picker
					// was open; enqueue instead of clobbering the active task.
					for _, p := range paths {
						s.enqueue(p)
					}
				} else {
					s.loadFile(paths[0])
					for _, extra := range paths[1:] {
						s.enqueue(extra)
					}
				}
			}
		})
	}()
}

func (s *importState) enqueue(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	clean := filepath.Clean(path)
	if !s.loading && s.plan == nil {
		s.loadFile(path)
		return
	}
	if s.loading && s.loadingPath == clean {
		return
	}
	if s.plan != nil && filepath.Clean(s.plan.File) == clean {
		return
	}
	for _, queued := range s.queue {
		if filepath.Clean(queued) == clean {
			return
		}
	}
	s.queue = append(s.queue, path)
	s.invalidate()
}

func (s *importState) loadNextQueued() bool {
	if len(s.queue) == 0 {
		return false
	}
	next := s.queue[0]
	s.queue = s.queue[1:]
	s.loadFile(next)
	return true
}

func (s *importState) loadFile(path string) {
	s.loadSeq++
	seq := s.loadSeq
	s.plan = nil
	s.planRev = 0
	s.confirmRestore = false
	s.loading = true
	s.loadingFile = filepath.Base(path)
	s.loadingPath = filepath.Clean(path)
	s.err = ""
	s.invalidate()
	snap, imp := importer.CloneLibrary(s.app.Store.Lib()), s.imp()
	go func() {
		plan, err := imp.Parse(path, snap)
		s.app.Post(func() {
			if s.closed || (importModel != nil && importModel != s) || seq != s.loadSeq {
				return
			}
			s.loading = false
			s.loadingFile = ""
			s.loadingPath = ""
			if err != nil {
				s.plan, s.err = nil, locales.ImportReadFailed(err.Error())
				return
			}
			s.plan = plan
			s.syncPlanStats()
		})
	}()
}

func (s *importState) syncPlanStats() {
	if s.plan == nil || s.loading || s.app == nil || s.app.Store == nil {
		return
	}
	lib := s.app.Store.Lib()
	if lib == nil || s.planRev == lib.Revision {
		return
	}
	importer.RefreshStats(s.plan, lib)
	s.planRev = lib.Revision
}

func (s *importState) view(c *ui.Context) {
	p := theme.Apply(c, s.app.Settings.Appearance)
	s.syncPlanStats()
	c.OnWindowShortcut(0, ui.KeyEscape, func() {
		if s.confirmRestore {
			s.confirmRestore = false
			return
		}
		if s.win != nil {
			s.win.Close()
		}
	})
	bar := c.TitleBar()
	leftPad, rightPad := max(bar.Left+12, float32(16)), max(bar.Right+16, float32(16))

	root := ui.Column(c).Fill().Background(p.Bg)
	var dragOver bool
	if !s.confirmRestore {
		if files := root.DroppedFiles(); len(files) > 0 {
			if matched := pickDroppedBookmarkFiles(files); len(matched) > 0 {
				if s.loading || s.plan != nil {
					for _, f := range matched {
						s.enqueue(f)
					}
				} else {
					s.loadFile(matched[0])
					for _, extra := range matched[1:] {
						s.enqueue(extra)
					}
				}
			} else if !s.loading && s.plan == nil {
				s.err = locales.ImportUnsupportedExt()
			} else {
				s.app.SayErr(locales.ImportUnsupportedExt(), "", nil)
			}
		}
		dragOver = root.FileDragOver()
	}

	root.Children(func() {
		ui.Row(c).Height(widget.BarH(bar)).Shrink(0).AlignItems(ui.Center).Padding(0, rightPad, 0, leftPad).Gap(10).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Line).DragWindow().Children(func() {
			backToMainButton(c, p, s.app, s.win)
			widget.Title(c, p, locales.ImportWindowTitle(), 15).FontWeight(600)
			if len(s.queue) > 0 {
				ui.Spacer(c)
				qBtn := ui.ButtonBase(c.Key("queue-next")).Height(24).PaddingX(9).Radius(999).
					AlignItems(ui.Center).Background(p.AccentBg).TextColor(p.AccentText).
					Label(locales.ImportQueueRemaining(len(s.queue)))
				if qBtn.Hovered() {
					qBtn.Background(p.Accent.Alpha(0.24))
				}
				qBtn.Children(func() {
					ui.Text(c, locales.ImportQueueRemaining(len(s.queue))).FontSize(11.5).FontWeight(500).SingleLine()
				})
				if !s.loading && !s.picking && qBtn.Clicked() {
					s.loadNextQueued()
				}
			}
		})
		s.body(c, p)
		s.footer(c, p)
		if dragOver {
			ui.Column(c).Absolute().Left(16).Top(widget.BarH(bar)+12).Right(16).Bottom(62).Center().Gap(10).
				Radius(12).Background(p.AccentBg.Alpha(0.92)).Border(2, p.Accent).BorderStyle(ui.BorderDashed).
				PassThrough().Children(func() {
				ui.Box(c).TextColor(p.AccentText).Children(func() { ui.Icon(c, theme.IconImport).FontSize(26) })
				ui.Text(c, locales.ImportDropOverlay()).FontSize(15).FontWeight(600).TextColor(p.AccentText)
			})
		}
		if confirmModal(c, p, &s.confirmRestore, locales.ImportConfirmRestoreTitle(),
			locales.ImportConfirmRestoreMsg(),
			locales.ActionCancel(), locales.ImportConfirmRestoreBtn(), true) == 1 {
			s.doRestore()
		}
		widget.Toast(c, s.app, p, 68)
	})
}

func (s *importState) body(c *ui.Context, p *theme.Palette) {
	winW, _ := c.Size()
	ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Children(func() {
		if s.plan == nil {
			borderCol, iconBg, iconFg, icon := p.Line.Mix(p.Text, 0.14), p.Field, p.Muted, theme.IconImport
			if s.err != "" {
				borderCol, iconBg, iconFg, icon = p.Danger.Alpha(0.45), p.DangerBg, p.Danger, theme.IconWarn
			}
			ui.Column(c).Width(420).MaxWidth(max(winW-48, 200)).Padding(28, 24).Gap(14).Center().
				Radius(12).Background(p.Surface).Border(1.5, borderCol).BorderStyle(ui.BorderDashed).Children(func() {
				ui.Box(c).Size(46, 46).Radius(12).Background(iconBg).Center().TextColor(iconFg).Children(func() { ui.Icon(c, icon).FontSize(22) })
				switch {
				case s.loading:
					widget.Title(c, p, locales.ImportParsing(), 16).FontWeight(600).Selectable()
					if s.loadingFile != "" {
						ui.Text(c, s.loadingFile).FontSize(12).TextColor(p.Muted).SingleLine().Ellipsis("…").Selectable()
					}
				case s.err != "":
					widget.Title(c, p, locales.ImportErrorHeading(), 16).FontWeight(600).Selectable()
					ui.Text(c, s.err).FontSize(12.5).TextColor(p.Danger).Wrap().Selectable()
					btn := widget.Primary(c.Key("repick-file"), p, locales.ImportRepickFile()).Label(locales.ImportRepickFile()).Disabled(s.picking)
					if !s.picking && btn.Clicked() {
						s.pickFile()
					}
				default:
					widget.Title(c, p, locales.ImportEmptyHeading(), 16).FontWeight(600).Selectable()
					btn := widget.Primary(c.Key("pick-file"), p, locales.ImportPickFile()).Label(locales.ImportPickFile()).
						Tooltip(locales.ImportPickFileTip()).Disabled(s.picking)
					if !s.picking && btn.Clicked() {
						s.pickFile()
					}
				}
			})
			return
		}
		plan := s.plan
		ui.Column(c).Width(460).MaxWidth(max(winW-48, 200)).Padding(18).Gap(16).Radius(12).Background(p.Surface).Border(1, p.Line).Children(func() {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(36, 36).Radius(9).Background(p.Field).Center().TextColor(p.Muted).Children(func() { ui.Icon(c, theme.IconFile).FontSize(18) })
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, filepath.Base(plan.File)).FontSize(14).FontWeight(600).TextColor(p.Text).SingleLine().Ellipsis("…").Selectable()
					kind := locales.ImportKindBrowser()
					if plan.Backup != nil {
						kind = locales.ImportKindBackup()
					}
					ui.Text(c, kind).FontSize(12).TextColor(p.Muted).SingleLine().Selectable()
				})
				changeBtn := widget.Secondary(c.Key("change-file"), p, locales.ActionChange()).Height(26).PaddingX(9).Radius(6).
					Label(locales.ActionChange()).Tooltip(locales.ImportChangeFileTip()).Disabled(s.picking || s.loading)
				if !s.picking && !s.loading && changeBtn.Clicked() {
					s.pickFile()
				}
			})
			ui.Box(c).Height(1).FillWidth().Background(p.LineSoft)
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				for _, st := range [][2]string{{locales.ImportStatFolders(), fmt.Sprintf("%d", countFolders(plan.Folders))}, {locales.ImportStatBookmarks(), fmt.Sprintf("%d", plan.Total)}, {locales.ImportStatDups(), fmt.Sprintf("%d", plan.Dups)}} {
					ui.Column(c).Grow(1).Basis(0).Padding(12, 10).Gap(4).Radius(9).Background(p.Bg).Border(1, p.LineSoft).AlignItems(ui.Center).Children(func() {
						ui.Text(c, st[1]).FontSize(18).FontWeight(600).TextColor(p.Text).SingleLine().Selectable()
						ui.Text(c, st[0]).FontSize(11.5).TextColor(p.Muted).SingleLine().Selectable()
					})
				}
			})
			if s.err != "" {
				ui.Text(c, s.err).FontSize(12).TextColor(p.Danger).Wrap().Selectable()
			}
			if plan.Total == 0 {
				msg := locales.ImportNoneAvailable()
				if plan.Backup != nil {
					msg = locales.ImportBackupEmpty()
				}
				ui.Text(c, msg).FontSize(12).TextColor(p.Muted).Wrap().Selectable()
			} else if plan.Total <= plan.Dups {
				ui.Text(c, locales.ImportAllExist()).FontSize(12).TextColor(p.Muted).Wrap().Selectable()
			}
		})
	})
}

func (s *importState) footer(c *ui.Context, p *theme.Palette) {
	ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Padding(11, 20).Gap(8).
		Background(p.Sidebar).BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(func() {
		ui.Spacer(c)
		if widget.Secondary(c.Key("import-cancel"), p, locales.ActionCancel()).Label(locales.ActionCancel()).Clicked() && s.win != nil {
			s.win.Close()
		}
		if s.plan != nil && s.plan.Backup != nil {
			canRestore := !s.loading && !s.picking
			restoreBtn := widget.Danger(c.Key("import-restore"), p, locales.ImportConfirmRestoreBtn(), false).
				Label(locales.ImportConfirmRestoreBtn()).Tooltip(locales.ImportRestoreTip()).Disabled(!canRestore)
			if canRestore && restoreBtn.Clicked() {
				s.confirmRestore = true
			}
		}
		canMerge := s.plan != nil && !s.loading && !s.picking && s.plan.Total > s.plan.Dups
		mergeBtn := widget.Primary(c.Key("import-merge"), p, locales.ImportMergeBtn()).Label(locales.ImportMergeBtn()).
			Tooltip(locales.ImportMergeTip()).Disabled(!canMerge)
		if canMerge && mergeBtn.Clicked() {
			var added []string
			s.app.Store.Update(func(l *model.Library) {
				added = s.imp().Apply(l, s.plan, app.ImportOptions{Mode: app.ImportMerge, SkipDups: true})
			})
			if len(added) == 0 {
				s.syncPlanStats()
				s.app.Say(locales.ImportNothingNew())
				return
			}
			s.finish(locales.ImportDoneCount(len(added)))
		}
	})
}

func (s *importState) doRestore() {
	if s.plan == nil || s.plan.Backup == nil {
		return
	}
	if err := s.app.Store.Replace(importer.CloneLibrary(s.plan.Backup)); err != nil {
		s.app.SayErr(locales.ImportRestoreFailed(err.Error()), "", nil)
		return
	}
	s.finish(locales.ImportRestoreDone(len(s.app.Store.Lib().Live())))
}

func (s *importState) finish(msg string) {
	s.app.EnsurePlace()
	s.app.Say(msg)
	if s.loadNextQueued() {
		return
	}
	if s.app.Win.ShowMain != nil {
		s.app.Win.ShowMain()
	}
	if s.win != nil {
		s.win.Close()
	}
}

func countFolders(folders []*app.ImportFolder) int {
	n := 0
	for _, f := range folders {
		n += 1 + countFolders(f.Children)
	}
	return n
}

func pickDroppedBookmarkFiles(files []string) []string {
	var out []string
	for _, f := range files {
		if ext := strings.ToLower(filepath.Ext(f)); ext == ".html" || ext == ".htm" || ext == ".json" {
			out = append(out, f)
		}
	}
	return out
}
