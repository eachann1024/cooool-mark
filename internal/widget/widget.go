// Package widget holds the small pieces every Cooool Mark view shares:
// icon buttons, pills, primary/secondary buttons and the toast.
package widget

import (
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/theme"
)

// IconButton is a 30×30 borderless button showing an icon, named label
// (tooltip and screen readers).
func IconButton(c *ui.Context, p *theme.Palette, icon *ui.SVG, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(30, 30).Radius(7).Center().TextColor(p.Text2)
	if b.Hovered() {
		b.Background(p.Hover.Mix(p.Line, 0.6))
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(16) })
	return b
}

// Pill is a group tab: coral tint when selected.
func Pill(c *ui.Context, p *theme.Palette, text string, selected bool) ui.Element {
	b := ui.ButtonBase(c).Height(26).MaxWidth(156).PaddingX(10).Radius(999).AlignItems(ui.Center).Shrink(0).
		Label(text).Tooltip(text)
	switch {
	case selected:
		b.Background(p.AccentBg).TextColor(p.AccentText)
	case b.Hovered():
		b.Background(p.RowSel).TextColor(p.Text)
	default:
		b.TextColor(p.Muted)
	}
	b.Children(func() {
		t := ui.Text(c, text).SingleLine()
		if selected {
			t.FontWeight(500)
		}
	})
	return b
}

// Primary is the coral main button.
func Primary(c *ui.Context, p *theme.Palette, text string) ui.Element {
	b := ui.ButtonBase(c).Height(30).PaddingX(14).Radius(8).AlignItems(ui.Center).
		Background(p.Primary).TextColor(p.OnPrimary)
	if b.Hovered() {
		b.Background(p.Primary.Mix(ui.Hex("#000000"), 0.08))
	}
	b.Children(func() { ui.Text(c, text).FontWeight(500).SingleLine() })
	return b
}

// Secondary is the quiet button.
func Secondary(c *ui.Context, p *theme.Palette, text string) ui.Element {
	b := ui.ButtonBase(c).Height(30).PaddingX(13).Radius(8).AlignItems(ui.Center).
		Background(p.Field).TextColor(p.Text)
	if b.Hovered() {
		b.Background(p.Line)
	}
	b.Children(func() { ui.Text(c, text).SingleLine() })
	return b
}

// Danger is a red text button (删除, 清空).
func Danger(c *ui.Context, p *theme.Palette, text string, filled bool) ui.Element {
	b := ui.ButtonBase(c).Height(30).PaddingX(13).Radius(8).AlignItems(ui.Center)
	if filled {
		b.Background(p.Danger).TextColor(p.OnDanger)
	} else {
		b.TextColor(p.Danger)
		if b.Hovered() {
			b.Background(p.DangerBg)
		}
	}
	b.Children(func() { ui.Text(c, text).SingleLine() })
	return b
}

// Title is a serif heading.
func Title(c *ui.Context, p *theme.Palette, text string, size float32) ui.Element {
	return ui.Text(c, text).Font(theme.TitleFont(c)).FontSize(size).FontWeight(500).TextColor(p.Text2).SingleLine()
}

// Toast draws the app's toast, centered at the bottom of the current
// parent, which must be a positioned container (call it last, so it
// paints above). It does nothing without a toast.
func Toast(c *ui.Context, a *app.App, p *theme.Palette, bottom float32) {
	t := a.Toast
	if t.Text == "" {
		return
	}
	ui.Row(c).Absolute().Left(0).Right(0).Bottom(bottom).Center().PassThrough().Children(func() {
		bg, fg := p.ToastBg, p.ToastText
		if t.Err {
			bg, fg = p.DangerBg, p.Danger
		}
		box := ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(5, 5, 5, 13).Radius(10).Background(bg).
			Shadow(0, 12, 32, -10, ui.RGBA(0, 0, 0, 0.35))
		if t.Action == "" {
			box.Padding(8, 13)
		}
		box.Children(func() {
			tEl := ui.Text(c, t.Text).TextColor(fg).SingleLine()
			if t.Err {
				tEl.Selectable()
			}
			if t.Action != "" {
				b := ui.ButtonBase(c).Height(26).PaddingX(9).Radius(6).AlignItems(ui.Center).
					Background(fg.Alpha(0.14)).TextColor(p.Accent.Mix(fg, 0.35))
				b.Children(func() { ui.Text(c, t.Action).FontWeight(500).SingleLine() })
				if b.Clicked() {
					a.ToastAction()
				}
			}
		})
	})
}
