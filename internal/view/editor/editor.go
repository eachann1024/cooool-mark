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
	"cooool-mark/internal/model"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/widget"
)

var (
	edWin *mygo.Window
	edSt  *state
)

// Show opens the editor for req (req.ID == "" adds a bookmark). Main thread.
func Show(a *app.App, req app.EditorRequest) {
	a.PrepareRegularWindow()
	if edWin != nil && edSt != nil {
		edSt.load(req)
		edWin.SetTitle(edSt.windowTitle())
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
		Content:              ui.View(st.view),
	})
	edWin = w
	st.win = w
	a.AddRegularWindow(w)
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

type state struct {
	a      *app.App
	win    *mygo.Window
	closed bool

	// Bookmark fields
	id        string
	url       string
	title     string
	note      string
	tags      []string
	icon      model.Icon
	groupID   string
	subID     string
	createdAt time.Time
	order     float64
	polished  bool

	// Tag / sub-group inline drafts
	tagDraft      string
	addingSub     bool
	newSubName    string
	newSubFocused bool

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

	aiSeq       uint64
	aiCancel    context.CancelFunc
	aiLoading   bool
	aiSuggest   *app.Polish
	aiErr       string
	aiDismissed bool

	saveErr string

	// Focus handles
	urlHandle    ui.Handle
	titleHandle  ui.Handle
	newSubHandle ui.Handle
	needFocus    *ui.Handle
}

func newState(a *app.App) *state {
	return &state{a: a}
}

func (s *state) windowTitle() string {
	if s.id == "" {
		return "新建书签"
	}
	return "编辑书签"
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

func (s *state) load(req app.EditorRequest) {
	s.cancelAll()
	s.sessionSeq++
	s.addingSub = false
	s.newSubName = ""
	s.newSubFocused = false
	s.tagDraft = ""
	s.fetchErr = ""
	s.aiErr = ""
	s.saveErr = ""
	s.lastFetchedURL = ""
	s.aiSuggest = nil
	s.aiDismissed = false

	lib := s.a.Store.Lib()
	if req.ID != "" {
		if b := lib.Bookmark(req.ID); b != nil {
			s.id = b.ID
			s.url = b.URL
			s.title = b.Title
			s.note = b.Note
			s.tags = slices.Clone(b.Tags)
			s.icon = b.Icon
			s.groupID = b.GroupID
			s.subID = b.SubID
			s.createdAt = b.CreatedAt
			s.order = b.Order
			s.polished = b.Polished
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
			s.needFocus = &s.titleHandle
			return
		}
	}

	s.id = ""
	s.url = strings.TrimSpace(req.URL)
	s.title = strings.TrimSpace(req.Title)
	s.note = ""
	s.tags = nil
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

	if s.url != "" {
		s.needFocus = &s.titleHandle
		if looksLikeURL(s.url) {
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
	if s.id == "" {
		s.icon.Image = ""
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
		s.fetchErr = "当前无法读取网页"
		return
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		s.fetchErr = "请输入网址"
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
					s.fetchErr = "读取网页超时"
				} else if iconOnly {
					s.fetchErr = "抓取图标失败"
				} else {
					s.fetchErr = "读取网页失败"
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
			}
			if len(meta.Icon) > 0 {
				if _, decErr := ui.DecodeImage(meta.Icon); decErr != nil {
					if iconOnly {
						s.fetchErr = "站点图标格式不支持"
					}
				} else if rel, saveErr := s.a.Store.SaveIcon(meta.Icon, meta.IconExt); saveErr != nil {
					if iconOnly {
						s.fetchErr = "保存图标失败"
					}
				} else {
					s.icon.Image = rel
				}
			} else if iconOnly {
				s.fetchErr = "未获取到站点图标"
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
			s.aiErr = "请先输入网址"
			s.needFocus = &s.urlHandle
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

	in := app.PolishInput{
		URL:         u,
		Title:       strings.TrimSpace(s.title),
		Description: strings.TrimSpace(s.note),
		Choices:     s.a.Choices(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	s.aiCancel = cancel

	go func() {
		defer cancel()
		p, err := s.a.AI.Polish(ctx, in)
		s.a.Post(func() {
			if s.closed || seq != s.aiSeq || strings.TrimSpace(s.url) != u {
				return
			}
			s.aiLoading = false
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					if errors.Is(err, context.DeadlineExceeded) {
						s.aiErr = "AI 润色超时"
					} else {
						s.aiErr = "AI 润色失败：" + err.Error()
					}
				}
				s.invalidate()
				return
			}
			p.Title = strings.TrimSpace(p.Title)
			p.Note = strings.TrimSpace(p.Note)
			p.Tags = cleanTags(p.Tags)
			if p.Title == "" && p.Note == "" && len(p.Tags) == 0 && p.GroupID == "" {
				s.aiSuggest = nil
			} else {
				s.aiSuggest = &p
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
	for _, t := range p.Tags {
		if !slices.Contains(s.tags, t) {
			return true
		}
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
	if strings.TrimSpace(s.tagDraft) != "" {
		s.commitTagDraft()
	}
	p := s.aiSuggest
	if p.Title != "" {
		s.title = p.Title
	}
	if p.Note != "" {
		s.note = p.Note
	}
	for _, t := range p.Tags {
		t = strings.TrimSpace(t)
		if t != "" && !slices.Contains(s.tags, t) {
			s.tags = append(s.tags, t)
		}
	}
	if p.GroupID != "" && s.a.Store.Lib().Sub(p.GroupID, p.SubID) != nil {
		s.groupID, s.subID = p.GroupID, p.SubID
		s.addingSub = false
		s.newSubName = ""
		s.newSubFocused = false
	}
	s.aiDismissed = true
	s.polished = true
}

func cleanTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

func (s *state) previewIcon() model.Icon {
	if s.icon.Image != "" {
		return s.icon
	}
	return theme.DefaultIcon(s.title, s.url)
}

func (s *state) commitTagDraft() {
	parts := strings.FieldsFunc(s.tagDraft, func(r rune) bool {
		return r == ',' || r == '，'
	})
	s.tagDraft = ""
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" && !slices.Contains(s.tags, tag) {
			s.tags = append(s.tags, tag)
		}
	}
}

func (s *state) commitNewSub() {
	name := strings.TrimSpace(s.newSubName)
	s.addingSub = false
	s.newSubName = ""
	s.newSubFocused = false
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

func (s *state) save() {
	if s.addingSub {
		s.commitNewSub()
	}
	if strings.TrimSpace(s.tagDraft) != "" {
		s.commitTagDraft()
	}

	u, errMsg := validateSaveURL(s.url)
	if errMsg != "" {
		s.saveErr = errMsg
		s.needFocus = &s.urlHandle
		s.invalidate()
		return
	}
	s.saveErr = ""

	title := strings.TrimSpace(s.title)
	if title == "" {
		title = fallbackTitleFromURL(u)
	}

	s.ensureValidPlace()

	icon := s.icon
	if icon.Image == "" {
		icon = theme.DefaultIcon(title, u)
	}

	bm := model.Bookmark{
		ID:        s.id,
		Title:     title,
		URL:       u,
		Note:      strings.TrimSpace(s.note),
		Tags:      slices.Clone(s.tags),
		Icon:      icon,
		GroupID:   s.groupID,
		SubID:     s.subID,
		Order:     s.order,
		CreatedAt: s.createdAt,
		Polished:  s.polished,
	}
	if s.a.SaveBookmark(bm) == "" {
		s.saveErr = "这条书签已被删除，请将修改复制到新书签"
		s.invalidate()
		return
	}
	s.close()
}

func (s *state) deleteCurrent() {
	if s.id == "" {
		return
	}
	s.a.Delete(s.id)
	s.close()
}

func validateSaveURL(raw string) (string, string) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", "请输入网址"
	}
	if strings.ContainsAny(u, " \t\n\r") {
		return "", "网址不能包含空格"
	}
	low := strings.ToLower(u)
	if !strings.Contains(u, "://") && !strings.HasPrefix(low, "about:") && !strings.HasPrefix(low, "file:") && !strings.HasPrefix(low, "javascript:") {
		u = "https://" + u
		low = strings.ToLower(u)
	}
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		parsed, err := url.Parse(u)
		if err != nil || parsed.Host == "" {
			return "", "请输入有效网址"
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

func fallbackTitleFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.TrimPrefix(u.Host, "www.")
	}
	return raw
}
