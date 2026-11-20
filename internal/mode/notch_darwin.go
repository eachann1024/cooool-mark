package mode

import (
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

var (
	notchStyleMask = objc.RegisterName("styleMask")
	notchSetStyle  = objc.RegisterName("setStyleMask:")
	notchSetLevel  = objc.RegisterName("setLevel:")
)

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
