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
	return autoScrollbar(c, e, t.ScrollbarStyle.Resolve(t), false, nil)
}

// AutoScrollbarState is AutoScrollbar backed by a caller-owned ScrollState.
func AutoScrollbarState(c *ui.Context, e ui.Element, scroll *ui.ScrollState) ui.Element {
	t := c.Theme()
	return autoScrollbar(c, e, t.ScrollbarStyle.Resolve(t), false, scroll)
}

// AutoScrollbarSlim uses a smaller scrollbar for compact horizontal strips,
// translates vertical mouse-wheel scrolling into horizontal movement, and
// applies soft edge fades when tabs overflow.
func AutoScrollbarSlim(c *ui.Context, e ui.Element) ui.Element {
	return AutoScrollbarSlimState(c, e, nil)
}

// AutoScrollbarSlimState is AutoScrollbarSlim backed by a caller-owned ScrollState.
func AutoScrollbarSlimState(c *ui.Context, e ui.Element, scroll *ui.ScrollState) ui.Element {
	t := c.Theme()
	return autoScrollbar(c, e, theme.SlimScrollbarStyle(t.Dark).Resolve(t), true, scroll)
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
	attachHorizontalWheel(e, st, &st.scroll)
	return e
}

func autoScrollbar(c *ui.Context, e ui.Element, base ui.ScrollbarStyle, horizontalWheel bool, external *ui.ScrollState) ui.Element {
	st := ui.Local(e, "auto-scrollbar", func() autoScrollState { return autoScrollState{} })
	scroll := &st.scroll
	if external != nil {
		scroll = external
	}
	e.TrackScroll(scroll)
	if horizontalWheel {
		attachHorizontalWheel(e, st, scroll)
		if !st.inited {
			// Request one post-layout pass so MaxX and Bounds are ready for edge fade immediately.
			c.AnimationFrame()
		}
		applyHorizontalEdgeFade(e, scroll)
	}

	moved := false
	if st.inited {
		dx, dy := scroll.X-st.lastX, scroll.Y-st.lastY
		moved = dx*dx+dy*dy > 0.01
	}
	st.lastX, st.lastY = scroll.X, scroll.Y
	st.inited = true

	overTrack := false
	if st.alpha > 0.01 {
		px, py, over := e.PointerPosition()
		b := e.Bounds()
		hit := max(base.TrackWidth, 10)
		overTrack = over && (scroll.MaxY > 0.5 && px >= b.W-hit ||
			scroll.MaxX > 0.5 && py >= b.H-hit)
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

// ScrollHorizontalWheel forwards scroll events on e (such as a wider header
// bar around a horizontal strip) to scroll horizontally. When passthrough is
// valid and hovered, native horizontal / Shift+wheel scrolling is left to it.
func ScrollHorizontalWheel(e ui.Element, scroll *ui.ScrollState, passthrough ui.Element) ui.Element {
	st := ui.Local(e, "horizontal-wheel-proxy", func() autoScrollState { return autoScrollState{} })
	e.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll || scroll == nil || scroll.MaxX <= 0.5 {
			return false
		}
		if passthrough.Valid() && passthrough.Hovered() && (ev.Mods&ui.Shift != 0 || abs32(ev.DX) >= abs32(ev.DY)) {
			return false
		}
		delta := ev.DY
		if abs32(ev.DX) > abs32(ev.DY) {
			delta = ev.DX
		}
		if delta == 0 {
			return false
		}
		return scrollHorizontalDelta(st, scroll, delta, ev.Precise)
	})
	return e
}

func attachHorizontalWheel(e ui.Element, st *autoScrollState, scroll *ui.ScrollState) {
	e.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll || ev.DY == 0 || scroll.MaxX <= 0.5 {
			return false
		}
		if ev.Mods&ui.Shift != 0 || abs32(ev.DX) >= abs32(ev.DY) {
			return false
		}
		return scrollHorizontalDelta(st, scroll, ev.DY, ev.Precise)
	})
}

func scrollHorizontalDelta(st *autoScrollState, scroll *ui.ScrollState, delta float32, precise bool) bool {
	if precise {
		next := max(0, min(scroll.X+delta, scroll.MaxX))
		if next == scroll.X && !scroll.Scrolling() {
			return false
		}
		st.targetX = next
		if scroll.Scrolling() {
			scroll.ScrollTo(next, scroll.Y, ui.Animation{Duration: time.Microsecond})
		} else {
			scroll.X = next
		}
		return true
	}
	baseX := scroll.X
	if scroll.Scrolling() && (delta > 0) == (st.targetX > scroll.X) {
		baseX = st.targetX
	}
	next := max(0, min(baseX+delta, scroll.MaxX))
	if next == scroll.X && !scroll.Scrolling() {
		return false
	}
	st.targetX = next
	scroll.ScrollTo(next, scroll.Y, ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.EaseOut})
	return true
}

func applyHorizontalEdgeFade(e ui.Element, scroll *ui.ScrollState) {
	if scroll.MaxX <= 0.5 {
		return
	}
	hasLeft := scroll.X > 1
	hasRight := scroll.X < scroll.MaxX-1
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
