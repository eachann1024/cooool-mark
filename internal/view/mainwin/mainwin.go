// Package mainwin is the compact main window with search.
package mainwin

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
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
	pal        *theme.Palette
	lastOpenID string
	lastOpenAt time.Time

	app *app.App

	// Search state.
	searchOpen     bool
	query          string
	lockedEngineID string
	lockedKeyword  string
	searchScope    string // "" = 全部, else groupID
	searchIdx      int
	searchList     ui.ListState
	searchCache struct {
		library  *model.Library
		revision int64
		query    string
		scope    string
		hits     []app.Hit
	}

	// Selection & cursor state.
	cursorID          string
	selectedIDs       map[string]bool
	cursorScrollMoved bool
	searchScrollMoved bool

	// Scroll & navigation.
	subTabsScroll       ui.ScrollState
	groupSort           widget.Sorter
	subSort             widget.Sorter
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
	groupCache          struct {
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
	searchHandle   ui.Handle
	headerMoreOpen bool
	// refocusHome asks the next build to put the focus back home (the
	// search field while searching), e.g. after the window is reactivated.
	refocusHome      bool
	moveWasOpen      bool
	contentHandle    ui.Handle
	renameHandle     ui.Handle
	newSubHandle     ui.Handle
	newGroupHandle   ui.Handle
	tplInputHandle   ui.Handle
	moveFilterHandle ui.Handle

	// pendingSearchCaret places the search field's caret after a paste from
	// its menu, once the field shows the new text. Negative means none.
	pendingSearchCaret int

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

// IsOpen reports whether the main window is currently open.
func IsOpen() bool {
	return win != nil && !win.IsDestroyed()
}

// RefreshChrome invalidates the main window when open (e.g. after locale changes).
func RefreshChrome() {
	if win != nil && !win.IsDestroyed() {
		win.Invalidate()
	}
}

// Show opens or focuses the main window. Main thread.
func Show(a *app.App) {
	a.PrepareRegularWindow()
	ensureState(a)
	if win != nil {
		win.Show()
		win.Focus()
		if !state.moveOpen {
			if state.searchOpen {
				state.searchHandle.SelectAll()
			} else {
				state.contentHandle.Focus()
			}
		}
		return
	}
	win = mygo.NewWindow(mygo.WindowOptions{
		Title:                locales.AppName(),
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
	a.AddRegularWindow(win)
	win.OnClose(func(*mygo.CloseEvent) {
		a.FlushSettings()
	})
	win.OnClosed(func() { win = nil })
	w := win
	win.OnFocus(func() {
		if state == nil || w.IsDestroyed() {
			return
		}
		state.refocusHome = true
		w.Invalidate()
	})
	if state.searchOpen && !state.moveOpen {
		state.searchHandle.SelectAll()
	} else {
		state.contentHandle.Focus()
	}
}

// FocusSearch opens or focuses the main window search overlay while preserving
// any existing search query if search is already open (for native menu Find).
func FocusSearch(a *app.App) {
	ensureState(a)
	state.moveOpen = false
	if state.searchOpen {
		Show(a)
		state.searchHandle.SelectAll()
		if win != nil {
			win.Invalidate()
		}
		return
	}
	ShowSearch(a, "")
}

// ShowSearch opens the main window with the search overlay focused and q typed.
func ShowSearch(a *app.App, q string) {
	ensureState(a)
	state.moveOpen = false
	state.searchOpen = true
	state.query = q
	state.lockedEngineID = ""
	state.lockedKeyword = ""
	state.searchScope = ""
	state.searchIdx = 0
	state.tplBookmarkID = ""
	state.tplQuery = ""
	state.pendingSearchCaret = -1
	state.searchList.ScrollTo(0, ui.Start)
	Show(a)
	if q != "" {
		state.searchHandle.SelectAll()
	} else {
		state.searchHandle.Focus()
	}
	if win != nil {
		win.Invalidate()
	}
}

func ensureState(a *app.App) {
	if state == nil {
		state = &mainState{
			app:                a,
			pendingSearchCaret: -1,
			selectedIDs:        make(map[string]bool),
		}
	} else {
		state.app = a
		if state.selectedIDs == nil {
			state.selectedIDs = make(map[string]bool)
		}
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
	s.pruneSelection(lib)
	p := theme.Apply(c, a.Settings.Appearance)
	s.pal = p
	c.Root().Background(p.Bg)

	s.handleGlobalShortcuts(c)
	s.syncBrowseSubFromScroll(c, lib)

	root := ui.Row(c).Fill().Background(p.Bg).AlignItems(ui.Stretch)
	root.Children(func() {
		if !s.searchOpen {
			s.buildSidebar(c, p)
		}

		right := ui.Column(c.Key("main-right")).Grow(1).MinWidth(0).FillHeight().Background(p.Bg)
		right.Children(func() {
			if s.searchOpen {
				s.buildSearchInline(c, p)
			} else {
				s.buildHeader(c, p)
				s.buildBody(c, p)
			}
			widget.ToastWithUpdate(c, a, p, 24)
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

	if s.moveWasOpen && !s.moveOpen {
		s.refocusHome = true
	}
	s.moveWasOpen = s.moveOpen
	if s.refocusHome {
		s.refocusHome = false
		if !s.moveOpen && (s.searchOpen || !s.inputFocused(c)) {
			s.focusHome(c)
		}
	}
}

// focusHome puts the keyboard focus where the main view expects it: the
// search field while searching, so Escape always ends the search, and the
// bookmark area otherwise, where typing starts a search.
func (s *mainState) focusHome(c *ui.Context) {
	if s.searchOpen {
		s.searchHandle.Focus(c)
	} else {
		s.contentHandle.Focus(c)
	}
}

func (s *mainState) openSearch(c *ui.Context, initial string, selectAll bool) {
	s.searchOpen = true
	s.query = initial
	s.lockedEngineID = ""
	s.lockedKeyword = ""
	s.searchScope = ""
	s.searchIdx = 0
	s.searchScrollMoved = false
	s.tplBookmarkID = ""
	s.tplQuery = ""
	s.pendingSearchCaret = -1
	s.searchList.ScrollTo(0, ui.Start)
	if selectAll && initial != "" {
		s.searchHandle.SelectAll(c)
	} else {
		s.searchHandle.Focus(c)
	}
}

func (s *mainState) closeSearch(c *ui.Context) {
	s.searchOpen = false
	s.clearSearch()
	if c != nil {
		s.contentHandle.Focus(c)
	}
}

func (s *mainState) clearSearch() {
	s.query = ""
	s.lockedEngineID = ""
	s.lockedKeyword = ""
	s.searchScope = ""
	s.searchIdx = 0
	s.searchList.ScrollTo(0, ui.Start)
	s.tplBookmarkID = ""
	s.tplQuery = ""
	s.pendingSearchCaret = -1
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
		if s.searchOpen {
			s.searchHandle.SelectAll(c)
		} else {
			s.openSearch(c, "", false)
		}
		return
	}
	if s.searchOpen {
		if c.Shortcut(0, ui.KeyEscape) && !s.searchHandle.Focused(c) {
			s.handleSearchEscape(c)
		}
		return
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
	if a.UpdateNotesOpen && c.Shortcut(0, ui.KeyEscape) {
		a.UpdateNotesOpen = false
		return
	}
	lib := a.Store.Lib()
	for i := 0; i < 9; i++ {
		if c.Shortcut(ui.Cmd, ui.Key1+ui.Key(i)) && !c.ShortcutRepeated() {
			if i < len(lib.Groups) {
				s.switchGroup(lib.Groups[i].ID)
			}
			return
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketLeft) && !c.ShortcutRepeated() {
		s.stepGroup(-1)
		return
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketRight) && !c.ShortcutRepeated() {
		s.stepGroup(1)
		return
	}
	if ch, ok := widget.TypedSearchKey(c); ok {
		s.openSearch(c, ch, false)
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyV) && !c.ShortcutRepeated() && a.Win.ShowEditor != nil {
		if u := strings.TrimSpace(c.ReadClipboard()); looksLikeURL(u) {
			a.Win.ShowEditor(app.EditorRequest{URL: ensureURLScheme(u), GroupID: a.Group, SubID: a.Sub})
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyZ) && !c.ShortcutRepeated() && a.Toast.Undoable {
		a.ToastAction()
	}

	// Escape clears template prompt or selection/cursor focus.
	if c.Shortcut(0, ui.KeyEscape) {
		switch {
		case s.tplBookmarkID != "":
			s.tplBookmarkID = ""
			s.tplQuery = ""
		case len(s.selectedIDs) > 0 || s.cursorID != "":
			s.clearSelection()
			s.cursorID = ""
		}
	}

	ids := s.visibleBookmarkIDs()
	if len(ids) == 0 {
		return
	}

	grid := a.View == "grid"
	if c.Shortcut(0, ui.KeyDown) {
		if !grid || !s.moveGridRow(ids, 1) {
			s.moveCursor(ids, 1)
		}
	}
	if c.Shortcut(0, ui.KeyUp) {
		if !grid || !s.moveGridRow(ids, -1) {
			s.moveCursor(ids, -1)
		}
	}
	if grid && c.Shortcut(0, ui.KeyRight) {
		s.moveCursor(ids, 1)
	}
	if grid && c.Shortcut(0, ui.KeyLeft) {
		s.moveCursor(ids, -1)
	}

	sel := s.selectedList()
	if len(sel) == 0 {
		return
	}

	if c.Shortcut(0, ui.KeyEnter) && !c.ShortcutRepeated() {
		if len(sel) > 1 {
			s.openBookmarks(c, sel)
		} else if b := lib.Bookmark(sel[0]); b != nil {
			s.activateBookmark(c, b)
		}
	}

	if c.Shortcut(ui.Cmd, ui.KeyE) && !c.ShortcutRepeated() && len(sel) == 1 {
		if a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{ID: sel[0]})
		}
	}

	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyM) && !c.ShortcutRepeated() {
		s.openMoveModal(sel)
	}

	if c.Shortcut(ui.Cmd, ui.KeyC) && !c.ShortcutRepeated() {
		if len(sel) > 1 {
			s.copyBookmarkURLs(c, sel)
		} else {
			s.copyBookmarkURL(c, sel[0])
		}
	}

	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyC) && !c.ShortcutRepeated() {
		if len(sel) > 1 {
			s.copyBookmarkTitles(c, sel)
		} else {
			s.copyBookmarkTitle(c, sel[0])
		}
	}

	if (c.Shortcut(0, ui.KeyBackspace) || c.Shortcut(0, ui.KeyDelete)) && !c.ShortcutRepeated() {
		s.clearSelection()
		a.Delete(sel...)
	}
}

func (s *mainState) isBookmarkSelected(id string) bool {
	if id == "" {
		return false
	}
	if len(s.selectedIDs) > 0 {
		return s.selectedIDs[id]
	}
	return s.cursorID == id
}

func (s *mainState) clearSelection() {
	clear(s.selectedIDs)
}

func (s *mainState) toggleSelect(id string) {
	if id == "" {
		return
	}
	if s.selectedIDs == nil {
		s.selectedIDs = make(map[string]bool)
	}
	if len(s.selectedIDs) == 0 && s.cursorID != "" && s.cursorID != id {
		s.selectedIDs[s.cursorID] = true
	}
	if s.selectedIDs[id] {
		delete(s.selectedIDs, id)
		if s.cursorID == id {
			s.cursorID = ""
			for other := range s.selectedIDs {
				s.cursorID = other
				break
			}
		}
		return
	}
	s.selectedIDs[id] = true
	s.cursorID = id
}

func (s *mainState) pruneSelection(lib *model.Library) {
	for id := range s.selectedIDs {
		if b := lib.Bookmark(id); b == nil || b.InTrash() {
			delete(s.selectedIDs, id)
		}
	}
}

func (s *mainState) selectedList() []string {
	ids := s.visibleBookmarkIDs()
	if len(s.selectedIDs) > 0 {
		out := make([]string, 0, len(s.selectedIDs))
		for _, id := range ids {
			if s.selectedIDs[id] {
				out = append(out, id)
			}
		}
		return out
	}
	if s.cursorID != "" && slices.Contains(ids, s.cursorID) {
		return []string{s.cursorID}
	}
	return nil
}

func (s *mainState) actionBookmarkIDs(targetID string) []string {
	if len(s.selectedIDs) > 1 && s.selectedIDs[targetID] {
		if sel := s.selectedList(); len(sel) > 1 {
			return sel
		}
	}
	return []string{targetID}
}

func (s *mainState) switchGroup(groupID string) {
	if groupID == "" || groupID == s.app.Group {
		return
	}
	s.clearSearch()
	s.clearSelection()
	s.cursorID = ""
	s.subTabsScroll.X = 0
	s.browseList.ScrollTo(0, ui.Start)
	s.stopBrowseFollow()
	s.app.Go(groupID, "")
}

func (s *mainState) stepGroup(delta int) {
	groups := s.app.Store.Lib().Groups
	if len(groups) <= 1 {
		return
	}
	idx := 0
	for i, g := range groups {
		if g.ID == s.app.Group {
			idx = i
			break
		}
	}
	next := (idx + delta + len(groups)) % len(groups)
	s.switchGroup(groups[next].ID)
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
	s.clearSelection()
	s.cursorID = target
	s.cursorScrollMoved = true
	s.stopBrowseFollow()
	if b := a.Store.Lib().Bookmark(target); b != nil && b.SubID != "" {
		a.SetSub(b.SubID)
	}
}

// moveGridRow moves the cursor to the same column of the previous or next
// card row, crossing sub-group sections. It reports false when the grid
// layout is unknown so the caller can fall back to linear movement.
func (s *mainState) moveGridRow(ids []string, delta int) bool {
	bc := &s.browseCache
	if bc.view != "grid" || bc.groupID != s.app.Group || s.cursorID == "" || !slices.Contains(ids, s.cursorID) {
		return false
	}
	r, ok := bc.bmRow[s.cursorID]
	if !ok || r < 0 || r >= len(bc.rows) {
		return false
	}
	col := slices.IndexFunc(bc.rows[r].items, func(b *model.Bookmark) bool { return b.ID == s.cursorID })
	if col < 0 {
		return false
	}
	for i := r + delta; i >= 0 && i < len(bc.rows); i += delta {
		row := bc.rows[i]
		if row.kind != browseRowGridRow || len(row.items) == 0 {
			continue
		}
		target := row.items[min(col, len(row.items)-1)].ID
		if idx := slices.Index(ids, target); idx >= 0 {
			cur := slices.Index(ids, s.cursorID)
			s.moveCursor(ids, idx-cur)
		}
		return true
	}
	return true // already on the first/last row: stay put
}

func (s *mainState) drawExternalDropOverlay(c *ui.Context, p *theme.Palette) {
	a := s.app
	lib := a.Store.Lib()
	targetName := locales.MainDropDefaultTarget()
	if g := lib.Group(a.Group); g != nil {
		targetName = g.Name
		if sub := lib.Sub(g.ID, a.Sub); sub != nil {
			targetName += " / " + sub.Name
		}
	}
	left := float32(196)
	if s.searchOpen {
		left = 12
	}
	ui.Column(c).Absolute().Left(left).Top(58).Right(12).Bottom(12).
		Radius(12).Background(p.AccentBg.Alpha(0.92)).Border(2, p.Accent).BorderStyle(ui.BorderDashed).
		Center().PassThrough().Children(func() {
		ui.Text(c, locales.MainDropSaveTo(targetName)).FontSize(14).FontWeight(600).TextColor(p.AccentText)
	})
}

func (s *mainState) handleExternalDrop(drop transfer.Drop) {
	a := s.app
	if paths, err := drop.Data.Files(); err == nil && len(paths) > 0 {
		var importable []string
		for _, path := range paths {
			low := strings.ToLower(path)
			if strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".htm") || strings.HasSuffix(low, ".json") {
				importable = append(importable, path)
			}
		}
		if len(importable) > 0 {
			if a.Win.ShowImport != nil {
				for _, path := range importable {
					a.Win.ShowImport(path)
				}
			}
			return
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
