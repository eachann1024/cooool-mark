package theme

import (
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var (
	menuIconCache     sync.Map // *ui.SVG -> ui.MenuIcon
	updateDotIconOnce sync.Once
	updateDotIcon     *mygo.MenuIcon
	updateDotSVG      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="4.5" fill="#d97757"/></svg>`))
)

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

// UpdateDotMenuIcon returns a non-template coral indicator dot for native
// application menu items when an update is available or ready to install.
func UpdateDotMenuIcon() *mygo.MenuIcon {
	updateDotIconOnce.Do(func() {
		if ic, err := mygo.NewMenuIconSVG(updateDotSVG, false); err == nil {
			updateDotIcon = ic
		}
	})
	return updateDotIcon
}
