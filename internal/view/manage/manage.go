// Package manage holds the import window.
package manage

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

// ShowImport opens the import window for path ("" asks for a file). Main thread.
func ShowImport(a *app.App, path string) { showImport(a, path) }

func backToMainButton(c *ui.Context, p *theme.Palette, a *app.App, win *mygo.Window) {
	if a.Win.ShowMain == nil {
		return
	}
	b := ui.ButtonBase(c.Key("back-main")).Height(26).Padding(0, 9, 0, 5).Radius(7).Gap(3).AlignItems(ui.Center).
		TextColor(p.Text2).Label(locales.ActionBackToBookmarks()).Tooltip(locales.ActionBackToBookmarks())
	if b.Hovered() {
		b.Background(p.Hover.Mix(p.Line, 0.6))
	}
	b.Children(func() {
		ui.Icon(c, theme.IconBack).FontSize(14)
		ui.Text(c, locales.BookmarksTitle()).SingleLine()
	})
	if b.Clicked() {
		a.Win.ShowMain()
		if win != nil {
			win.Close()
		}
	}
}

func confirmModal(c *ui.Context, p *theme.Palette, open *bool, title, message, cancelLabel, confirmLabel string, danger bool) int {
	if !*open {
		return -1
	}
	chosen := -1
	winW, _ := c.Size()
	ui.DialogBase(c.Key("confirm-modal"), open, func(back, panel ui.Element) {
		back.Background(p.Overlay)
		panel.Width(392).MaxWidth(max(winW-32, 200)).Padding(20, 20, 16, 20).Gap(10).Radius(12).
			Background(p.Surface).Border(1, p.Line).
			Shadow(0, 24, 64, -16, ui.RGBA(0, 0, 0, 0.32))
		widget.Title(c, p, title, 17)
		if message != "" {
			ui.Text(c, message).TextColor(p.Text2).Wrap().Selectable()
		}
		ui.Row(c).Gap(8).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
			cancel := widget.Secondary(c.Key("modal-cancel"), p, cancelLabel).AutoFocus()
			if cancel.Clicked() {
				chosen = 0
			}
			var confirm ui.Element
			if danger {
				confirm = widget.Danger(c.Key("modal-confirm"), p, confirmLabel, true)
			} else {
				confirm = widget.Primary(c.Key("modal-confirm"), p, confirmLabel)
			}
			if confirm.Clicked() {
				chosen = 1
			}
		})
	})
	if chosen != -1 {
		*open = false
	}
	return chosen
}
