package theme

import (
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var menuIconCache sync.Map // *ui.SVG -> ui.MenuIcon

// MenuIcon turns one of the stroke icons above into a template icon for
// ui.MenuItem.Icon. Each icon is converted once and reused. It returns nil
// (shows no icon) when the SVG cannot be drawn.
func MenuIcon(s *ui.SVG) ui.MenuIcon {
	if s == nil {
		return nil
	}
	if v, ok := menuIconCache.Load(s); ok {
		return v.(ui.MenuIcon)
	}
	ic, err := mygo.NewMenuIconSVG(s, true)
	if err != nil || ic == nil {
		return nil
	}
	v, _ := menuIconCache.LoadOrStore(s, ui.MenuIcon(ic))
	return v.(ui.MenuIcon)
}
