package app

import (
	"context"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo"

	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
	"cooool-mark/internal/store"
)

// ---- bookmarks ----

// EnrichOptions controls background webpage recognition and AI auto-polish
// after a bookmark is saved.
type EnrichOptions struct {
	Fetch          bool
	AllowTitle     bool
	AllowNote      bool
	AllowPolish    bool
	FallbackTitle  string
	FetchedExcerpt string
}

// IsEnriching reports whether bookmark id currently has a background fetch or
// AI polish job running.
func (a *App) IsEnriching(id string) bool {
	return a.Enriching[id] > 0
}

func (a *App) startEnriching(id string) {
	if a.Enriching == nil {
		a.Enriching = map[string]int{}
	}
	a.Enriching[id]++
	a.Invalidate()
}

func (a *App) stopEnriching(id string) {
	if a.Enriching == nil {
		return
	}
	if n := a.Enriching[id]; n <= 1 {
		delete(a.Enriching, id)
	} else {
		a.Enriching[id] = n - 1
	}
	a.Invalidate()
}

// SaveBookmark adds b (ID "") or replaces the bookmark with its ID. It
// returns the id. New bookmarks go to the end of their sub-group. With AI
// auto-polish on and the bookmark not yet polished, a background pass
// fills empty fields afterwards (never blocking the save).
func (a *App) SaveBookmark(b model.Bookmark) string {
	return a.saveBookmark(b, true, true)
}

// SaveNewBookmark saves a new bookmark immediately and optionally runs
// background webpage recognition (title, description, icon) and AI auto-polish
// without blocking the caller.
func (a *App) SaveNewBookmark(b model.Bookmark, opts EnrichOptions) string {
	id := a.saveBookmark(b, true, false)
	if id == "" {
		return ""
	}
	if opts.Fetch || opts.AllowPolish {
		a.EnrichBookmark(id, opts)
	}
	return id
}

func (a *App) saveBookmark(b model.Bookmark, toast, autoPolish bool) string {
	now := time.Now()
	b.Title = strings.TrimSpace(b.Title)
	b.URL = strings.TrimSpace(b.URL)
	if b.Icon.Image == "" && b.Icon.Letter == "" {
		b.Icon = defaultIcon(b.Title, b.URL)
	}
	isNew := b.ID == ""
	if !isNew && a.Store.Lib().Bookmark(b.ID) == nil {
		return ""
	}
	a.Store.Update(func(l *model.Library) {
		if isNew {
			b.ID = model.NewID()
			b.CreatedAt = now
			b.Order = nextOrder(l, b.GroupID, b.SubID)
		}
		b.UpdatedAt = now
		if old := l.Bookmark(b.ID); old != nil {
			if old.GroupID != b.GroupID || old.SubID != b.SubID {
				b.Order = nextOrder(l, b.GroupID, b.SubID)
			}
			*old = b
		} else {
			l.Bookmarks = append(l.Bookmarks, b)
		}
	})
	if isNew {
		if toast {
			a.Say(locales.ToastSavedTo(a.PathOf(&b)))
		}
		if autoPolish {
			a.Polish(b.ID, false)
		}
	}
	return b.ID
}

// QuickAddFromClipboard saves the URL currently in the system clipboard.
// Returns true when a URL was found and saved (or already existed).
func (a *App) QuickAddFromClipboard() bool {
	u := extractFirstURL(mygo.Clipboard.ReadText())
	if u == "" {
		a.Say(locales.ToastClipboardNoURL())
		return false
	}
	a.QuickAddURL(u, "", "", "", false)
	return true
}

// QuickAddURL saves rawURL into the requested group/sub (or the active place
// when empty), then fetches its page metadata and icon in the background
// before running AI auto-polish. When silent is true or the app is not active,
// a desktop notification confirms the save.
func (a *App) QuickAddURL(rawURL, title, groupName, subName string, silent bool) string {
	cleanURL := normalizeQuickURL(rawURL)
	if cleanURL == "" {
		return ""
	}
	lib := a.Store.Lib()
	if existing := lib.FindURL(cleanURL); existing != nil {
		msg := locales.ToastAlreadyIn(a.PathOf(existing))
		a.Say(msg)
		if silent || !mygo.App.IsActive() {
			a.Notify("quicksave-"+existing.ID, locales.NotifyQuickSaveTitle(), msg, func() {
				a.Go(existing.GroupID, existing.SubID)
				if a.Win.ShowMain != nil {
					a.Win.ShowMain()
				}
			})
		}
		return existing.ID
	}

	groupID, subID := a.resolveTargetPlace(groupName, subName)
	return a.QuickAddToPlace(cleanURL, title, groupID, subID, silent)
}

// QuickAddToPlace saves cleanURL directly into groupID/subID (resolving fallbacks
// if missing) and runs background metadata fetch + AI auto-polish.
func (a *App) QuickAddToPlace(rawURL, title, groupID, subID string, silent bool) string {
	cleanURL := normalizeQuickURL(rawURL)
	if cleanURL == "" {
		return ""
	}
	lib := a.Store.Lib()
	if existing := lib.FindURL(cleanURL); existing != nil {
		a.Say(locales.ToastAlreadyIn(a.PathOf(existing)))
		return existing.ID
	}

	if g := lib.Group(groupID); g == nil {
		if len(lib.Groups) > 0 {
			groupID = lib.Groups[0].ID
		} else {
			groupID = a.AddGroup(locales.DefaultGroup())
		}
	}
	if a.Store.Lib().Sub(groupID, subID) == nil {
		if g := a.Store.Lib().Group(groupID); g != nil && len(g.Subs) > 0 {
			subID = g.Subs[0].ID
		} else {
			subID = a.AddSub(groupID, locales.DefaultSub())
		}
	}

	fallbackTitle := strings.TrimSpace(title)
	hasExplicitTitle := fallbackTitle != ""
	if fallbackTitle == "" {
		fallbackTitle = hostFallbackTitle(cleanURL)
	}

	id := a.saveBookmark(model.Bookmark{
		Title:   fallbackTitle,
		URL:     cleanURL,
		GroupID: groupID,
		SubID:   subID,
	}, true, false)
	if id == "" {
		return ""
	}

	path := a.PathOf(a.Store.Lib().Bookmark(id))
	if silent || !mygo.App.IsActive() {
		a.Notify("quicksave-"+id, locales.NotifyQuickSaveTitle(), locales.NotifyQuickSaveBody(fallbackTitle, path), func() {
			a.Go(groupID, subID)
			if a.Win.ShowMain != nil {
				a.Win.ShowMain()
			}
		})
	}

	a.EnrichBookmark(id, EnrichOptions{
		Fetch:         a.Fetch != nil,
		AllowTitle:    !hasExplicitTitle,
		AllowNote:     true,
		AllowPolish:   true,
		FallbackTitle: fallbackTitle,
	})
	return id
}

// EnrichBookmark fetches page metadata/icon (when requested) and then runs
// background AI auto-polish for bookmark id without blocking the UI.
func (a *App) EnrichBookmark(id string, opts EnrichOptions) {
	b := a.Store.Lib().Bookmark(id)
	if b == nil || b.InTrash() {
		return
	}
	if !opts.Fetch || a.Fetch == nil {
		if opts.AllowPolish {
			a.polishBookmark(id, polishOptions{
				force:          false,
				allowTitle:     opts.AllowTitle,
				allowNote:      opts.AllowNote,
				fallbackTitle:  opts.FallbackTitle,
				fetchedExcerpt: strings.TrimSpace(opts.FetchedExcerpt),
				fetchedOK:      true,
			})
		}
		return
	}

	dir := a.Store.Dir()
	targetURL := b.URL
	expectedUpdated := b.UpdatedAt
	a.startEnriching(id)

	Background(20*time.Second, func(ctx context.Context) {
		meta, err := a.Fetch.Fetch(ctx, targetURL)
		a.Post(func() {
			defer a.stopEnriching(id)
			cur := a.Store.Lib().Bookmark(id)
			if a.Store.Dir() != dir || cur == nil || cur.InTrash() || cur.URL != targetURL || !cur.UpdatedAt.Equal(expectedUpdated) {
				return
			}
			fetchedTitle := ""
			fetchedDesc := ""
			fetchedExcerpt := ""
			if err == nil {
				fetchedTitle = strings.TrimSpace(meta.Title)
				fetchedDesc = strings.TrimSpace(meta.Description)
				fetchedExcerpt = strings.TrimSpace(meta.Excerpt)
				var iconRel string
				if len(meta.Icon) > 0 && cur.Icon.Image == "" {
					ext := meta.IconExt
					if ext == "" {
						ext = ".png"
					}
					if savedPath, saveErr := a.Store.SaveIcon(meta.Icon, ext); saveErr == nil {
						iconRel = savedPath
					}
				}
				a.Store.Update(func(l *model.Library) {
					bm := l.Bookmark(id)
					if bm == nil {
						return
					}
					changed := false
					if fetchedTitle != "" && opts.AllowTitle && isReplaceableTitle(bm.Title, bm.URL, opts.FallbackTitle) {
						bm.Title = fetchedTitle
						changed = true
					}
					if fetchedDesc != "" && opts.AllowNote && strings.TrimSpace(bm.Note) == "" {
						bm.Note = fetchedDesc
						changed = true
					}
					if iconRel != "" && bm.Icon.Image == "" {
						bm.Icon.Image = iconRel
						changed = true
					}
					if changed {
						bm.UpdatedAt = time.Now()
					}
				})
			}
			if opts.AllowPolish {
				a.polishBookmark(id, polishOptions{
					force:          false,
					allowTitle:     opts.AllowTitle,
					allowNote:      opts.AllowNote,
					fallbackTitle:  opts.FallbackTitle,
					fetchedTitle:   fetchedTitle,
					fetchedDesc:    fetchedDesc,
					fetchedExcerpt: fetchedExcerpt,
					fetchedOK:      err == nil,
				})
			}
		})
	})
}

func (a *App) resolveTargetPlace(groupName, subName string) (string, string) {
	groupName = strings.TrimSpace(groupName)
	subName = strings.TrimSpace(subName)
	lib := a.Store.Lib()

	var groupID, subID string
	if groupName != "" {
		for _, g := range lib.Groups {
			if strings.EqualFold(strings.TrimSpace(g.Name), groupName) {
				groupID = g.ID
				break
			}
		}
		if groupID == "" {
			subLabel := subName
			if subLabel == "" {
				subLabel = locales.DefaultSub()
			}
			now := time.Now()
			gid, sid := model.NewID(), model.NewID()
			a.Store.Update(func(l *model.Library) {
				order := 1.0
				if n := len(l.Groups); n > 0 {
					order = l.Groups[n-1].Order + 1
				}
				l.Groups = append(l.Groups, model.Group{
					ID:        gid,
					Name:      groupName,
					Order:     order,
					UpdatedAt: now,
					Subs:      []model.Sub{{ID: sid, Name: subLabel, Order: 1, UpdatedAt: now}},
				})
			})
			return gid, sid
		}
	} else {
		a.EnsurePlace()
		groupID, subID = a.Group, a.Sub
	}

	if groupID != "" && subName != "" {
		if g := a.Store.Lib().Group(groupID); g != nil {
			for _, s := range g.Subs {
				if strings.EqualFold(strings.TrimSpace(s.Name), subName) {
					subID = s.ID
					break
				}
			}
			if subID == "" {
				subID = a.AddSub(groupID, subName)
			}
		}
	}
	return groupID, subID
}

func normalizeQuickURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsAny(s, " \t\n\r") {
		return ""
	}
	low := strings.ToLower(s)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		if !looksLikeBareDomain(s) {
			return ""
		}
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return s
}

func extractFirstURL(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if u := normalizeQuickURL(line); u != "" {
			return u
		}
	}
	return ""
}

func looksLikeBareDomain(q string) bool {
	dot := strings.IndexByte(q, '.')
	if dot <= 0 || dot >= len(q)-1 {
		return false
	}
	host := q
	if slash := strings.IndexAny(host, "/?#"); slash >= 0 {
		host = host[:slash]
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return false
	}
	tld := parts[len(parts)-1]
	return len(tld) >= 2 && !strings.ContainsAny(tld, "0123456789")
}

func hostFallbackTitle(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host != "" {
		return host
	}
	return raw
}

// defaultIcon is set by the theme package (avoids an import cycle).
var defaultIcon = func(title, url string) model.Icon { return model.Icon{Letter: "?", Color: "#2b2a27"} }

// SetDefaultIcon wires theme.DefaultIcon.
func SetDefaultIcon(fn func(title, url string) model.Icon) { defaultIcon = fn }

func nextOrder(l *model.Library, g, s string) float64 {
	max := 0.0
	for _, b := range l.In(g, s) {
		if b.Order > max {
			max = b.Order
		}
	}
	return max + 1
}

type polishOptions struct {
	force          bool
	allowTitle     bool
	allowNote      bool
	fallbackTitle  string
	fetchedTitle   string
	fetchedDesc    string
	fetchedExcerpt string
	fetchedOK      bool
}

// Polish asks the AI to refine a saved bookmark's title and note without
// moving it out of its user-chosen group. Manual re-polish fetches fresh
// webpage metadata/content first so AI does not guess or recycle stale notes.
func (a *App) Polish(id string, force bool) {
	if a.AI == nil || (!force && !a.Settings.AIAutoPolish) || !a.AI.Status().Ready {
		return
	}
	b := a.Store.Lib().Bookmark(id)
	if b == nil || b.InTrash() || (b.Polished && !force) {
		return
	}
	if force && a.Fetch != nil && !isLocalOrIPURL(b.URL) && !b.IsTemplate() {
		dir := a.Store.Dir()
		targetURL := b.URL
		expectedUpdated := b.UpdatedAt
		a.startEnriching(id)
		Background(20*time.Second, func(ctx context.Context) {
			meta, err := a.Fetch.Fetch(ctx, targetURL)
			a.Post(func() {
				defer a.stopEnriching(id)
				cur := a.Store.Lib().Bookmark(id)
				if a.Store.Dir() != dir || cur == nil || cur.InTrash() || cur.URL != targetURL || !cur.UpdatedAt.Equal(expectedUpdated) {
					return
				}
				var fetchedTitle, fetchedDesc, fetchedExcerpt string
				if err == nil {
					fetchedTitle = strings.TrimSpace(meta.Title)
					fetchedDesc = strings.TrimSpace(meta.Description)
					fetchedExcerpt = strings.TrimSpace(meta.Excerpt)
					if len(meta.Icon) > 0 && cur.Icon.Image == "" {
						ext := meta.IconExt
						if ext == "" {
							ext = ".png"
						}
						if savedPath, saveErr := a.Store.SaveIcon(meta.Icon, ext); saveErr == nil {
							a.Store.Update(func(l *model.Library) {
								if bm := l.Bookmark(id); bm != nil && bm.Icon.Image == "" {
									bm.Icon.Image = savedPath
									bm.UpdatedAt = time.Now()
								}
							})
						}
					}
				}
				a.polishBookmark(id, polishOptions{
					force:          true,
					allowTitle:     true,
					allowNote:      true,
					fetchedTitle:   fetchedTitle,
					fetchedDesc:    fetchedDesc,
					fetchedExcerpt: fetchedExcerpt,
					fetchedOK:      err == nil,
				})
			})
		})
		return
	}
	a.polishBookmark(id, polishOptions{
		force:      force,
		allowTitle: force,
		allowNote:  force,
		fetchedOK:  true,
	})
}

func (a *App) polishBookmark(id string, opts polishOptions) {
	if a.AI == nil || (!opts.force && !a.Settings.AIAutoPolish) || !a.AI.Status().Ready {
		return
	}
	b := a.Store.Lib().Bookmark(id)
	if b == nil || b.InTrash() || (b.Polished && !opts.force) {
		return
	}

	title := strings.TrimSpace(b.Title)
	if ft := strings.TrimSpace(opts.fetchedTitle); ft != "" {
		if isReplaceableTitle(title, b.URL, opts.fallbackTitle) || (opts.force && len([]rune(ft)) > len([]rune(title))) {
			title = ft
		}
	}

	desc := strings.TrimSpace(opts.fetchedDesc)
	if IsJunkAINote(desc) {
		desc = ""
	}
	// On manual re-polish of an already-polished bookmark, avoid feeding the
	// old AI-generated note back in as ground truth when fresh fetch failed.
	if desc == "" && (!opts.force || !b.Polished) {
		if curNote := strings.TrimSpace(b.Note); curNote != "" && !IsJunkAINote(curNote) {
			desc = curNote
		}
	}
	excerpt := strings.TrimSpace(opts.fetchedExcerpt)

	grounded := HasGroundedNoteContext(b.URL, title, desc, excerpt)
	needsTitle := TitleNeedsCleanup(title, b.URL)
	isLocalEngine := a.Settings.AIEngine == model.AILocal

	if isLocalEngine && !grounded && !needsTitle {
		if opts.force {
			if IsJunkAINote(b.Note) {
				a.Store.Update(func(l *model.Library) {
					if bm := l.Bookmark(id); bm != nil {
						bm.Note = ""
						bm.UpdatedAt = time.Now()
					}
				})
			}
			a.Say(locales.ToastPolishInsufficient())
		}
		return
	}
	if !opts.force {
		if isLocalOrIPURL(b.URL) && !grounded && !needsTitle {
			return
		}
		if !opts.fetchedOK && !grounded && !needsTitle {
			return
		}
	}

	dir, updated := a.Store.Dir(), b.UpdatedAt
	// Do not pass group choices here: saved bookmarks already live in the
	// user's chosen group, and omitting choices keeps small local models
	// focused on clean title & note generation.
	in := PolishInput{
		URL:         b.URL,
		Title:       title,
		Description: desc,
		Excerpt:     excerpt,
	}
	a.startEnriching(id)
	Background(60*time.Second, func(ctx context.Context) {
		p, err := a.AI.Polish(ctx, in)
		a.Post(func() {
			defer a.stopEnriching(id)
			current := a.Store.Lib().Bookmark(id)
			if current == nil || current.InTrash() || a.Store.Dir() != dir || !current.UpdatedAt.Equal(updated) {
				return
			}
			if err != nil {
				if opts.force {
					a.SayErr(locales.ToastPolishFailed(), locales.ActionRetry(), func() { a.Polish(id, true) })
				}
				return
			}
			p.Title = strings.TrimSpace(p.Title)
			p.Note = strings.TrimSpace(p.Note)
			updatedAny := false
			a.Store.Update(func(l *model.Library) {
				bm := l.Bookmark(id)
				if bm == nil {
					return
				}
				if p.Title != "" && (opts.force || opts.allowTitle || isReplaceableTitle(bm.Title, bm.URL, opts.fallbackTitle)) {
					if bm.Title != p.Title {
						bm.Title = p.Title
						updatedAny = true
					}
				}
				if p.Note != "" && (opts.force || opts.allowNote || strings.TrimSpace(bm.Note) == "" || IsJunkAINote(bm.Note)) {
					if bm.Note != p.Note {
						bm.Note = p.Note
						updatedAny = true
					}
				} else if opts.force && p.Note == "" && IsJunkAINote(bm.Note) {
					bm.Note = ""
					updatedAny = true
				}
				bm.Polished = true
				bm.UpdatedAt = time.Now()
			})
			if opts.force && !updatedAny && !grounded {
				a.Say(locales.ToastPolishInsufficient())
				return
			}
			completed := time.Now()
			a.PolishedAt[id] = completed
			time.AfterFunc(1500*time.Millisecond, func() {
				a.Post(func() {
					if a.PolishedAt[id] == completed {
						delete(a.PolishedAt, id)
					}
				})
			})
		})
	})
}

// HasGroundedNoteContext reports whether the available metadata contains actual
// webpage text or a descriptive title subtitle, rather than only a bare URL and
// brand/site name. Small local models should not invent notes when this is false.
func HasGroundedNoteContext(rawURL, title, desc, excerpt string) bool {
	if d := strings.TrimSpace(desc); d != "" && !IsJunkAINote(d) {
		return true
	}
	if strings.TrimSpace(excerpt) != "" {
		return true
	}
	t := strings.TrimSpace(title)
	if t == "" || isReplaceableTitle(t, rawURL, "") || sameAsHostBrand(t, rawURL) {
		return false
	}
	for _, sep := range []string{" - ", " | ", " — ", " – ", " :: ", ": ", "：", " · "} {
		if strings.Contains(t, sep) {
			parts := strings.Split(t, sep)
			for _, part := range parts {
				p := strings.TrimSpace(part)
				if p == "" || sameAsHostBrand(p, rawURL) || isGenericSiteSuffix(p) {
					continue
				}
				if countCJK(p) >= 4 || len(strings.Fields(p)) >= 3 {
					return true
				}
			}
		}
	}
	return countCJK(t) >= 18 || len(strings.Fields(t)) >= 7
}

// TitleNeedsCleanup reports whether title looks noisy, missing, or contains
// site suffixes that AI title cleanup can usefully simplify.
func TitleNeedsCleanup(title, rawURL string) bool {
	t := strings.TrimSpace(title)
	if t == "" || isReplaceableTitle(t, rawURL, "") {
		return true
	}
	if len([]rune(t)) > 24 {
		return true
	}
	for _, sep := range []string{" - ", " | ", " — ", " – ", " :: ", ": ", "：", "_"} {
		if strings.Contains(t, sep) {
			return true
		}
	}
	return false
}

func sameAsHostBrand(title, rawURL string) bool {
	normTitle := alphaNumLower(title)
	if normTitle == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(u.Hostname()), "www."))
	if host == "" {
		return false
	}
	if normTitle == alphaNumLower(host) {
		return true
	}
	if parts := strings.Split(host, "."); len(parts) >= 2 {
		if normTitle == alphaNumLower(parts[0]) {
			return true
		}
	}
	return false
}

func isGenericSiteSuffix(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "官网", "官方网站", "首页", "主页", "登录", "注册", "home", "homepage", "official", "official site", "official website", "login", "sign in":
		return true
	}
	return false
}

func alphaNumLower(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func countCJK(s string) int {
	n := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			n++
		}
	}
	return n
}

func isReplaceableTitle(title, rawURL, fallbackTitle string) bool {
	t := strings.TrimSpace(title)
	if t == "" || t == rawURL {
		return true
	}
	if fallbackTitle != "" && strings.EqualFold(t, strings.TrimSpace(fallbackTitle)) {
		return true
	}
	host := hostFallbackTitle(rawURL)
	return host != "" && strings.EqualFold(t, host)
}

func isLocalOrIPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}
	return net.ParseIP(host) != nil
}

// IsJunkAINote reports whether note looks like meta-commentary about grouping
// or an empty placeholder rather than an actual webpage summary.
func IsJunkAINote(note string) bool {
	s := strings.TrimSpace(note)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	junkSubstrings := []string{
		"无需特殊分组",
		"无需分组",
		"不需要特殊分组",
		"不需要分组",
		"无匹配分组",
		"没有匹配的分组",
		"没有合适分组",
		"未找到匹配分组",
		"无需归类",
		"属于通用功能",
		"可选分组",
		"分组路径",
		"留空",
		"空字符串",
		"暂无简介",
		"暂无备注",
		"无备注",
		"无简介",
		"一句话中文备注",
		"no special group",
		"no matching group",
		"no suitable group",
		"no group needed",
		"empty string",
		"one-sentence note",
		"general function",
	}
	for _, sub := range junkSubstrings {
		if strings.Contains(low, sub) {
			return true
		}
	}
	switch low {
	case "无", "暂无", "未知", "n/a", "none", "null", "unknown", "-":
		return true
	}
	return false
}

// Move moves bookmarks to a sub-group (single membership: they leave
// where they were), with undo.
func (a *App) Move(ids []string, groupID, subID string) {
	if a.Store.Lib().Sub(groupID, subID) == nil {
		a.SayErr(locales.ToastTargetSubMissing(), "", nil)
		return
	}
	type was struct {
		g, s string
		o    float64
	}
	before := map[string]was{}
	a.Store.Update(func(l *model.Library) {
		next := nextOrder(l, groupID, subID)
		for _, id := range ids {
			if b := l.Bookmark(id); b != nil {
				before[id] = was{b.GroupID, b.SubID, b.Order}
				b.GroupID, b.SubID, b.Order = groupID, subID, next
				b.UpdatedAt = time.Now()
				next++
			}
		}
	})
	if len(before) == 0 {
		return
	}
	path := ""
	if g := a.Store.Lib().Group(groupID); g != nil {
		path = g.Name
		if s := a.Store.Lib().Sub(groupID, subID); s != nil {
			path += " / " + s.Name
		}
	}
	text := locales.ToastMovedTo(path)
	if len(ids) > 1 {
		text = locales.ToastMovedCount(len(ids))
	}
	a.SayUndo(text, func() {
		a.Store.Update(func(l *model.Library) {
			for id, w := range before {
				if b := l.Bookmark(id); b != nil {
					b.GroupID, b.SubID, b.Order = w.g, w.s, w.o
					b.UpdatedAt = time.Now()
				}
			}
		})
	})
}

// Reorder puts a selection before another bookmark in its sub-group
// ("" = at the end), preserving the selection's relative order.
func (a *App) Reorder(ids []string, groupID, subID, beforeID string) {
	if slices.Contains(ids, beforeID) {
		return
	}
	a.Store.Update(func(l *model.Library) {
		items := l.In(groupID, subID)
		moved := make(map[string]bool, len(ids))
		for _, id := range ids {
			moved[id] = true
		}
		var moving, remaining []*model.Bookmark
		insertAt := -1
		for _, b := range items {
			if moved[b.ID] {
				moving = append(moving, b)
				continue
			}
			if b.ID == beforeID {
				insertAt = len(remaining)
			}
			remaining = append(remaining, b)
		}
		if insertAt < 0 {
			insertAt = len(remaining)
		}
		ordered := slices.Insert(remaining, insertAt, moving...)
		now := time.Now()
		for i, b := range ordered {
			if order := float64(i + 1); b.Order != order {
				b.Order = order
				b.UpdatedAt = now
			}
		}
	})
}

// Delete removes bookmarks and offers instant undo via toast / Cmd+Z.
func (a *App) Delete(ids ...string) {
	if len(ids) == 0 {
		return
	}
	gone := make(map[string]bool, len(ids))
	for _, id := range ids {
		gone[id] = true
	}
	var saved []model.Bookmark
	a.Store.Update(func(l *model.Library) {
		for _, b := range l.Bookmarks {
			if gone[b.ID] {
				saved = append(saved, b)
			}
		}
		store.Purge(l, ids...)
	})
	if len(saved) == 0 {
		return
	}
	text := locales.ToastDeletedOne(saved[0].Title)
	if len(saved) > 1 {
		text = locales.ToastDeletedCount(len(saved))
	}
	a.SayUndo(text, func() { a.restoreSaved(saved) })
}

func (a *App) restoreSaved(saved []model.Bookmark) {
	if len(saved) == 0 {
		return
	}
	now := time.Now()
	a.Store.Update(func(l *model.Library) {
		for _, b := range saved {
			b.DeletedAt = nil
			b.UpdatedAt = now
			if l.Sub(b.GroupID, b.SubID) == nil && len(l.Groups) > 0 && len(l.Groups[0].Subs) > 0 {
				b.GroupID = l.Groups[0].ID
				b.SubID = l.Groups[0].Subs[0].ID
				b.Order = nextOrder(l, b.GroupID, b.SubID)
			}
			l.Bookmarks = append(l.Bookmarks, b)
			dropTomb(l, b.ID)
		}
	})
}

// ---- groups ----

// AddGroup creates a group with one sub-group and shows it.
func (a *App) AddGroup(name string) string {
	now := time.Now()
	g := model.Group{ID: model.NewID(), Name: name, UpdatedAt: now,
		Subs: []model.Sub{{ID: model.NewID(), Name: locales.DefaultSub(), UpdatedAt: now}}}
	a.Store.Update(func(l *model.Library) {
		g.Order = float64(len(l.Groups) + 1)
		if n := len(l.Groups); n > 0 {
			g.Order = l.Groups[n-1].Order + 1
		}
		l.Groups = append(l.Groups, g)
	})
	a.Go(g.ID, "")
	return g.ID
}

// AddSub creates a sub-group in a group.
func (a *App) AddSub(groupID, name string) string {
	if a.Store.Lib().Group(groupID) == nil {
		return ""
	}
	now := time.Now()
	id := model.NewID()
	a.Store.Update(func(l *model.Library) {
		if g := l.Group(groupID); g != nil {
			order := 1.0
			if n := len(g.Subs); n > 0 {
				order = g.Subs[n-1].Order + 1
			}
			g.Subs = append(g.Subs, model.Sub{ID: id, Name: name, Order: order, UpdatedAt: now})
			g.UpdatedAt = now
		}
	})
	return id
}

// RenameGroup renames a group.
func (a *App) RenameGroup(id, name string) {
	a.Store.Update(func(l *model.Library) {
		if g := l.Group(id); g != nil && strings.TrimSpace(name) != "" {
			g.Name, g.UpdatedAt = strings.TrimSpace(name), time.Now()
		}
	})
}

// RenameSub renames a sub-group.
func (a *App) RenameSub(groupID, subID, name string) {
	a.Store.Update(func(l *model.Library) {
		if s := l.Sub(groupID, subID); s != nil && strings.TrimSpace(name) != "" {
			s.Name, s.UpdatedAt = strings.TrimSpace(name), time.Now()
			l.Group(groupID).UpdatedAt = time.Now()
		}
	})
}

// CountIn counts live bookmarks of a group ("" sub = the whole group).
func (a *App) CountIn(groupID, subID string) int {
	n := 0
	for _, b := range a.Store.Lib().Live() {
		if b.GroupID == groupID && (subID == "" || b.SubID == subID) {
			n++
		}
	}
	return n
}

// DeleteGroup deletes a group, its sub-groups and its bookmarks; undo brings all of it back.
func (a *App) DeleteGroup(id string) {
	lib := a.Store.Lib()
	g := lib.Group(id)
	if g == nil {
		return
	}
	saved := *g
	saved.Subs = append([]model.Sub(nil), g.Subs...)
	idx := 0
	for i := range lib.Groups {
		if lib.Groups[i].ID == id {
			idx = i
		}
	}
	removed := a.purgeWhere(func(b *model.Bookmark) bool { return b.GroupID == id })
	a.Store.Update(func(l *model.Library) {
		out := make([]model.Group, 0, len(l.Groups))
		for _, x := range l.Groups {
			if x.ID != id {
				out = append(out, x)
			}
		}
		l.Groups = out
		l.Tombstones = append(l.Tombstones, model.Tombstone{ID: id, DeletedAt: time.Now()})
	})
	a.EnsurePlace()
	a.SayUndo(locales.ToastDeletedOne(saved.Name), func() {
		a.Store.Update(func(l *model.Library) {
			saved.UpdatedAt = time.Now()
			idx = min(idx, len(l.Groups))
			l.Groups = append(l.Groups[:idx], append([]model.Group{saved}, l.Groups[idx:]...)...)
			dropTomb(l, id)
		})
		a.restoreSaved(removed)
	})
}

// DeleteSub deletes a sub-group and its bookmarks, with undo.
func (a *App) DeleteSub(groupID, subID string) {
	lib := a.Store.Lib()
	s := lib.Sub(groupID, subID)
	if s == nil {
		return
	}
	saved := *s
	removed := a.purgeWhere(func(b *model.Bookmark) bool { return b.GroupID == groupID && b.SubID == subID })
	a.Store.Update(func(l *model.Library) {
		if g := l.Group(groupID); g != nil {
			out := make([]model.Sub, 0, len(g.Subs))
			for _, x := range g.Subs {
				if x.ID != subID {
					out = append(out, x)
				}
			}
			g.Subs = out
			g.UpdatedAt = time.Now()
		}
		l.Tombstones = append(l.Tombstones, model.Tombstone{ID: subID, DeletedAt: time.Now()})
	})
	a.EnsurePlace()
	a.SayUndo(locales.ToastDeletedOne(saved.Name), func() {
		a.Store.Update(func(l *model.Library) {
			if g := l.Group(groupID); g != nil {
				saved.UpdatedAt = time.Now()
				g.Subs = append(g.Subs, saved)
				sortSubs(g.Subs)
				g.UpdatedAt = time.Now()
			}
			dropTomb(l, subID)
		})
		a.restoreSaved(removed)
	})
}

// MoveSub moves a sub-group (with its bookmarks) into another group.
func (a *App) MoveSub(fromGroup, subID, toGroup string) {
	a.Store.Update(func(l *model.Library) {
		from, to := l.Group(fromGroup), l.Group(toGroup)
		if from == nil || to == nil || from == to {
			return
		}
		var moved *model.Sub
		out := from.Subs[:0]
		for i := range from.Subs {
			if from.Subs[i].ID == subID {
				s := from.Subs[i]
				moved = &s
				continue
			}
			out = append(out, from.Subs[i])
		}
		from.Subs = out
		if moved == nil {
			return
		}
		now := time.Now()
		moved.UpdatedAt = now
		moved.Order = float64(len(to.Subs) + 1)
		to.Subs = append(to.Subs, *moved)
		from.UpdatedAt, to.UpdatedAt = now, now
		for i := range l.Bookmarks {
			if b := &l.Bookmarks[i]; b.GroupID == fromGroup && b.SubID == subID {
				b.GroupID, b.UpdatedAt = toGroup, now
			}
		}
	})
}

// PromoteSub turns a sub-group into a group of its own (its bookmarks
// keep their sub-group, renamed "常用" under the new group).
func (a *App) PromoteSub(groupID, subID string) {
	a.Store.Update(func(l *model.Library) {
		from := l.Group(groupID)
		s := l.Sub(groupID, subID)
		if from == nil || s == nil {
			return
		}
		now := time.Now()
		ng := model.Group{ID: model.NewID(), Name: s.Name, UpdatedAt: now,
			Subs: []model.Sub{{ID: subID, Name: locales.DefaultSub(), UpdatedAt: now}}}
		if n := len(l.Groups); n > 0 {
			ng.Order = l.Groups[n-1].Order + 1
		}
		out := from.Subs[:0]
		for _, x := range from.Subs {
			if x.ID != subID {
				out = append(out, x)
			}
		}
		from.Subs, from.UpdatedAt = out, now
		l.Groups = append(l.Groups, ng)
		for i := range l.Bookmarks {
			if b := &l.Bookmarks[i]; b.GroupID == groupID && b.SubID == subID {
				b.GroupID, b.UpdatedAt = ng.ID, now
			}
		}
	})
}

// ReorderGroups sets the groups' order to ids.
func (a *App) ReorderGroups(ids []string) {
	a.Store.Update(func(l *model.Library) {
		pos := map[string]int{}
		for i, id := range ids {
			pos[id] = i
		}
		now := time.Now()
		for i := range l.Groups {
			if p, ok := pos[l.Groups[i].ID]; ok {
				l.Groups[i].Order, l.Groups[i].UpdatedAt = float64(p+1), now
			}
		}
		sortGroups(l.Groups)
	})
}

// ReorderSubs sets a group's sub-groups' order to ids.
func (a *App) ReorderSubs(groupID string, ids []string) {
	a.Store.Update(func(l *model.Library) {
		g := l.Group(groupID)
		if g == nil {
			return
		}
		pos := map[string]int{}
		for i, id := range ids {
			pos[id] = i
		}
		now := time.Now()
		for i := range g.Subs {
			if p, ok := pos[g.Subs[i].ID]; ok {
				g.Subs[i].Order, g.Subs[i].UpdatedAt = float64(p+1), now
			}
		}
		sortSubs(g.Subs)
		g.UpdatedAt = now
	})
}

func (a *App) purgeWhere(match func(*model.Bookmark) bool) []model.Bookmark {
	var removed []model.Bookmark
	var ids []string
	a.Store.Update(func(l *model.Library) {
		for _, b := range l.Bookmarks {
			if !b.InTrash() && match(&b) {
				removed = append(removed, b)
				ids = append(ids, b.ID)
			}
		}
		if len(ids) > 0 {
			store.Purge(l, ids...)
		}
	})
	return removed
}

func dropTomb(l *model.Library, id string) {
	out := make([]model.Tombstone, 0, len(l.Tombstones))
	for _, t := range l.Tombstones {
		if t.ID != id {
			out = append(out, t)
		}
	}
	l.Tombstones = out
}

func sortSubs(s []model.Sub) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Order < s[j-1].Order; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func sortGroups(g []model.Group) {
	for i := 1; i < len(g); i++ {
		for j := i; j > 0 && g[j].Order < g[j-1].Order; j-- {
			g[j], g[j-1] = g[j-1], g[j]
		}
	}
}
