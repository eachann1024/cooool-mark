// Package mainwin is the compact main window with search.
package mainwin

import (
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

var (
	win   *mygo.Window
	state *mainState
)

type dragPayload struct {
	IDs         []string
	SourceGroup string
	SourceSub   string
}

type mainState struct {
	app *app.App

	// Search state.
	query       string
	searchScope string // "" = 全部, else groupID
	searchIdx   int
	searchList  ui.ListState
	searchCache struct {
		library  *model.Library
		revision int64
		query    string
		scope    string
		hits     []app.Hit
	}

	// Selection & cursor state.
	cursorID          string
	cursorScrollMoved bool
	searchScrollMoved bool

	// Scroll & navigation.
	browseList          ui.ListState
	browseScroll        ui.ScrollState
	browseScrollInited  bool
	browseFollowActive  bool
	browseFollowPending bool
	lastBrowseScrollY   float32
	lastBrowseListW     float32
	lastBrowseListH     float32
	subHeaderHandles    map[string]*ui.Handle
	lastBrowseGroup     string
	scrollToSub         string
	groupCache      struct {
		library  *model.Library
		revision int64
		groupID  string
		subs     [][]*model.Bookmark
		ids      []string
	}
	browseCache struct {
		library  *model.Library
		revision int64
		groupID  string
		view     string
		cols     int
		rows     []browseRow
		subRow   map[string]int
		bmRow    map[string]int
	}

	// Handles.
	searchHandle     ui.Handle
	contentHandle    ui.Handle
	renameHandle     ui.Handle
	newSubHandle     ui.Handle
	newGroupHandle   ui.Handle
	tplInputHandle   ui.Handle
	moveFilterHandle ui.Handle

	// Inline group/sub-group editing.
	renamingGroupID string
	renamingSubID   string
	renameDraft     string
	renameFocused   bool

	addingGroup     bool
	newGroupDraft   string
	newGroupFocused bool
	addingSub       bool
	newSubDraft     string
	newSubFocused   bool

	// Template bookmark inline query prompt.
	tplBookmarkID string
	tplQuery      string

	// "移动到…" picker modal.
	moveOpen    bool
	moveIDs     []string
	moveFilter  string
	moveIdx     int
	moveFocused bool
}

// Show opens or focuses the main window. Main thread.
func Show(a *app.App) {
	ensureState(a)
	if win != nil {
		win.Show()
		win.Focus()
		return
	}
	win = mygo.NewWindow(mygo.WindowOptions{
		Title:                "Cooool Mark",
		Width:                880,
		Height:               580,
		MinWidth:             640,
		MinHeight:            420,
		StateKey:             "main",
		TitleBarStyle:        mygo.TitleBarHiddenInset,
		TrafficLightPosition: widget.Lights(),
		TitleBarHeight:       widget.HeaderH,
		Content:              ui.View(state.view),
	})
	a.AddWindow(win)
	win.OnClosed(func() { win = nil })
}

// ShowSearch opens the main window with the search field focused and q typed.
func ShowSearch(a *app.App, q string) {
	ensureState(a)
	state.query = q
	state.searchScope = ""
	state.searchIdx = 0
	state.searchList.ScrollTo(0, ui.Start)
	state.searchHandle.SelectAll()
	Show(a)
	if win != nil {
		win.Invalidate()
	}
}

func ensureState(a *app.App) {
	if state == nil {
		state = &mainState{app: a}
	} else {
		state.app = a
	}
}

func (s *mainState) view(c *ui.Context) {
	a := s.app
	a.EnsurePlace()
	lib := a.Store.Lib()
	if s.searchScope != "" && lib.Group(s.searchScope) == nil {
		s.searchScope = ""
		s.searchIdx = 0
		s.searchList.ScrollTo(0, ui.Start)
	}
	if b := lib.Bookmark(s.tplBookmarkID); b == nil || !b.IsTemplate() {
		s.tplBookmarkID = ""
	}
	p := theme.Apply(c, a.Settings.Appearance)
	c.Root().Background(p.Bg)

	s.handleGlobalShortcuts(c)
	s.syncBrowseSubFromScroll(c, lib)

	root := ui.Row(c).Fill().Background(p.Bg).AlignItems(ui.Stretch)
	root.Children(func() {
		s.buildSidebar(c, p)

		right := ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(p.Bg)
		right.Children(func() {
			s.buildHeader(c, p)
			s.buildBody(c, p)
			widget.Toast(c, a, p, 24)
		})
	})

	// External URL / file drag-and-drop onto the main window.
	dropOpts := transfer.DropOptions{
		Formats:    []transfer.Format{transfer.URIList, transfer.Text, transfer.FileList},
		Operations: transfer.Copy,
	}
	if _, over := ui.DataDragOver(root, dropOpts); over {
		s.drawExternalDropOverlay(c, p)
	}
	if drop, ok := ui.DropData(root, dropOpts); ok {
		s.handleExternalDrop(drop)
	}

	s.buildMoveModal(c, p)
}

func (s *mainState) clearSearch() {
	s.query = ""
	s.searchScope = ""
	s.searchIdx = 0
	s.searchList.ScrollTo(0, ui.Start)
	s.tplBookmarkID = ""
}

func (s *mainState) inputFocused(c *ui.Context) bool {
	return s.searchHandle.Focused(c) ||
		s.renameHandle.Focused(c) ||
		s.newSubHandle.Focused(c) ||
		s.newGroupHandle.Focused(c) ||
		s.tplInputHandle.Focused(c) ||
		s.moveFilterHandle.Focused(c)
}

func (s *mainState) toggleView() {
	s.app.ToggleView()
}

func (s *mainState) handleGlobalShortcuts(c *ui.Context) {
	a := s.app
	if s.moveOpen {
		return
	}

	// Always active shortcuts.
	if c.Shortcut(ui.Cmd, ui.KeyN) && !c.ShortcutRepeated() {
		if a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) && !c.ShortcutRepeated() {
		s.searchHandle.SelectAll(c)
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) && !c.ShortcutRepeated() {
		if a.Win.ShowSettings != nil {
			a.Win.ShowSettings("")
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyL) && !c.ShortcutRepeated() {
		s.toggleView()
	}

	if s.inputFocused(c) {
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyV) && !c.ShortcutRepeated() && a.Win.ShowEditor != nil {
		if u := strings.TrimSpace(c.ReadClipboard()); looksLikeURL(u) {
			a.Win.ShowEditor(app.EditorRequest{URL: ensureURLScheme(u), GroupID: a.Group, SubID: a.Sub})
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyZ) && !c.ShortcutRepeated() && a.Toast.Action == "撤销" {
		a.ToastAction()
	}

	// Escape clears template prompt, search, or cursor focus.
	if c.Shortcut(0, ui.KeyEscape) {
		switch {
		case s.tplBookmarkID != "":
			s.tplBookmarkID = ""
			s.tplQuery = ""
		case s.query != "":
			s.clearSearch()
		case s.cursorID != "":
			s.cursorID = ""
		}
	}

	searching := strings.TrimSpace(s.query) != ""
	if searching {
		return
	}

	ids := s.visibleBookmarkIDs()
	if len(ids) == 0 {
		return
	}

	if c.Shortcut(0, ui.KeyDown) {
		s.moveCursor(ids, 1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		s.moveCursor(ids, -1)
	}

	if s.cursorID == "" || !slices.Contains(ids, s.cursorID) {
		return
	}

	if c.Shortcut(0, ui.KeyEnter) && !c.ShortcutRepeated() {
		if b := a.Store.Lib().Bookmark(s.cursorID); b != nil {
			s.activateBookmark(c, b)
		}
	}

	if c.Shortcut(ui.Cmd, ui.KeyE) && !c.ShortcutRepeated() {
		if a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{ID: s.cursorID})
		}
	}

	if c.Shortcut(ui.Cmd, ui.KeyM) && !c.ShortcutRepeated() {
		s.openMoveModal([]string{s.cursorID})
	}

	if c.Shortcut(ui.Cmd, ui.KeyC) && !c.ShortcutRepeated() {
		s.copyBookmarkURL(c, s.cursorID)
	}

	if (c.Shortcut(0, ui.KeyBackspace) || c.Shortcut(0, ui.KeyDelete)) && !c.ShortcutRepeated() {
		a.Delete(s.cursorID)
	}
}

func (s *mainState) moveCursor(ids []string, delta int) {
	a := s.app
	idx := slices.Index(ids, s.cursorID)
	if idx < 0 {
		if delta > 0 {
			idx = 0
		} else {
			idx = len(ids) - 1
		}
	} else {
		idx = max(0, min(idx+delta, len(ids)-1))
	}
	target := ids[idx]
	s.cursorID = target
	s.cursorScrollMoved = true
	s.stopBrowseFollow()
	if b := a.Store.Lib().Bookmark(target); b != nil && b.SubID != "" {
		a.SetSub(b.SubID)
	}
}

func (s *mainState) drawExternalDropOverlay(c *ui.Context, p *theme.Palette) {
	a := s.app
	lib := a.Store.Lib()
	targetName := "收藏夹"
	if g := lib.Group(a.Group); g != nil {
		targetName = g.Name
		if sub := lib.Sub(g.ID, a.Sub); sub != nil {
			targetName += " / " + sub.Name
		}
	}
	ui.Column(c).Absolute().Left(196).Top(58).Right(12).Bottom(12).
		Radius(12).Background(p.AccentBg.Alpha(0.92)).Border(2, p.Accent).BorderStyle(ui.BorderDashed).
		Center().PassThrough().Children(func() {
		ui.Text(c, "松手保存到「"+targetName+"」").FontSize(14).FontWeight(600).TextColor(p.AccentText)
	})
}

func (s *mainState) handleExternalDrop(drop transfer.Drop) {
	a := s.app
	if paths, err := drop.Data.Files(); err == nil && len(paths) > 0 {
		for _, path := range paths {
			low := strings.ToLower(path)
			if strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".htm") || strings.HasSuffix(low, ".json") {
				if a.Win.ShowImport != nil {
					a.Win.ShowImport(path)
				}
				return
			}
		}
	}
	if urls, err := drop.Data.URLs(); err == nil && len(urls) > 0 {
		if a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{URL: urls[0], GroupID: a.Group, SubID: a.Sub})
		}
		return
	}
	if raw, err := drop.Data.Read(transfer.Text); err == nil {
		txt := strings.TrimSpace(string(raw))
		if looksLikeURL(txt) && a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{URL: ensureURLScheme(txt), GroupID: a.Group, SubID: a.Sub})
		}
	}
}
