package widget

import (
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	sortStart     = 4
	sortOffsetKey = "sort-offset"
)

var (
	sortShift  = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.08}}
	sortSettle = ui.Animation{Spring: ui.SpringOptions{Duration: 280 * time.Millisecond, Bounce: 0.12}}
)

// Sorter reorders a row or column of keyed items by pressing and dragging
// one: it follows the pointer while the others slide out of its way, and
// the new order is reported once it is let go.
type Sorter struct {
	Axis ui.Orientation

	laidOut []string
	order   []string
	initial []string
	bounds  map[string]ui.Rect

	pressID string
	pending float32

	active  string
	sizes   map[string]float32
	origin  float32
	gap     float32
	from    float32
	travel  float32
	visible float32

	settling   string
	settleFrom float32
	settleNew  bool
	quietID    string
	quiet      int
	dropped    []string
}

// Order returns the order to lay the items out in for this frame: ids, or
// the order a drag in progress shows.
func (s *Sorter) Order(ids []string) []string {
	if s.active != "" && !sameItems(s.order, ids) {
		s.reset()
	}
	if s.active != "" {
		s.laidOut = slices.Clone(s.order)
	} else {
		s.laidOut = slices.Clone(ids)
	}
	return s.laidOut
}

// Active reports whether id is the item being dragged.
func (s *Sorter) Active(id string) bool { return s.active == id }

// Dropped returns the new order once a drag that moved an item ends.
func (s *Sorter) Dropped() ([]string, bool) {
	ids := s.dropped
	s.dropped = nil
	return ids, ids != nil
}

// Item lets handle drag item, whose key is id, and reports whether a click
// of handle should be ignored because the press dragged it.
func (s *Sorter) Item(id string, item, handle ui.Element) (quiet bool) {
	if s.bounds == nil {
		s.bounds = map[string]ui.Rect{}
	}
	s.bounds[id] = item.LayoutBounds()

	dx, dy, held := handle.Dragged()
	delta := dy
	if s.Axis == ui.Horizontal {
		delta = dx
	}

	began := false
	switch {
	case held && s.active == "" && (s.pressID == id || s.pressID == ""):
		s.pressID = id
		s.pending += delta
		if abs32(s.pending) >= sortStart {
			s.begin(id)
			began = true
		}
	case !held && s.pressID == id && s.active == "":
		s.pressID, s.pending = "", 0
	}

	wasActive := s.active == id
	if wasActive {
		quiet = true
		switch {
		case began:
		case held:
			s.drag(delta)
		default:
			s.end(handle.PressCanceled())
		}
	}

	switch {
	case wasActive:
		off := item.Jump(sortOffsetKey, s.visible-s.slot(s.laidOut, id))
		s.translate(item, off)
		item.ZIndex(10).Cursor(ui.CursorGrabbing)
	case s.settling == id:
		if s.settleNew {
			item.Jump(sortOffsetKey, s.settleFrom)
			s.settleNew = false
		}
		off := item.AnimateTo(sortOffsetKey, 0, sortSettle)
		s.translate(item, off)
		item.ZIndex(10)
		if abs32(off) < 0.05 {
			s.settling = ""
		}
	default:
		item.Transition(ui.ElementTransition{Position: true, Animation: &sortShift})
	}

	if s.quietID == id && s.quiet > 0 {
		quiet = true
		if s.active != id {
			s.quiet--
		}
	}
	return quiet
}

func (s *Sorter) begin(id string) {
	s.sizes = map[string]float32{}
	starts := make([]float32, 0, len(s.laidOut))
	for _, key := range s.laidOut {
		b, ok := s.bounds[key]
		if !ok || b.W == 0 && b.H == 0 {
			s.pressID, s.pending = "", 0
			return
		}
		start, size := b.Y, b.H
		if s.Axis == ui.Horizontal {
			start, size = b.X, b.W
		}
		s.sizes[key] = size
		starts = append(starts, start)
	}
	s.origin = starts[0]
	s.gap = 0
	if len(starts) > 1 {
		s.gap = max(0, starts[1]-starts[0]-s.sizes[s.laidOut[0]])
	}
	s.active, s.settling = id, ""
	s.order = slices.Clone(s.laidOut)
	s.initial = slices.Clone(s.laidOut)
	s.from = s.slot(s.order, id)
	s.travel = 0
	s.quietID, s.quiet = id, 2
	s.drag(s.pending)
}

func (s *Sorter) drag(delta float32) {
	id := s.active
	size := s.sizes[id]
	last := s.origin
	for _, key := range s.order {
		last += s.sizes[key] + s.gap
	}
	last -= s.gap + size
	s.travel += delta
	s.visible = min(max(s.from+s.travel, s.origin), last)
	s.travel = s.visible - s.from

	center := s.visible + size/2
	rest := slices.DeleteFunc(slices.Clone(s.order), func(key string) bool { return key == id })
	best, bestDist := 0, float32(math.MaxFloat32)
	pos := s.origin
	for k := 0; k <= len(rest); k++ {
		if d := abs32(pos + size/2 - center); d < bestDist {
			best, bestDist = k, d
		}
		if k < len(rest) {
			pos += s.sizes[rest[k]] + s.gap
		}
	}
	s.order = slices.Insert(rest, best, id)
}

func (s *Sorter) end(canceled bool) {
	id := s.active
	if canceled {
		s.order = slices.Clone(s.initial)
	}
	if !slices.Equal(s.order, s.initial) {
		s.dropped = slices.Clone(s.order)
	}
	s.settleFrom = s.visible - s.slot(s.order, id)
	s.settleNew = true
	s.reset()
	s.settling = id
}

func (s *Sorter) reset() {
	s.active, s.pressID, s.pending = "", "", 0
	s.order, s.initial = nil, nil
}

func (s *Sorter) slot(order []string, id string) float32 {
	pos := s.origin
	for _, key := range order {
		if key == id {
			return pos
		}
		pos += s.sizes[key] + s.gap
	}
	return pos
}

func (s *Sorter) translate(item ui.Element, off float32) {
	if s.Axis == ui.Horizontal {
		item.Translate(off, 0)
	} else {
		item.Translate(0, off)
	}
}

func sameItems(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, id := range a {
		if !slices.Contains(b, id) {
			return false
		}
	}
	return true
}
