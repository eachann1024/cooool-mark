package mode

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type nsEdgeInsets struct {
	Top, Left, Bottom, Right float64
}

type nsRect struct {
	X, Y, W, H float64
}

var (
	notchStyleMask = objc.RegisterName("styleMask")
	notchSetStyle  = objc.RegisterName("setStyleMask:")
	notchSetLevel  = objc.RegisterName("setLevel:")

	notchScreenOnce   sync.Once
	selScreens        objc.SEL
	selCount          objc.SEL
	selObjectAtIndex  objc.SEL
	selDeviceDesc     objc.SEL
	selObjectForKey   objc.SEL
	selUnsignedInt    objc.SEL
	selRespondsTo     objc.SEL
	selSafeAreaInsets objc.SEL
	nsScreenNumKey    objc.ID
	selAuxLeft        objc.SEL
	selAuxRight       objc.SEL
	selFrame          objc.SEL
	msgEdgeInsets     func(objc.ID, objc.SEL) nsEdgeInsets
	msgRect           func(objc.ID, objc.SEL) nsRect
)

func initNotchScreen() {
	notchScreenOnce.Do(func() {
		libObjC, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			return
		}
		msgSendAddr, err := purego.Dlsym(libObjC, "objc_msgSend")
		if err != nil {
			return
		}
		stret := msgSendAddr
		if runtime.GOARCH == "amd64" {
			if s, err := purego.Dlsym(libObjC, "objc_msgSend_stret"); err == nil {
				stret = s
			}
		}
		purego.RegisterFunc(&msgEdgeInsets, stret)
		purego.RegisterFunc(&msgRect, stret)
		selAuxLeft = objc.RegisterName("auxiliaryTopLeftArea")
		selAuxRight = objc.RegisterName("auxiliaryTopRightArea")
		selFrame = objc.RegisterName("frame")
		selScreens = objc.RegisterName("screens")
		selCount = objc.RegisterName("count")
		selObjectAtIndex = objc.RegisterName("objectAtIndex:")
		selDeviceDesc = objc.RegisterName("deviceDescription")
		selObjectForKey = objc.RegisterName("objectForKey:")
		selUnsignedInt = objc.RegisterName("unsignedIntValue")
		selRespondsTo = objc.RegisterName("respondsToSelector:")
		selSafeAreaInsets = objc.RegisterName("safeAreaInsets")
		nsScreenNumKey = objc.ID(objc.GetClass("NSString")).
			Send(objc.RegisterName("alloc")).
			Send(objc.RegisterName("initWithUTF8String:"), "NSScreenNumber")
	})
}

func displayHasNotch(d mygo.Display) bool {
	_, _, ok := displayNotchSize(d)
	return ok
}

// displayNotchSize returns the camera housing size in points for the display's
// current scaled resolution; ok is false when d has no notch.
func displayNotchSize(d mygo.Display) (w, h float64, ok bool) {
	if d.ID == 0 {
		return 0, 0, false
	}
	initNotchScreen()
	if msgEdgeInsets == nil || nsScreenNumKey == 0 {
		return 0, 0, false
	}
	cls := objc.GetClass("NSScreen")
	if cls == 0 {
		return 0, 0, false
	}
	screens := objc.ID(cls).Send(selScreens)
	if screens == 0 {
		return 0, 0, false
	}
	count := int(screens.Send(selCount))
	for i := 0; i < count; i++ {
		s := screens.Send(selObjectAtIndex, uintptr(i))
		if s == 0 {
			continue
		}
		desc := s.Send(selDeviceDesc)
		if desc == 0 {
			continue
		}
		num := desc.Send(selObjectForKey, uintptr(nsScreenNumKey))
		if num == 0 {
			continue
		}
		displayID := uint32(num.Send(selUnsignedInt))
		if int64(displayID) != d.ID {
			continue
		}
		if s.Send(selRespondsTo, uintptr(selSafeAreaInsets)) == 0 {
			return 0, 0, false
		}
		insets := msgEdgeInsets(s, selSafeAreaInsets)
		if insets.Top <= 0 {
			return 0, 0, false
		}
		h = insets.Top
		w = 0
		if s.Send(selRespondsTo, uintptr(selAuxLeft)) != 0 && s.Send(selRespondsTo, uintptr(selAuxRight)) != 0 {
			frame := msgRect(s, selFrame)
			l := msgRect(s, selAuxLeft)
			r := msgRect(s, selAuxRight)
			w = frame.W - l.W - r.W
		}
		if w <= 0 || w > 400 {
			w = 200
		}
		return w, h, true
	}
	return 0, 0, false
}



func prepareNotchWindow(w *mygo.Window) {
	const titled = 1 << 0
	const statusWindowLevel = 25
	window := objc.ID(w.NativeHandle())
	if window == 0 {
		return
	}
	// MyGo hides the chrome on frameless windows but leaves this style on.
	// AppKit constrains titled windows below the menu bar.
	mask := uintptr(window.Send(notchStyleMask))
	window.Send(notchSetStyle, mask&^titled)
	window.Send(notchSetLevel, statusWindowLevel)
}
