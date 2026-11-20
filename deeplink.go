package main

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
)

var (
	launchMu        sync.Mutex
	launchReady     bool
	launchApp       *app.App
	pendingURLs     []string
	pendingFiles    []string
	pendingNotifIDs []string
)

type launchArgsSummary struct {
	hasRealTarget bool
	missingFiles  []string
}

func queueOrHandleURL(raw string) {
	launchMu.Lock()
	if !launchReady || launchApp == nil {
		pendingURLs = append(pendingURLs, raw)
		launchMu.Unlock()
		return
	}
	a := launchApp
	launchMu.Unlock()
	handleDeepLink(a, raw)
}

func queueOrHandleFile(path string) {
	launchMu.Lock()
	if !launchReady || launchApp == nil {
		pendingFiles = append(pendingFiles, path)
		launchMu.Unlock()
		return
	}
	a := launchApp
	launchMu.Unlock()
	handleOpenFile(a, path)
}

func queueOrHandleNotificationClick(id string) {
	launchMu.Lock()
	if !launchReady || launchApp == nil {
		pendingNotifIDs = append(pendingNotifIDs, id)
		launchMu.Unlock()
		return
	}
	a := launchApp
	launchMu.Unlock()
	a.HandleNotificationClick(id)
}

func flushPendingLaunchEvents(a *app.App) bool {
	launchMu.Lock()
	launchApp = a
	launchReady = true
	urls := pendingURLs
	files := pendingFiles
	notifs := pendingNotifIDs
	pendingURLs = nil
	pendingFiles = nil
	pendingNotifIDs = nil
	launchMu.Unlock()

	handled := len(urls) > 0 || len(files) > 0 || len(notifs) > 0
	for _, u := range urls {
		handleDeepLink(a, u)
	}
	for _, f := range files {
		handleOpenFile(a, f)
	}
	for _, id := range notifs {
		a.HandleNotificationClick(id)
	}
	return handled
}

func inspectLaunchArgs(args []string, wd string) launchArgsSummary {
	var out launchArgsSummary
	if wd == "" {
		wd, _ = os.Getwd()
	}
	for _, raw := range args {
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "-") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(s), "coooolmark:") {
			if u, err := url.Parse(s); err == nil && strings.EqualFold(u.Scheme, "coooolmark") {
				if _, qErr := url.ParseQuery(u.RawQuery); qErr == nil {
					out.hasRealTarget = true
				}
			}
			continue
		}
		if p, ok := resolveImportableFilePath(s, wd); ok {
			if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
				out.hasRealTarget = true
			} else {
				out.missingFiles = append(out.missingFiles, p)
			}
		}
	}
	return out
}

func resolveImportableFilePath(raw, wd string) (string, bool) {
	p := strings.TrimSpace(raw)
	if u, err := url.Parse(p); err == nil && strings.EqualFold(u.Scheme, "file") {
		p = filepath.FromSlash(u.Path)
		if runtime.GOOS == "windows" {
			p = strings.TrimPrefix(p, `\`)
		}
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".html", ".htm", ".json":
	default:
		return "", false
	}
	if !filepath.IsAbs(p) && wd != "" {
		p = filepath.Join(wd, p)
	}
	return p, true
}

func validateExternalHTTPURL(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsAny(s, " \t\n\r") {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	return u.String(), true
}

func ensureVisibleForError(a *app.App) {
	if a != nil && !a.HasRegularWindow() && a.Win.ShowMain != nil {
		a.Win.ShowMain()
	}
}

func handleOpenFile(a *app.App, path string) {
	if a == nil || strings.TrimSpace(path) == "" {
		return
	}
	wd, _ := os.Getwd()
	resolved, ok := resolveImportableFilePath(path, wd)
	if !ok {
		ensureVisibleForError(a)
		a.SayErr(locales.ImportUnsupportedExt(), "", nil)
		return
	}
	if _, err := os.Stat(resolved); os.IsNotExist(err) {
		ensureVisibleForError(a)
		a.SayErr(locales.ToastFileMissing(filepath.Base(resolved)), "", nil)
		return
	}
	if a.Win.ShowImport != nil {
		a.Win.ShowImport(resolved)
	}
}

// handleDeepLink handles coooolmark:// URLs:
//   - coooolmark://open
//   - coooolmark://search?q=...
//   - coooolmark://quicksearch
//   - coooolmark://add?url=https://...&title=...&group=...&sub=...
//   - coooolmark://settings?page=general|ai|data
//   - coooolmark://toggle?mode=notch|edge|tray|quicksearch
//
// External add links always open a draft in the editor for explicit user
// confirmation — they never silently write to the library, create groups,
// or trigger automatic background network/AI fetches.
func handleDeepLink(a *app.App, raw string) {
	if a == nil {
		return
	}
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || !strings.EqualFold(u.Scheme, "coooolmark") {
		if strings.HasPrefix(strings.ToLower(trimmed), "coooolmark:") {
			ensureVisibleForError(a)
			a.SayErr(locales.EditorErrURLInvalid(), "", nil)
		}
		return
	}
	action := strings.ToLower(strings.TrimSpace(u.Host))
	if action == "" {
		action = strings.ToLower(strings.Trim(u.Path, "/"))
	}
	q, qErr := url.ParseQuery(u.RawQuery)
	if qErr != nil {
		ensureVisibleForError(a)
		a.SayErr(locales.EditorErrURLInvalid(), "", nil)
		return
	}

	switch action {
	case "", "open", "main":
		if a.Win.ShowMain != nil {
			a.Win.ShowMain()
		}

	case "search":
		query := q.Get("q")
		if query == "" {
			query = q.Get("query")
		}
		if a.Win.ShowSearch != nil {
			a.Win.ShowSearch(query)
		}

	case "quicksearch":
		if a.Win.ShowQuickSearch != nil {
			a.Win.ShowQuickSearch()
		}

	case "add", "save", "new":
		rawTarget := strings.TrimSpace(q.Get("url"))
		cleanURL := ""
		if rawTarget != "" {
			var ok bool
			cleanURL, ok = validateExternalHTTPURL(rawTarget)
			if !ok {
				ensureVisibleForError(a)
				a.SayErr(locales.EditorErrURLInvalid(), "", nil)
				return
			}
		}
		title := strings.TrimSpace(q.Get("title"))
		groupName := strings.TrimSpace(q.Get("group"))
		subName := strings.TrimSpace(q.Get("sub"))

		gid, sid := a.Group, a.Sub
		if groupName != "" {
			lib := a.Store.Lib()
			for _, g := range lib.Groups {
				if strings.EqualFold(strings.TrimSpace(g.Name), groupName) {
					gid = g.ID
					if len(g.Subs) > 0 {
						sid = g.Subs[0].ID
					}
					if subName != "" {
						for _, s := range g.Subs {
							if strings.EqualFold(strings.TrimSpace(s.Name), subName) {
								sid = s.ID
								break
							}
						}
					}
					break
				}
			}
		}
		if a.Win.ShowEditor != nil {
			a.Win.ShowEditor(app.EditorRequest{
				URL:          cleanURL,
				Title:        title,
				GroupID:      gid,
				SubID:        sid,
				FromExternal: true,
			})
		}

	case "settings", "preferences":
		page := strings.ToLower(strings.TrimSpace(q.Get("page")))
		if page == "" {
			page = strings.ToLower(strings.Trim(u.Path, "/"))
		}
		if a.Win.ShowSettings != nil {
			a.Win.ShowSettings(page)
		}

	case "toggle":
		target := strings.ToLower(strings.TrimSpace(q.Get("mode")))
		if target == "" {
			target = strings.ToLower(strings.Trim(u.Path, "/"))
		}
		switch target {
		case "notch":
			if a.Win.ToggleNotch != nil {
				a.Win.ToggleNotch()
			}
		case "edge":
			if a.Win.ToggleEdge != nil {
				a.Win.ToggleEdge()
			}
		case "tray":
			if a.Win.ToggleTray != nil {
				a.Win.ToggleTray()
			}
		case "quicksearch", "search":
			if a.Win.ShowQuickSearch != nil {
				a.Win.ShowQuickSearch()
			}
		default:
			if a.Win.ShowMain != nil {
				a.Win.ShowMain()
			}
		}

	default:
		if a.Win.ShowMain != nil {
			a.Win.ShowMain()
		}
	}
}
