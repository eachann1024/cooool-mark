package mode

import "github.com/ebitengine/purego/objc"

// NSEventMaskLeftMouseDown | NSEventMaskRightMouseDown | NSEventMaskOtherMouseDown.
const quickSearchMouseDownMask = uintptr((1 << 1) | (1 << 3) | (1 << 25))

var (
	qsSelAddGlobalMon  = objc.RegisterName("addGlobalMonitorForEventsMatchingMask:handler:")
	qsSelAddLocalMon   = objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:")
	qsSelRemoveMonitor = objc.RegisterName("removeMonitor:")
	qsSelEventWindow   = objc.RegisterName("window")
	qsSelRetain        = objc.RegisterName("retain")
	qsSelRelease       = objc.RegisterName("release")
)

type quickSearchOutsideMonitor struct {
	globalMonitor objc.ID
	localMonitor  objc.ID
	globalBlock   objc.Block
	localBlock    objc.Block
}

func (q *quickSearchMode) startOutsideClickWatch() {
	if q.outside.globalMonitor != 0 || q.outside.localMonitor != 0 {
		return
	}
	nsEvent := objc.ID(objc.GetClass("NSEvent"))
	if nsEvent == 0 {
		return
	}
	if q.outside.globalBlock == 0 {
		q.outside.globalBlock = objc.NewBlock(func(_ objc.Block, _ objc.ID) {
			if q.win != nil && q.win.IsVisible() {
				q.hide()
			}
		})
	}
	if q.outside.localBlock == 0 {
		q.outside.localBlock = objc.NewBlock(func(_ objc.Block, ev objc.ID) objc.ID {
			if q.win != nil && q.win.IsVisible() {
				qWin := objc.ID(q.win.NativeHandle())
				var evWin objc.ID
				if ev != 0 {
					evWin = ev.Send(qsSelEventWindow)
				}
				if qWin == 0 || evWin != qWin {
					q.hide()
				}
			}
			return ev
		})
	}
	if gm := nsEvent.Send(qsSelAddGlobalMon, quickSearchMouseDownMask, uintptr(q.outside.globalBlock)); gm != 0 {
		gm.Send(qsSelRetain)
		q.outside.globalMonitor = gm
	}
	if lm := nsEvent.Send(qsSelAddLocalMon, quickSearchMouseDownMask, uintptr(q.outside.localBlock)); lm != 0 {
		lm.Send(qsSelRetain)
		q.outside.localMonitor = lm
	}
}

func (q *quickSearchMode) stopOutsideClickWatch() {
	nsEvent := objc.ID(objc.GetClass("NSEvent"))
	if m := q.outside.globalMonitor; m != 0 {
		q.outside.globalMonitor = 0
		if nsEvent != 0 {
			nsEvent.Send(qsSelRemoveMonitor, m)
		}
		m.Send(qsSelRelease)
	}
	if m := q.outside.localMonitor; m != 0 {
		q.outside.localMonitor = 0
		if nsEvent != 0 {
			nsEvent.Send(qsSelRemoveMonitor, m)
		}
		m.Send(qsSelRelease)
	}
}

func (q *quickSearchMode) releaseOutsideClickWatch() {
	q.stopOutsideClickWatch()
	if q.outside.globalBlock != 0 {
		q.outside.globalBlock.Release()
		q.outside.globalBlock = 0
	}
	if q.outside.localBlock != 0 {
		q.outside.localBlock.Release()
		q.outside.localBlock = 0
	}
}
