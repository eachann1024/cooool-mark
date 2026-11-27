// Package editor is the small add/edit bookmark window.
package editor

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

var (
	edWin *mygo.Window
	edSt  *state
)

// RefreshChrome updates the editor window title and triggers a redraw when open.
func RefreshChrome() {
	if edWin != nil && !edWin.IsDestroyed() && edSt != nil {
		edWin.SetTitle(edSt.windowTitle())
		edWin.Invalidate()
	}
}

// AllowClose bypasses the unsaved-draft confirmation when the OS session is ending.
func AllowClose() {
	if edSt != nil {
		edSt.forceClose = true
	}
}

// PromptUnsavedOnQuit checks before any windows are closed during app quit
// whether the editor holds an unsaved draft. When true, it focuses the editor
// with the close confirmation dialog and returns true so OnBeforeQuit can
// cancel the quit without closing other windows (like Main or Import).
func PromptUnsavedOnQuit() bool {
	if edWin == nil || edWin.IsDestroyed() || edSt == nil || edSt.forceClose || !edSt.hasPendingDraft() {
		return false
	}
	edSt.confirmSwitch = false
	edSt.confirmClose = true
	edSt.quitAfterClose = true
	edWin.Show()
	edWin.Focus()
	edWin.Invalidate()
	return true
}

// Show opens the editor for req (req.ID == "" adds a bookmark). Main thread.
func Show(a *app.App, req app.EditorRequest) {
	a.PrepareRegularWindow()
	if edWin != nil && !edWin.IsDestroyed() && edSt != nil {
		if edSt.hasPendingDraft() {
			edSt.pendingReqs = append(edSt.pendingReqs, req)
			edSt.confirmClose = false
			edSt.quitAfterClose = false
			edSt.confirmSwitch = true
		} else {
			edSt.load(req)
			edWin.SetTitle(edSt.windowTitle())
		}
		edWin.Show()
		edWin.Focus()
		edWin.Invalidate()
		return
	}

	st := newState(a)
	st.load(req)
	edSt = st

	w := mygo.NewWindow(mygo.WindowOptions{
		Title:                st.windowTitle(),
		Width:                492,
		Height:               600,
		MinWidth:             420,
		MinHeight:            500,
		StateKey:             "editor",
		TitleBarStyle:        mygo.TitleBarHiddenInset,
		TrafficLightPosition: widget.Lights(),
		TitleBarHeight:       widget.HeaderH,
		Content: ui.View(func(c *ui.Context) {
			c.OnWindowShortcut(ui.Cmd, ui.KeyS, st.save)
			st.view(c)
		}),
	})
	edWin = w
	st.win = w
	a.AddRegularWindow(w)
	w.OnClose(func(e *mygo.CloseEvent) {
		if !st.forceClose && st.hasPendingDraft() {
			e.PreventDefault()
			st.confirmSwitch = false
			st.confirmClose = true
			st.invalidate()
		}
	})
	w.OnClosed(func() {
		st.closed = true
		st.cancelAll()
		st.win = nil
		if edWin == w {
			edWin = nil
			edSt = nil
		}
	})
}

type editorSnapshot struct {
	url       string
	title     string
	note      string
	groupID   string
	subID     string
	iconImg   string
	keywords  []string
	searchOff bool
}

type state struct {
	a      *app.App
	win    *mygo.Window
	closed bool

	// Bookmark fields
	id           string
	url          string
	title        string
	note         string
	titleEdited  bool
	noteEdited   bool
	noteDirty    bool
	icon         model.Icon
	groupID      string
	subID        string
	createdAt    time.Time
	order        float64
	polished     bool
	keywords     []string
	showInSearch bool
	searchRank   float64
	keywordInput string
	keywordErr   string

	// Draft protection & queued incoming requests
	fromExternal   bool
	baseline       editorSnapshot
	pendingReqs    []app.EditorRequest
	confirmSwitch  bool
	confirmClose   bool
	quitAfterClose bool
	forceClose     bool

	// Sub-group inline draft
	addingSub  bool
	newSubName string

	// Async fetch & AI state
	sessionSeq     uint64
	fetchSeq       uint64
	fetchCancel    context.CancelFunc
	debounceTimer  *time.Timer
	fetching       bool
	fetchingIcon   bool
	fetchErr       string
	justPasted     bool
	lastFetchedURL string
	fetchedTitle   string
	fetchedDesc    string
	fetchedExcerpt string

	aiSeq       uint64
	aiCancel    context.CancelFunc
	aiLoading   bool
	aiSuggest   *app.Polish
	aiErr       string
	aiDismissed bool

	saveErr string

	// Focus handles
	urlHandle     ui.Handle
	titleHandle   ui.Handle
	newSubHandle  ui.Handle
	keywordHandle ui.Handle
	needFocus     *ui.Handle
}

func newState(a *app.App) *state {
	return &state{a: a}
}

func (s *state) windowTitle() string {
	if s.id == "" {
		return locales.EditorNewTitle()
	}
	return locales.EditorEditTitle()
}

func (s *state) cancelFetch() {
	s.fetchSeq++
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
		s.debounceTimer = nil
	}
	if s.fetchCancel != nil {
		s.fetchCancel()
		s.fetchCancel = nil
	}
	s.fetching = false
	s.fetchingIcon = false
}

func (s *state) cancelAI() {
	s.aiSeq++
	if s.aiCancel != nil {
		s.aiCancel()
		s.aiCancel = nil
	}
	s.aiLoading = false
}

func (s *state) cancelAll() {
	s.justPasted = false
	s.cancelFetch()
	s.cancelAI()
}

func (s *state) snapshot() editorSnapshot {
	keywords, searchOff := s.templateFieldsForSave(strings.TrimSpace(s.url))
	return editorSnapshot{
		url:       strings.TrimSpace(s.url),
		title:     strings.TrimSpace(s.title),
		note:      strings.TrimSpace(s.note),
		groupID:   s.groupID,
		subID:     s.subID,
		iconImg:   s.icon.Image,
		keywords:  keywords,
		searchOff: searchOff,
	}
}

func (s *state) isDirty() bool {
	curKeywords, curSearchOff := s.templateFieldsForSave(strings.TrimSpace(s.url))
	if strings.TrimSpace(s.url) != s.baseline.url ||
		strings.TrimSpace(s.title) != s.baseline.title ||
		strings.TrimSpace(s.note) != s.baseline.note ||
		s.groupID != s.baseline.groupID ||
		s.subID != s.baseline.subID ||
		s.icon.Image != s.baseline.iconImg ||
		!slices.Equal(curKeywords, s.baseline.keywords) ||
		curSearchOff != s.baseline.searchOff ||
		(s.addingSub && strings.TrimSpace(s.newSubName) != "") {
		return true
	}
	return false
}

func (s *state) hasPendingDraft() bool {
	return s.isDirty()
}

func (s *state) requestClose() {
	if s.hasPendingDraft() {
		s.confirmSwitch = false
		s.confirmClose = true
		s.quitAfterClose = false
		s.invalidate()
		return
	}
	s.forceClose = true
	s.close()
}

func (s *state) finishClose() {
	quit := s.quitAfterClose
	s.quitAfterClose = false
	s.forceClose = true
	s.close()
	if quit {
		mygo.App.Quit()
	}
}

func (s *state) advancePending() bool {
	if len(s.pendingReqs) == 0 {
		return false
	}
	next := s.pendingReqs[0]
	s.pendingReqs = s.pendingReqs[1:]
	s.confirmSwitch = false
	s.confirmClose = false
	s.load(next)
	if s.win != nil {
		s.win.SetTitle(s.windowTitle())
	}
	s.invalidate()
	return true
}

func (s *state) load(req app.EditorRequest) {
	s.cancelAll()
	s.sessionSeq++
	s.fromExternal = req.FromExternal
	s.confirmClose = false
	s.forceClose = false
	s.addingSub = false
	s.newSubName = ""
	s.fetchErr = ""
	s.aiErr = ""
	s.saveErr = ""
	s.keywordInput = ""
	s.keywordErr = ""
	s.keywords = nil
	s.showInSearch = true
	s.searchRank = 0
	s.lastFetchedURL = ""
	s.aiSuggest = nil
	s.aiDismissed = false
	s.noteDirty = false
	s.fetchedTitle = ""
	s.fetchedDesc = ""
	s.fetchedExcerpt = ""

	lib := s.a.Store.Lib()
	if req.ID != "" {
		if b := lib.Bookmark(req.ID); b != nil {
			s.id = b.ID
			s.url = b.URL
			s.title = b.Title
			s.note = b.Note
			s.icon = b.Icon
			s.groupID = b.GroupID
			s.subID = b.SubID
			s.createdAt = b.CreatedAt
			s.order = b.Order
			s.polished = b.Polished
			if b.IsTemplate() {
				s.keywords = app.NormalizeKeywords(b.Keywords)
				s.showInSearch = !b.SearchOff
				s.searchRank = b.SearchRank
			}
			s.titleEdited = true
			s.noteEdited = true
			if s.icon.Letter == "" || s.icon.Color == "" {
				def := theme.DefaultIcon(s.title, s.url)
				if s.icon.Letter == "" {
					s.icon.Letter = def.Letter
				}
				if s.icon.Color == "" {
					s.icon.Color = def.Color
				}
			}
			s.ensureValidPlace()
			s.baseline = s.snapshot()
			s.needFocus = &s.titleHandle
			return
		}
	}

	s.id = ""
	s.url = strings.TrimSpace(req.URL)
	s.title = strings.TrimSpace(req.Title)
	s.note = ""
	s.titleEdited = s.title != ""
	s.noteEdited = false
	s.createdAt = time.Time{}
	s.order = 0
	s.polished = false

	s.groupID = req.GroupID
	s.subID = req.SubID
	if lib.Sub(s.groupID, s.subID) == nil {
		s.groupID, s.subID = s.a.Group, s.a.Sub
	}
	s.ensureValidPlace()

	s.icon = theme.DefaultIcon(s.title, s.url)
	// For a new bookmark, compare against an empty draft so any prefilled URL
	// or title (from drag-drop, clipboard, or deep link) is protected until
	// explicitly saved or discarded.
	s.baseline = editorSnapshot{
		groupID: s.groupID,
		subID:   s.subID,
	}

	if s.url != "" {
		s.needFocus = &s.titleHandle
		// External deep-link drafts require explicit user confirmation before
		// making outbound network/WebView/AI requests.
		if !req.FromExternal && looksLikeURL(s.url) {
			s.startFetch(s.url, false, true, false)
		}
	} else {
		s.needFocus = &s.urlHandle
	}
}

func (s *state) ensureValidPlace() {
	lib := s.a.Store.Lib()
	g := lib.Group(s.groupID)
	if g == nil && len(lib.Groups) > 0 {
		g = &lib.Groups[0]
	}
	if g == nil {
		s.groupID, s.subID = "", ""
		return
	}
	s.groupID = g.ID
	if lib.Sub(g.ID, s.subID) == nil {
		s.subID = ""
		if len(g.Subs) > 0 {
			s.subID = g.Subs[0].ID
		}
	}
}

func (s *state) invalidate() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

func (s *state) close() {
	if s.win != nil {
		s.win.Close()
	}
}

func (s *state) scheduleDebouncedFetch() {
	s.saveErr = ""
	if !isTemplateURL(s.url) {
		s.keywordErr = ""
	}
	u := strings.TrimSpace(s.url)
	if u == s.lastFetchedURL && s.fetchErr == "" {
		return
	}
	s.cancelFetch()
	s.cancelAI()
	s.aiSuggest = nil
	s.aiErr = ""
	s.aiDismissed = false
	s.fetchErr = ""
	s.fetchedTitle = ""
	s.fetchedDesc = ""
	s.fetchedExcerpt = ""
	if s.id == "" {
		s.icon.Image = ""
		if !s.titleEdited {
			s.title = ""
		}
		if !s.noteEdited {
			s.note = ""
		}
	}
	if !looksLikeURL(u) {
		s.justPasted = false
		return
	}
	seq := s.fetchSeq
	delay := 450 * time.Millisecond
	if s.justPasted {
		s.justPasted = false
		delay = 30 * time.Millisecond
	}
	s.debounceTimer = time.AfterFunc(delay, func() {
		s.a.Post(func() {
			if s.closed || s.fetchSeq != seq {
				return
			}
			cur := strings.TrimSpace(s.url)
			if looksLikeURL(cur) && (cur != s.lastFetchedURL || s.fetchErr != "") {
				s.startFetch(cur, false, true, false)
			}
		})
	})
}

func (s *state) startFetch(rawURL string, forceMeta, runAI, iconOnly bool) {
	if s.a.Fetch == nil {
		s.fetchErr = locales.EditorErrFetchUnavailable()
		return
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		s.fetchErr = locales.EditorErrEnterURL()
		s.needFocus = &s.urlHandle
		return
	}
	s.cancelFetch()
	seq := s.fetchSeq
	s.lastFetchedURL = rawURL
	s.fetching = true
	s.fetchingIcon = iconOnly
	s.fetchErr = ""
	s.saveErr = ""

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	s.fetchCancel = cancel

	go func() {
		defer cancel()
		meta, err := s.a.Fetch.Fetch(ctx, rawURL)
		s.a.Post(func() {
			if s.closed || seq != s.fetchSeq || strings.TrimSpace(s.url) != rawURL {
				return
			}
			s.fetching = false
			s.fetchingIcon = false
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					s.fetchErr = locales.EditorErrFetchTimeout()
				} else if iconOnly {
					s.fetchErr = locales.EditorErrFetchIconFailed()
				} else {
					s.fetchErr = locales.EditorErrFetchFailed()
				}
				s.invalidate()
				return
			}
			if !iconOnly {
				if meta.URL != "" && !strings.Contains(rawURL, "{query}") && strings.TrimSpace(s.url) == rawURL {
					s.url = meta.URL
					s.lastFetchedURL = meta.URL
				}
				if meta.Title != "" && (forceMeta || strings.TrimSpace(s.title) == "") {
					s.title = meta.Title
				}
				if meta.Description != "" && (forceMeta || strings.TrimSpace(s.note) == "") {
					s.note = meta.Description
				}
				s.fetchedTitle = strings.TrimSpace(meta.Title)
				s.fetchedDesc = strings.TrimSpace(meta.Description)
				s.fetchedExcerpt = strings.TrimSpace(meta.Excerpt)
			}
			if len(meta.Icon) > 0 {
				if _, decErr := ui.DecodeImage(meta.Icon); decErr != nil {
					if iconOnly {
						s.fetchErr = locales.EditorErrIconFormat()
					}
				} else if rel, saveErr := s.a.Store.SaveIcon(meta.Icon, meta.IconExt); saveErr != nil {
					if iconOnly {
						s.fetchErr = locales.EditorErrSaveIcon()
					}
				} else {
					s.icon.Image = rel
				}
			} else if iconOnly {
				s.fetchErr = locales.EditorErrNoIcon()
			}
			if s.icon.Image == "" {
				s.icon = theme.DefaultIcon(s.title, s.url)
			}
			s.invalidate()
			if runAI && !iconOnly {
				s.startAI(false)
			}
		})
	}()
}

func (s *state) aiReady() bool {
	return s.a.AI != nil && s.a.AI.Status().Ready
}

func (s *state) startAI(force bool) {
	if !s.aiReady() {
		return
	}
	if !force && (!s.a.Settings.AIAutoPolish || s.aiDismissed) {
		return
	}
	u := strings.TrimSpace(s.url)
	if u == "" {
		if force {
			s.aiErr = locales.EditorErrEnterURLFirst()
			s.needFocus = &s.urlHandle
		}
		return
	}
	desc := strings.TrimSpace(s.fetchedDesc)
	if app.IsJunkAINote(desc) {
		desc = ""
	}
	if desc == "" && (!s.polished || s.noteDirty) {
		if n := strings.TrimSpace(s.note); n != "" && !app.IsJunkAINote(n) {
			desc = n
		}
	}
	title := strings.TrimSpace(s.title)
	excerpt := strings.TrimSpace(s.fetchedExcerpt)
	isLocal := s.a.Settings.AIEngine == model.AILocal
	needFreshFetch := force && s.a.Fetch != nil && s.lastFetchedURL != u &&
		!strings.Contains(u, "{query}") && !app.IsLocalOrIPURL(u)

	if !needFreshFetch && isLocal &&
		!app.HasGroundedNoteContext(u, title, desc, excerpt) &&
		!app.TitleNeedsCleanup(title, u) {
		if force {
			s.cancelAI()
			s.aiSuggest = nil
			if strings.TrimSpace(s.note) != "" && ((s.polished && !s.noteDirty) || app.IsJunkAINote(s.note)) {
				s.note = ""
				s.polished = false
				s.aiErr = locales.ToastPolishCleared()
			} else {
				s.aiErr = locales.ToastPolishInsufficient()
			}
			s.invalidate()
		}
		return
	}

	s.cancelAI()
	seq := s.aiSeq
	s.aiLoading = true
	s.aiErr = ""
	s.aiSuggest = nil
	if force {
		s.aiDismissed = false
	}

	choices := s.a.Choices()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	s.aiCancel = cancel

	go func() {
		defer cancel()
		var fetchedMeta *app.PageMeta
		if needFreshFetch {
			fctx, fcancel := context.WithTimeout(ctx, 20*time.Second)
			meta, ferr := s.a.Fetch.Fetch(fctx, u)
			fcancel()
			if ferr == nil {
				fetchedMeta = &meta
				if ft := strings.TrimSpace(meta.Title); ft != "" && !app.IsPlaceholderTitle(ft) && (title == "" || app.IsPlaceholderTitle(title) || len([]rune(ft)) > len([]rune(title))) {
					title = ft
				}
				if fd := strings.TrimSpace(meta.Description); fd != "" && !app.IsJunkAINote(fd) {
					desc = fd
				}
				if fe := strings.TrimSpace(meta.Excerpt); fe != "" {
					excerpt = fe
				}
			}
		}

		if isLocal &&
			!app.HasGroundedNoteContext(u, title, desc, excerpt) &&
			!app.TitleNeedsCleanup(title, u) {
			s.a.Post(func() {
				if s.closed || seq != s.aiSeq || strings.TrimSpace(s.url) != u {
					return
				}
				s.aiLoading = false
				if fetchedMeta != nil {
					s.lastFetchedURL = u
					s.fetchedTitle = strings.TrimSpace(fetchedMeta.Title)
					s.fetchedDesc = strings.TrimSpace(fetchedMeta.Description)
					s.fetchedExcerpt = strings.TrimSpace(fetchedMeta.Excerpt)
				}
				if force {
					if strings.TrimSpace(s.note) != "" && ((s.polished && !s.noteDirty) || app.IsJunkAINote(s.note)) {
						s.note = ""
						s.polished = false
						s.aiErr = locales.ToastPolishCleared()
					} else {
						s.aiErr = locales.ToastPolishInsufficient()
					}
				}
				s.invalidate()
			})
			return
		}

		in := app.PolishInput{
			URL:         u,
			Title:       title,
			Description: desc,
			Excerpt:     excerpt,
			Choices:     choices,
		}
		p, err := s.a.AI.Polish(ctx, in)
		s.a.Post(func() {
			if s.closed || seq != s.aiSeq || strings.TrimSpace(s.url) != u {
				return
			}
			s.aiLoading = false
			if fetchedMeta != nil {
				s.lastFetchedURL = u
				s.fetchedTitle = strings.TrimSpace(fetchedMeta.Title)
				s.fetchedDesc = strings.TrimSpace(fetchedMeta.Description)
				s.fetchedExcerpt = strings.TrimSpace(fetchedMeta.Excerpt)
			}
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					if errors.Is(err, context.DeadlineExceeded) {
						s.aiErr = locales.EditorErrAITimeout()
					} else {
						s.aiErr = locales.EditorErrAIFailed(err.Error())
					}
				}
				s.invalidate()
				return
			}
			p.Title = strings.TrimSpace(p.Title)
			p.Note = strings.TrimSpace(p.Note)
			s.aiSuggest = &p
			if !s.hasAISuggestion() {
				s.aiSuggest = nil
				if force {
					if p.Title == "" && p.Note == "" && p.GroupID == "" && strings.TrimSpace(s.note) != "" &&
						((s.polished && !s.noteDirty) || app.IsJunkAINote(s.note)) {
						s.note = ""
						s.polished = false
						s.aiErr = locales.ToastPolishCleared()
					} else if p.Note == "" && !s.polished && app.HasGroundedNoteContext(u, title, desc, excerpt) {
						s.aiErr = locales.ToastPolishNoNote()
					} else {
						s.a.Say(locales.ToastPolishUnchanged())
					}
				}
			}
			s.invalidate()
		})
	}()
}

func (s *state) hasAISuggestion() bool {
	if !s.aiReady() || s.aiSuggest == nil || s.aiDismissed {
		return false
	}
	p := s.aiSuggest
	if p.Title != "" && p.Title != strings.TrimSpace(s.title) {
		return true
	}
	if p.Note != "" && p.Note != strings.TrimSpace(s.note) {
		return true
	}
	if p.GroupID != "" && s.a.Store.Lib().Sub(p.GroupID, p.SubID) != nil &&
		(p.GroupID != s.groupID || p.SubID != s.subID) {
		return true
	}
	return false
}

func (s *state) applyAI() {
	if s.aiSuggest == nil {
		return
	}
	p := s.aiSuggest
	if p.Title != "" {
		s.title = p.Title
	}
	if p.Note != "" {
		s.note = p.Note
	}
	if p.GroupID != "" && s.a.Store.Lib().Sub(p.GroupID, p.SubID) != nil {
		s.groupID, s.subID = p.GroupID, p.SubID
		s.addingSub = false
		s.newSubName = ""
	}
	s.aiDismissed = true
	s.polished = true
}

func (s *state) previewIcon() model.Icon {
	if s.icon.Image != "" {
		return s.icon
	}
	return theme.DefaultIcon(s.title, s.url)
}

func (s *state) commitNewSub() {
	name := strings.TrimSpace(s.newSubName)
	s.addingSub = false
	s.newSubName = ""
	if name == "" || s.groupID == "" {
		return
	}
	lib := s.a.Store.Lib()
	if g := lib.Group(s.groupID); g != nil {
		for _, sub := range g.Subs {
			if sub.Name == name {
				s.subID = sub.ID
				return
			}
		}
	}
	if subID := s.a.AddSub(s.groupID, name); subID != "" && s.a.Store.Lib().Sub(s.groupID, subID) != nil {
		s.subID = subID
	}
}

func (s *state) doSave() bool {
	u, errMsg := validateSaveURL(s.url)
	if errMsg != "" {
		s.saveErr = errMsg
		s.needFocus = &s.urlHandle
		s.invalidate()
		return false
	}
	if s.id != "" && s.a.Store.Lib().Bookmark(s.id) == nil {
		s.saveErr = locales.EditorErrDeleted()
		s.invalidate()
		return false
	}
	s.saveErr = ""

	if s.addingSub {
		s.commitNewSub()
	}

	isNew := s.id == ""
	rawTitle := strings.TrimSpace(s.title)
	fallbackTitle := fallbackTitleFromURL(u)
	title := rawTitle
	if title == "" {
		title = fallbackTitle
	}

	s.ensureValidPlace()

	icon := s.icon
	if icon.Image == "" {
		icon = theme.DefaultIcon(title, u)
	}

	keywords, searchOff := s.templateFieldsForSave(u)
	var searchRank float64
	if isTemplateURL(u) {
		searchRank = s.searchRank
	}

	bm := model.Bookmark{
		ID:         s.id,
		Title:      title,
		URL:        u,
		Note:       strings.TrimSpace(s.note),
		Icon:       icon,
		GroupID:    s.groupID,
		SubID:      s.subID,
		Order:      s.order,
		CreatedAt:  s.createdAt,
		Polished:   s.polished,
		Keywords:   keywords,
		SearchOff:  searchOff,
		SearchRank: searchRank,
	}
	if isNew {
		fetchedOK := !s.fetching && s.fetchErr == "" && s.lastFetchedURL != "" &&
			(s.lastFetchedURL == u || strings.TrimSpace(s.url) == s.lastFetchedURL)
		needFetch := s.a.Fetch != nil && !fetchedOK
		allowTitle := !s.titleEdited
		allowNote := !s.noteEdited
		// If the editor already produced a visible AI suggestion that the user
		// chose not to apply, respect that choice instead of overwriting it in
		// the background.
		allowPolish := !s.polished && !s.aiDismissed && s.aiSuggest == nil && (allowTitle || allowNote)
		if s.a.SaveNewBookmark(bm, app.EnrichOptions{
			Fetch:          needFetch,
			AllowTitle:     allowTitle,
			AllowNote:      allowNote,
			AllowPolish:    allowPolish,
			FallbackTitle:  fallbackTitle,
			FetchedExcerpt: s.fetchedExcerpt,
		}) == "" {
			s.saveErr = locales.EditorErrDeleted()
			s.invalidate()
			return false
		}
		return true
	}
	if s.a.SaveBookmark(bm) == "" {
		s.saveErr = locales.EditorErrDeleted()
		s.invalidate()
		return false
	}
	return true
}

func (s *state) save() {
	if !s.doSave() {
		return
	}
	if s.advancePending() {
		return
	}
	s.forceClose = true
	s.close()
}

func (s *state) deleteCurrent() {
	if s.id == "" {
		return
	}
	s.a.Delete(s.id)
	s.forceClose = true
	s.close()
}

func validateSaveURL(raw string) (string, string) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", locales.EditorErrEnterURL()
	}
	if strings.ContainsAny(u, " \t\n\r") {
		return "", locales.EditorErrURLSpace()
	}
	low := strings.ToLower(u)
	if !strings.Contains(u, "://") && !strings.HasPrefix(low, "about:") && !strings.HasPrefix(low, "file:") && !strings.HasPrefix(low, "javascript:") {
		u = "https://" + u
		low = strings.ToLower(u)
	}
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		parsed, err := url.Parse(u)
		if err != nil || parsed.Host == "" {
			return "", locales.EditorErrURLInvalid()
		}
	}
	return u, ""
}

func looksLikeURL(raw string) bool {
	u := strings.TrimSpace(raw)
	if u == "" || strings.ContainsAny(u, " \t\n\r") {
		return false
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		parsed, err := url.Parse(u)
		return err == nil && parsed.Host != "" && strings.Contains(parsed.Host, ".")
	}
	return strings.Contains(u, ".") && !strings.HasPrefix(u, ".") && !strings.HasSuffix(u, ".")
}

func isTemplateURL(raw string) bool {
	return strings.Contains(raw, "{query}")
}

func (s *state) isNewTemplate() bool {
	return s.id == "" || !isTemplateURL(s.baseline.url)
}

func (s *state) templateFieldsForSave(u string) ([]string, bool) {
	if !isTemplateURL(u) {
		return nil, false
	}
	return app.NormalizeKeywords(s.keywords), !s.showInSearch
}

func (s *state) makeTemplate(sp app.SearchParam) {
	s.url = sp.Pre + "{query}" + sp.Post
	s.saveErr = ""
	s.keywordErr = ""
}

func (s *state) tryAddKeyword(raw string) bool {
	norm := app.NormalizeKeywords([]string{raw})
	if len(norm) == 0 {
		s.keywordErr = ""
		return true
	}
	kw := norm[0]
	if slices.Contains(s.keywords, kw) {
		s.keywordErr = ""
		return true
	}
	if owner := s.a.KeywordOwner(kw, s.id); owner != nil {
		s.keywordErr = locales.EditorKeywordTaken(keywordOwnerTitle(owner))
		return false
	}
	s.keywords = app.NormalizeKeywords(append(s.keywords, kw))
	s.keywordErr = ""
	return true
}

func (s *state) removeKeyword(kw string) {
	s.keywords = slices.DeleteFunc(s.keywords, func(k string) bool { return k == kw })
	s.keywordErr = ""
}

func keywordOwnerTitle(b *model.Bookmark) string {
	if t := strings.TrimSpace(b.Title); t != "" {
		return t
	}
	return fallbackTitleFromURL(b.URL)
}

func fallbackTitleFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.TrimPrefix(u.Host, "www.")
	}
	return raw
}
