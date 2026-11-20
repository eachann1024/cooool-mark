package manage

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/importer"
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
	picking        bool
	loadSeq        int
	err            string
	confirmRestore bool
}

var (
	importWin   *mygo.Window
	importModel *importState
)

func showImport(a *app.App, path string) {
	if importWin != nil {
		importWin.Show()
		importWin.Focus()
		importWin.Invalidate()
		if path != "" && importModel != nil {
			importModel.loadFile(path)
		} else if importModel != nil && importModel.plan == nil && !importModel.loading && !importModel.picking && importModel.err == "" {
			importModel.pickFile()
		}
		return
	}
	st := &importState{app: a}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: "导入与恢复", Width: 540, Height: 420, MinWidth: 440, MinHeight: 340,
		StateKey: "manage-import", TitleBarStyle: mygo.TitleBarHiddenInset,
		TrafficLightPosition: widget.Lights(), TitleBarHeight: widget.HeaderH,
		Content: ui.View(st.view),
	})
	st.win, importWin, importModel = w, w, st
	a.AddWindow(w)
	w.OnClosed(func() {
		st.closed = true
		st.loadSeq++
		st.loading = false
		st.loadingFile = ""
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
	s.invalidate()
	win := s.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: win, Title: "选择书签文件",
			Filters: []mygo.FileFilter{{Name: "书签或备份文件", Extensions: []string{"html", "htm", "json"}}},
		})
		s.app.Post(func() {
			if s.closed || (importModel != nil && importModel != s) {
				return
			}
			s.picking = false
			if err != nil {
				s.err = "选择文件失败：" + err.Error()
			} else if len(paths) > 0 {
				s.loadFile(paths[0])
			}
		})
	}()
}

func (s *importState) loadFile(path string) {
	s.loadSeq++
	seq := s.loadSeq
	s.plan = nil
	s.planRev = 0
	s.confirmRestore = false
	s.loading = true
	s.loadingFile = filepath.Base(path)
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
			if err != nil {
				s.plan, s.err = nil, "读取失败："+err.Error()
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
	bar := c.TitleBar()
	leftPad, rightPad := max(bar.Left+12, float32(16)), max(bar.Right+16, float32(16))

	root := ui.Column(c).Fill().Background(p.Bg)
	var dragOver bool
	if !s.loading && !s.picking && !s.confirmRestore {
		if files := root.DroppedFiles(); len(files) > 0 {
			if f := pickDroppedBookmarkFile(files); f != "" {
				s.loadFile(f)
			} else {
				s.plan = nil
				s.planRev = 0
				s.err = "仅支持导入 .html、.htm 或 .json 文件"
			}
		}
		dragOver = root.FileDragOver()
	}

	root.Children(func() {
		ui.Row(c).Height(widget.BarH(bar)).Shrink(0).AlignItems(ui.Center).Padding(0, rightPad, 0, leftPad).Gap(10).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Line).DragWindow().Children(func() {
			backToMainButton(c, p, s.app, s.win)
			widget.Title(c, p, "导入与恢复", 15).FontWeight(600)
		})
		s.body(c, p)
		s.footer(c, p)
		if dragOver {
			ui.Column(c).Absolute().Left(16).Top(widget.BarH(bar)+12).Right(16).Bottom(62).Center().Gap(10).
				Radius(12).Background(p.AccentBg.Alpha(0.92)).Border(2, p.Accent).BorderStyle(ui.BorderDashed).
				PassThrough().Children(func() {
				ui.Box(c).TextColor(p.AccentText).Children(func() { ui.Icon(c, theme.IconImport).FontSize(26) })
				ui.Text(c, "松手读取书签文件").FontSize(15).FontWeight(600).TextColor(p.AccentText)
			})
		}
		if confirmModal(c, p, &s.confirmRestore, "确认覆盖恢复全部数据？",
			"覆盖前会自动备份当前书签到 backups 目录，此操作将替换现有分组与书签。",
			"取消", "覆盖恢复", true) == 1 {
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
					widget.Title(c, p, "正在解析书签文件…", 16).FontWeight(600)
					if s.loadingFile != "" {
						ui.Text(c, s.loadingFile).FontSize(12).TextColor(p.Muted).SingleLine().Ellipsis("…")
					}
				case s.err != "":
					widget.Title(c, p, "读取文件失败", 16).FontWeight(600)
					ui.Text(c, s.err).FontSize(12.5).TextColor(p.Danger).Wrap()
					btn := widget.Primary(c.Key("repick-file"), p, "重新选择文件").Label("重新选择文件").Disabled(s.picking)
					if !s.picking && btn.Clicked() {
						s.pickFile()
					}
				default:
					widget.Title(c, p, "拖入书签文件，或选择文件", 16).FontWeight(600)
					btn := widget.Primary(c.Key("pick-file"), p, "选择文件").Label("选择文件").
						Tooltip("选择 .html / .htm / .json 文件").Disabled(s.picking)
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
					ui.Text(c, filepath.Base(plan.File)).FontSize(14).FontWeight(600).TextColor(p.Text).SingleLine().Ellipsis("…")
					kind := "浏览器书签文件"
					if plan.Backup != nil {
						kind = "Cooool Mark JSON 备份"
					}
					ui.Text(c, kind).FontSize(12).TextColor(p.Muted).SingleLine()
				})
				changeBtn := widget.Secondary(c.Key("change-file"), p, "换一个").Height(26).PaddingX(9).Radius(6).
					Label("更换文件").Tooltip("重新选择书签或备份文件").Disabled(s.picking || s.loading)
				if !s.picking && !s.loading && changeBtn.Clicked() {
					s.pickFile()
				}
			})
			ui.Box(c).Height(1).FillWidth().Background(p.LineSoft)
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				for _, st := range [][2]string{{"文件夹数", fmt.Sprintf("%d", countFolders(plan.Folders))}, {"书签数", fmt.Sprintf("%d", plan.Total)}, {"重复数", fmt.Sprintf("%d", plan.Dups)}} {
					ui.Column(c).Grow(1).Basis(0).Padding(12, 10).Gap(4).Radius(9).Background(p.Bg).Border(1, p.LineSoft).AlignItems(ui.Center).Children(func() {
						ui.Text(c, st[1]).FontSize(18).FontWeight(600).TextColor(p.Text).SingleLine()
						ui.Text(c, st[0]).FontSize(11.5).TextColor(p.Muted).SingleLine()
					})
				}
			})
			if s.err != "" {
				ui.Text(c, s.err).FontSize(12).TextColor(p.Danger).Wrap()
			}
			if plan.Total == 0 {
				msg := "此文件中没有可导入的书签"
				if plan.Backup != nil {
					msg = "此备份不包含书签，仍可覆盖恢复或更换文件"
				}
				ui.Text(c, msg).FontSize(12).TextColor(p.Muted).Wrap()
			} else if plan.Total <= plan.Dups {
				ui.Text(c, "所有书签均已存在于书签库中，无需重复导入").FontSize(12).TextColor(p.Muted).Wrap()
			}
		})
	})
}

func (s *importState) footer(c *ui.Context, p *theme.Palette) {
	ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Padding(11, 20).Gap(8).
		Background(p.Sidebar).BorderWidth(1, 0, 0, 0).BorderColor(p.Line).Children(func() {
		ui.Spacer(c)
		if widget.Secondary(c.Key("import-cancel"), p, "取消").Label("取消").Clicked() && s.win != nil {
			s.win.Close()
		}
		if s.plan != nil && s.plan.Backup != nil {
			canRestore := !s.loading && !s.picking
			restoreBtn := widget.Danger(c.Key("import-restore"), p, "覆盖恢复", false).
				Label("覆盖恢复").Tooltip("用此备份覆盖恢复全部数据").Disabled(!canRestore)
			if canRestore && restoreBtn.Clicked() {
				s.confirmRestore = true
			}
		}
		canMerge := s.plan != nil && !s.loading && !s.picking && s.plan.Total > s.plan.Dups
		mergeBtn := widget.Primary(c.Key("import-merge"), p, "导入（自动去重合并）").Label("导入（自动去重合并）").
			Tooltip("同名分组自动合并并跳过重复网址").Disabled(!canMerge)
		if canMerge && mergeBtn.Clicked() {
			var added []string
			s.app.Store.Update(func(l *model.Library) {
				added = s.imp().Apply(l, s.plan, app.ImportOptions{Mode: app.ImportMerge, SkipDups: true})
			})
			if len(added) == 0 {
				s.syncPlanStats()
				s.app.Say("没有新的书签可导入")
				return
			}
			s.finish(fmt.Sprintf("已导入 %d 个书签", len(added)))
		}
	})
}

func (s *importState) doRestore() {
	if s.plan == nil || s.plan.Backup == nil {
		return
	}
	if err := s.app.Store.Replace(importer.CloneLibrary(s.plan.Backup)); err != nil {
		s.app.SayErr("恢复失败："+err.Error(), "", nil)
		return
	}
	s.finish(fmt.Sprintf("已恢复 %d 个书签", len(s.app.Store.Lib().Live())))
}

func (s *importState) finish(msg string) {
	s.app.EnsurePlace()
	s.app.Say(msg)
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

func pickDroppedBookmarkFile(files []string) string {
	for _, f := range files {
		if ext := strings.ToLower(filepath.Ext(f)); ext == ".html" || ext == ".htm" || ext == ".json" {
			return f
		}
	}
	return ""
}
