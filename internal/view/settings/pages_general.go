package settings

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
)

func (s *state) pageGeneral(c *ui.Context, p *theme.Palette) {
	a := s.a
	card(c, p, func() {
		segmentedRow(c, p, "外观", a.Settings.Appearance,
			[]string{"自动", "浅色", "深色"},
			[]string{"auto", "light", "dark"},
			func(v string) {
				a.Settings.Appearance = v
				a.SaveSettings()
			},
		)
		switchRow(c, p, "开机自动启动", &s.openAtLogin, func() {
			if err := mygo.App.SetOpenAtLogin(s.openAtLogin); err != nil {
				s.openAtLogin = !s.openAtLogin
				a.SayErr("无法设置开机启动", "", nil)
			}
		})
		switchRow(c, p, "启动时不显示主窗口", &a.Settings.LaunchInBackground, a.SaveSettings)
		if runtime.GOOS == "darwin" {
			switchRow(c, p, "无窗口时隐藏程序坞图标", &a.Settings.HideDockWhenClosed, a.SaveSettings)
		}
		segmentedRow(c, p, "默认视图", a.Settings.DefaultView,
			[]string{"列表", "网格"},
			[]string{"list", "grid"},
			func(v string) {
				a.Settings.DefaultView = v
				a.View = v
				a.SaveSettings()
			},
		)
		switchRow(c, p, "显示备注", &a.Settings.ShowNote, a.SaveSettings)
		switchRow(c, p, "显示标签", &a.Settings.ShowTags, a.SaveSettings)
	})

	card(c, p, func() {
		ui.Row(c).FillWidth().Padding(11, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "唤出方式").FontSize(13).FontWeight(500).TextColor(p.Text).SingleLine()
		})

		globalLabel := "全局快速搜索（Alt+Space）"
		if runtime.GOOS == "darwin" {
			globalLabel = "全局快速搜索（⌥Space）"
		}
		globalOn := a.Settings.GlobalSearchOn()
		switchRow(c, p, globalLabel, &globalOn, func() {
			a.Settings.GlobalSearchOff = !globalOn
			a.SaveSettings()
		})

		tray := a.Settings.ModeOn(model.ModeTray)
		switchRow(c, p, "菜单栏", &tray, func() {
			a.Settings.SetMode(model.ModeTray, tray)
			a.SaveSettings()
		})

		notch := a.Settings.ModeOn(model.ModeNotch)
		switchRow(c, p, "刘海", &notch, func() {
			a.Settings.SetMode(model.ModeNotch, notch)
			a.SaveSettings()
		})

		edge := a.Settings.ModeOn(model.ModeEdge)
		switchRow(c, p, "贴边", &edge, func() {
			a.Settings.SetMode(model.ModeEdge, edge)
			a.SaveSettings()
		})

		if edge {
			edgeSide := a.Settings.EdgeSide
			if edgeSide != "left" {
				edgeSide = "right"
			}
			ui.Box(c).FillWidth().Background(p.Bg.Mix(p.Surface, 0.5)).Children(func() {
				segmentedRow(c, p, "贴边位置", edgeSide,
					[]string{"左侧", "右侧"},
					[]string{"left", "right"},
					func(v string) {
						a.Settings.EdgeSide = v
						a.SaveSettings()
					},
				)
			})
		}
	})
}
