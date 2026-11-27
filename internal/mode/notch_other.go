//go:build !darwin

package mode

import "github.com/egoist/mygo"

func prepareNotchWindow(*mygo.Window) {}

func displayHasNotch(mygo.Display) bool { return false }

func displayNotchSize(mygo.Display) (float64, float64, bool) { return 0, 0, false }
