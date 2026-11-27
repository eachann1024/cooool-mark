package settings

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/locales"
	"cooool-mark/internal/theme"
)

func (s *state) pageEngines(c *ui.Context, p *theme.Palette) {
	a := s.a
	engines := a.AllTemplates()

	card(c, p, func() {
		if len(engines) == 0 {
			ui.Row(c.Key("engines-empty")).FillWidth().Padding(24, 16).Center().Children(func() {
				ui.Text(c, locales.SettingsEnginesEmpty()).FontSize(13).TextColor(p.Muted).Wrap()
			})
			return
		}

		for i, b := range engines {
			id := b.ID
			title := strings.TrimSpace(b.Title)
			if title == "" {
				title = b.URL
			}
			kws := strings.Join(b.Keywords, " ")
			visible := !b.SearchOff
			upDisabled := i == 0
			downDisabled := i == len(engines)-1

			ui.Row(c.Key("engine-"+id)).FillWidth().Padding(10, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(24, 24).Shrink(0).Children(func() {
					theme.Tile(c, b.Icon, a.Store.IconPath(b.Icon), 24)
				})

				ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, title).FontSize(13).TextColor(p.Text).Shrink(1).MinWidth(0).SingleLine()
					if kws != "" {
						ui.Text(c, kws).FontSize(11.5).TextColor(p.Muted).Shrink(0).MaxWidth(180).SingleLine()
					}
				})

				ui.Switch(c.Key("engine-vis-"+id), &visible).Label(title).OnChange(func() {
					a.SetEngineVisible(id, visible)
				})

				ui.Row(c).Gap(2).Shrink(0).AlignItems(ui.Center).Children(func() {
					engineMoveBtn(c.Key("engine-up-"+id), p, theme.IconChevronUp, locales.ActionMoveUp(), upDisabled).
						OnClick(func() { a.MoveEngine(id, -1) })
					engineMoveBtn(c.Key("engine-down-"+id), p, theme.IconChevronDown, locales.ActionMoveDown(), downDisabled).
						OnClick(func() { a.MoveEngine(id, 1) })
				})
			})
		}
	})
}

func engineMoveBtn(c *ui.Context, p *theme.Palette, icon *ui.SVG, label string, disabled bool) ui.Element {
	b := ui.ButtonBase(c).Size(26, 26).Radius(6).Center().
		Label(label).Tooltip(label).Disabled(disabled)
	if disabled {
		b.TextColor(p.Muted.Alpha(0.35))
	} else {
		b.TextColor(p.Text2).Cursor(ui.CursorPointer)
		if b.Hovered() {
			b.Background(p.Hover).TextColor(p.Text)
		}
	}
	b.Children(func() {
		ui.Icon(c, icon).FontSize(13)
	})
	return b
}
