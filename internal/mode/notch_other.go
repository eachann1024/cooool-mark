//go:build !darwin

package mode

import "github.com/egoist/mygo"

func prepareNotchWindow(*mygo.Window) {}

func displayHasNotch(mygo.Display) bool { return false }
