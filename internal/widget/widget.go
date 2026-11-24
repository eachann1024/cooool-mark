// Package widget holds the small pieces every Cooool Mark view shares:
// icon buttons, pills, primary/secondary buttons and the toast.
package widget

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/update"
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

// ToastWithUpdate draws any active transient toast, or the persistent
// interactive update bar (with optional release-notes popover) when an update
// stage is active in the main window.
func ToastWithUpdate(c *ui.Context, a *app.App, p *theme.Palette, bottom float32) {
	if a.Toast.Text != "" {
		Toast(c, a, p, bottom)
		return
	}
	if a.Updater == nil {
		return
	}
	snap := a.Updater.Snapshot()
	if !a.UpdateBarVisible(snap) {
		return
	}
	updateBar(c, a, p, snap, bottom)
}

func updateBar(c *ui.Context, a *app.App, p *theme.Palette, snap update.Snapshot, bottom float32) {
	ui.Column(c.Key("update-bar-anchor")).Absolute().Left(0).Right(0).Bottom(bottom).
		AlignItems(ui.Center).Gap(8).PassThrough().Children(func() {
		if snap.Status == update.StatusAvailable && a.UpdateNotesOpen {
			updateNotesCard(c, a, p, snap)
		}
		updateBarPill(c, a, p, snap)
	})
}

func updateNotesCard(c *ui.Context, a *app.App, p *theme.Palette, snap update.Snapshot) {
	lines := snap.Notes.ForLanguage(locales.IsChinese()).Lines()
	if len(lines) == 0 {
		return
	}
	ver := strings.TrimPrefix(snap.LatestVersion, "v")
	ui.Column(c.Key("update-notes-pop")).Width(320).MaxWidth(360).Padding(11, 13).Gap(6).
		Radius(10).Background(p.Surface).BorderWidth(1).BorderColor(p.Line).
		Shadow(0, 12, 30, -8, ui.RGBA(0, 0, 0, 0.22)).Children(func() {
		ui.Row(c.Key("update-notes-head")).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, locales.UpdateNotesTitle(ver)).FontSize(12).FontWeight(600).
				TextColor(p.AccentText).Grow(1).MinWidth(0).SingleLine()
			closeBtn := ui.ButtonBase(c.Key("update-notes-close")).Size(20, 20).Radius(4).Center().Shrink(0).
				Label(locales.ActionClose()).Tooltip(locales.ActionClose()).TextColor(p.Muted)
			if closeBtn.Hovered() {
				closeBtn.Background(p.Hover).TextColor(p.Text)
			}
			closeBtn.Children(func() { ui.Icon(c, theme.IconClose).FontSize(11) })
			if closeBtn.Clicked() {
				a.UpdateNotesOpen = false
				a.Invalidate()
			}
		})
		bullets := make([]string, len(lines))
		for i, line := range lines {
			bullets[i] = "• " + line
		}
		AutoScrollbar(c, ui.Scroll(c.Key("update-notes-list"))).FillWidth().MaxHeight(160).Children(func() {
			ui.Text(c, strings.Join(bullets, "\n")).FontSize(12).LineHeight(1.5).
				TextColor(p.Text2).Wrap().Selectable()
		})
	})
}

func updateBarPill(c *ui.Context, a *app.App, p *theme.Palette, snap update.Snapshot) {
	bg, fg := p.ToastBg, p.ToastText
	accentBg, accentFg := theme.Dark.Primary, theme.Dark.OnPrimary
	accentHover := accentBg.Mix(ui.Hex("#ffffff"), 0.14)
	ver := strings.TrimPrefix(snap.LatestVersion, "v")

	box := ui.Row(c.Key("update-bar-pill")).AlignItems(ui.Center).Radius(10).Background(bg).
		Shadow(0, 12, 30, -8, ui.RGBA(0, 0, 0, 0.42))

	switch snap.Status {
	case update.StatusChecking:
		box.Gap(8).Padding(8, 13).Children(func() {
			ui.Spinner(c.Key("update-bar-spinner")).Size(13, 13).TextColor(accentBg).Label(locales.UpdateChecking())
			ui.Text(c, locales.UpdateChecking()).FontSize(12.5).TextColor(fg).SingleLine()
		})

	case update.StatusAvailable:
		box.Gap(8).Padding(5, 6, 5, 13).Children(func() {
			ui.Text(c, locales.UpdateBarAvailable(ver)).FontSize(12.5).TextColor(fg).SingleLine()
			primaryLabel := locales.UpdateBarNow()
			if !snap.InPlace {
				primaryLabel = locales.SettingsOpenDownload()
			}
			if toastPrimaryBtn(c, "update-bar-primary", primaryLabel, accentBg, accentHover, accentFg).Clicked() {
				a.StartUpdateDownload()
			}
			if toastGhostBtn(c, "update-bar-details", locales.UpdateBarDetails(), fg, a.UpdateNotesOpen).Clicked() {
				if len(snap.Notes.ForLanguage(locales.IsChinese()).Lines()) > 0 {
					a.UpdateNotesOpen = !a.UpdateNotesOpen
					a.Invalidate()
				} else {
					_ = a.OpenURL(snap.ReleaseURL)
				}
			}
			if toastCloseBtn(c, "update-bar-dismiss", locales.ActionClose(), fg).Clicked() {
				a.DismissUpdateBar(update.StatusAvailable, ver)
			}
		})

	case update.StatusDownloading:
		if snap.Preparing {
			box.Gap(8).Padding(8, 13).Children(func() {
				label := locales.UpdateBarVerifying(ver)
				ui.Spinner(c.Key("update-bar-spinner")).Size(13, 13).TextColor(accentBg).Label(label)
				ui.Text(c, label).FontSize(12.5).TextColor(fg).SingleLine()
			})
			return
		}
		box.Gap(9).Padding(5, 6, 5, 13).Children(func() {
			label := locales.UpdateBarDownloading(ver)
			ui.Text(c, label).FontSize(12.5).TextColor(fg).SingleLine()
			ui.Row(c.Key("update-bar-prog")).Gap(8).AlignItems(ui.Center).Children(func() {
				progVal := float64(snap.Percent) / 100
				if snap.TotalBytes <= 0 && snap.Percent == 0 {
					progVal = -1
				}
				ui.Progress(c.Key("update-bar-progress"), progVal).Width(92).Height(5).Radius(999).
					Background(fg.Alpha(0.18)).TextColor(accentBg).Label(label)
				ui.Text(c, fmt.Sprintf("%d%%", snap.Percent)).FontSize(12).TextColor(fg.Alpha(0.82)).
					MinWidth(28).SingleLine()
			})
			if toastGhostBtn(c, "update-bar-cancel", locales.ActionCancel(), fg, false).Clicked() {
				a.CancelUpdateDownload()
			}
		})

	case update.StatusReady:
		box.Gap(8).Padding(5, 6, 5, 13).Children(func() {
			ui.Text(c, locales.UpdateBarReady(ver)).FontSize(12.5).TextColor(fg).SingleLine()
			if toastPrimaryBtn(c, "update-bar-restart", locales.SettingsUpdateRestart(), accentBg, accentHover, accentFg).Clicked() {
				a.InstallUpdateNow()
			}
			if toastCloseBtn(c, "update-bar-dismiss-ready", locales.UpdateBarLater(), fg).Clicked() {
				a.DismissUpdateBar(update.StatusReady, ver)
			}
		})

	case update.StatusInstalling:
		box.Gap(8).Padding(8, 13).Children(func() {
			label := locales.UpdateBarInstalling(ver)
			ui.Spinner(c.Key("update-bar-spinner")).Size(13, 13).TextColor(accentBg).Label(label)
			ui.Text(c, label).FontSize(12.5).TextColor(fg).SingleLine()
		})
	}
}

func toastPrimaryBtn(c *ui.Context, key, label string, bg, hover, fg ui.Color) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(26).PaddingX(10).Radius(6).AlignItems(ui.Center).Shrink(0).
		Label(label).Background(bg).TextColor(fg)
	if b.Hovered() {
		b.Background(hover)
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(500).SingleLine() })
	return b
}

func toastGhostBtn(c *ui.Context, key, label string, fg ui.Color, active bool) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(26).PaddingX(10).Radius(6).AlignItems(ui.Center).Shrink(0).
		Label(label).TextColor(fg)
	if active || b.Hovered() {
		b.Background(fg.Alpha(0.22))
	} else {
		b.Background(fg.Alpha(0.13))
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(500).SingleLine() })
	return b
}

func toastCloseBtn(c *ui.Context, key, label string, fg ui.Color) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(24, 24).Radius(6).Center().Shrink(0).
		Label(label).Tooltip(label).TextColor(fg.Alpha(0.56))
	if b.Hovered() {
		b.Background(fg.Alpha(0.12)).TextColor(fg)
	}
	b.Children(func() { ui.Icon(c, theme.IconClose).FontSize(12) })
	return b
}

// TextTruncated reports whether a ui.Text element bound to h was clipped by
// SingleLine (maxLines <= 1) or MaxLines(maxLines) in its last committed
// layout. Callers must pass the same text, fontSize, weight, and maxLines used
// when building the element.
func TextTruncated(c *ui.Context, h *ui.Handle, text string, fontSize float32, weight, maxLines int) bool {
	if text == "" || h == nil {
		return false
	}
	b := h.Bounds(c)
	if b.W <= 0 {
		return false
	}
	span := ui.Span{Text: text, Size: fontSize, Weight: weight}
	if maxLines <= 1 {
		if strings.ContainsRune(text, '\n') {
			return true
		}
		fullW, _ := c.MeasureText(0, span)
		return fullW > b.W+0.5
	}
	_, oneLineH := c.MeasureText(0, ui.Span{Text: "Ag", Size: fontSize, Weight: weight})
	if oneLineH <= 0 {
		return false
	}
	_, fullH := c.MeasureText(max(b.W, 1), span)
	return fullH > float32(maxLines)*oneLineH+0.5
}
