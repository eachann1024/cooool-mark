package widget

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// HeaderH is the height of every window's top bar, the band the window
// controls sit centered in.
const HeaderH = 46

// Lights puts the macOS traffic lights centered in a HeaderH-tall band
// (WindowOptions.TrafficLightPosition, with TitleBarHeight: HeaderH for
// the window buttons on Windows and Linux).
func Lights() *mygo.Point { return &mygo.Point{X: 18, Y: 16} }

// BarH is the height of a top bar that lines up with the window controls:
// the controls' band, or HeaderH where there are none (full screen).
func BarH(bar ui.TitleBar) float32 {
	if bar.Height > 0 {
		return bar.Height
	}
	return HeaderH
}

// Header is a draggable top bar of BarH, its content vertically centered
// with the window controls. left and right are the gaps kept from the
// window's edges, or from the controls where they are.
func Header(c *ui.Context, bar ui.TitleBar, left, right float32) ui.Element {
	l, r := left, right
	if bar.Left > 0 {
		l = bar.Left + 12
	}
	if bar.Right > 0 {
		r = bar.Right + 8
	}
	return ui.Row(c).Height(BarH(bar)).Shrink(0).FillWidth().AlignItems(ui.Center).
		Padding(0, r, 0, l).DragWindow()
}
