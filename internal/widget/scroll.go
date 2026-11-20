package widget

import (
	"time"

	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/theme"
)

type autoScrollState struct {
	scroll       ui.ScrollState
	lastX, lastY float32
	targetX      float32
	alpha        float32
	inited       bool
}

// AutoScrollbar reveals the scrollbar when the scroll position changes.
func AutoScrollbar(c *ui.Context, e ui.Element) ui.Element {
	t := c.Theme()
	return autoScrollbar(c, e, t.ScrollbarStyle.Resolve(t), false)
}

// AutoScrollbarSlim uses a smaller scrollbar for compact horizontal strips,
// translates vertical mouse-wheel scrolling into horizontal movement, and
// applies soft edge fades when tabs overflow.
func AutoScrollbarSlim(c *ui.Context, e ui.Element) ui.Element {
	t := c.Theme()
	return autoScrollbar(c, e, theme.SlimScrollbarStyle(t.Dark).Resolve(t), true)
}

// RevealSelectedTab keeps the active tab visible when a strip is opened
// or its tabs are reordered, without pulling it back during manual scrolling.
func RevealSelectedTab(tab ui.Element, selected bool, index int) {
	last := ui.Local(tab, "selected-tab-index", func() int { return -1 })
	if !selected {
		*last = -1
		return
	}
	if index != *last {
		tab.ScrollIntoView()
		*last = index
	}
}

// WheelScrollHorizontal lets vertical mouse-wheel scrolling move a horizontal
// scroll container whose scrollbars remain hidden.
func WheelScrollHorizontal(e ui.Element) ui.Element {
	st := ui.Local(e, "horizontal-wheel", func() autoScrollState { return autoScrollState{} })
	e.TrackScroll(&st.scroll)
	attachHorizontalWheel(e, st)
	return e
}

func autoScrollbar(c *ui.Context, e ui.Element, base ui.ScrollbarStyle, horizontalWheel bool) ui.Element {
	st := ui.Local(e, "auto-scrollbar", func() autoScrollState { return autoScrollState{} })
	e.TrackScroll(&st.scroll)
	if horizontalWheel {
		attachHorizontalWheel(e, st)
		if !st.inited {
			// Request one post-layout pass so MaxX and Bounds are ready for edge fade immediately.
			c.AnimationFrame()
		}
		applyHorizontalEdgeFade(e, st)
	}

	moved := false
	if st.inited {
		dx, dy := st.scroll.X-st.lastX, st.scroll.Y-st.lastY
		moved = dx*dx+dy*dy > 0.01
	}
	st.lastX, st.lastY = st.scroll.X, st.scroll.Y
	st.inited = true

	overTrack := false
	if st.alpha > 0.01 {
		px, py, over := e.PointerPosition()
		b := e.Bounds()
		hit := max(base.TrackWidth, 10)
		overTrack = over && (st.scroll.MaxY > 0.5 && px >= b.W-hit ||
			st.scroll.MaxX > 0.5 && py >= b.H-hit)
	}

	const alphaKey = "auto-scrollbar-alpha"
	if moved || overTrack {
		st.alpha = e.AnimateTo(alphaKey, 1, ui.Animation{
			Duration: 90 * time.Millisecond, Ease: ui.EaseOut, NonMotion: true,
		})
	} else {
		st.alpha = e.AnimateTo(alphaKey, 0, ui.Animation{
			Delay: 650 * time.Millisecond, Duration: 280 * time.Millisecond, Ease: ui.EaseOut, NonMotion: true,
		})
	}
	if st.alpha <= 0.01 {
		return e.Scrollbars(false)
	}

	s := base
	s.Thumb = base.Thumb.Alpha(st.alpha)
	// MyGo uses Hover as the resting color when KeepScrollbars is enabled.
	s.Hover = s.Thumb
	if overTrack {
		s.Hover = base.Hover.Alpha(st.alpha)
	}
	s.Active = base.Active.Alpha(st.alpha)
	s.Track = ui.Transparent
	return e.Scrollbars(true).KeepScrollbars(true).ScrollbarStyle(s)
}

func attachHorizontalWheel(e ui.Element, st *autoScrollState) {
	e.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll || ev.DY == 0 || st.scroll.MaxX <= 0.5 {
			return false
		}
		if ev.Mods&ui.Shift != 0 || abs32(ev.DX) >= abs32(ev.DY) {
			return false
		}
		delta := ev.DY
		if ev.Precise {
			next := max(0, min(st.scroll.X+delta, st.scroll.MaxX))
			if next == st.scroll.X && !st.scroll.Scrolling() {
				return false
			}
			st.targetX = next
			if st.scroll.Scrolling() {
				st.scroll.ScrollTo(next, st.scroll.Y, ui.Animation{Duration: time.Microsecond})
			} else {
				st.scroll.X = next
			}
			return true
		}
		baseX := st.scroll.X
		if st.scroll.Scrolling() && (delta > 0) == (st.targetX > st.scroll.X) {
			baseX = st.targetX
		}
		next := max(0, min(baseX+delta, st.scroll.MaxX))
		if next == st.scroll.X && !st.scroll.Scrolling() {
			return false
		}
		st.targetX = next
		st.scroll.ScrollTo(next, st.scroll.Y, ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.EaseOut})
		return true
	})
}

func applyHorizontalEdgeFade(e ui.Element, st *autoScrollState) {
	if st.scroll.MaxX <= 0.5 {
		return
	}
	hasLeft := st.scroll.X > 1
	hasRight := st.scroll.X < st.scroll.MaxX-1
	if !hasLeft && !hasRight {
		return
	}
	w := e.Bounds().W
	fade := float32(0.10)
	if w > 48 {
		fade = min(float32(0.16), 24/w)
	}
	opaque := ui.RGB(0, 0, 0)
	stops := make([]ui.GradientStop, 0, 4)
	if hasLeft {
		stops = append(stops, ui.GradientStop{Color: ui.Transparent, At: 0}, ui.GradientStop{Color: opaque, At: fade})
	} else {
		stops = append(stops, ui.GradientStop{Color: opaque, At: 0})
	}
	if hasRight {
		stops = append(stops, ui.GradientStop{Color: opaque, At: 1 - fade}, ui.GradientStop{Color: ui.Transparent, At: 1})
	} else {
		stops = append(stops, ui.GradientStop{Color: opaque, At: 1})
	}
	e.MaskLinearGradient(ui.LinearGradient{Angle: 90, Stops: stops})
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
