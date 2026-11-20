// Package app is the state every window shares and the actions that
// change it. All of it runs on the main thread: views call the actions
// directly; goroutines go through App.Post.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"cooool-mark/internal/model"
	"cooool-mark/internal/store"
)

// BuildVersion is injected at release build time via -ldflags.
var BuildVersion = "1.0.0"

// Toast is the one transient message at the bottom of a window, with an
// optional action (撤销, 重试…).
type Toast struct {
	Text   string
	Action string
	Do     func()
	Err    bool
	Until  time.Time
	seq    int
}

// EditorRequest opens the editor: ID edits a bookmark; otherwise a new
// one, prefilled with URL/Title and placed in GroupID/SubID.
type EditorRequest struct {
	ID             string
	URL, Title     string
	GroupID, SubID string
}

// Windows are the windows the views can open, set up by main.
type Windows struct {
	ShowMain        func()
	ShowEditor      func(EditorRequest)
	ShowSearch      func(query string)
	ShowQuickSearch func()
	ShowSettings    func(page string) // "general", "ai", "data"
	ShowImport   func(path string) // "" asks for a file
	// ApplyModes creates/destroys the notch, tray and edge windows
	// to match Settings.OpenModes.
	ApplyModes func()
	// HideSummoned hides whichever summoned panel (notch/tray/edge)
	// is open, after a bookmark opened from it.
	HideSummoned func()
}

// App is the shared state.
type App struct {
	Store    *store.Store
	Settings model.Settings
	Win      Windows

	Fetch  Fetcher
	Search Searcher
	AI     AI
	Import Importer

	// Where the main window is.
	Group, Sub        string
	View              string // list | grid
	StealthBackground bool

	Toast Toast

	settingsPath  string
	savedSettings atomic.Pointer[model.Settings]
	placeTimer    *time.Timer
	placeDirty    bool
	windows       []*mygo.Window
	regularWins   []*mygo.Window
	postMu        sync.Mutex
	postQueue     []func()
	toastSeq      int
	// PolishedAt marks bookmarks the AI just filled, for a 1.5 s highlight.
	PolishedAt map[string]time.Time
}

// PeekSettings reads settings.json before mygo.App.Run() starts.
func PeekSettings() model.Settings {
	s := model.DefaultSettings()
	userData, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return s
	}
	if data, err := os.ReadFile(filepath.Join(userData, "settings.json")); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

// New loads the settings and opens the library where they say.
func New() (*App, error) {
	userData, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return nil, err
	}
	a := &App{settingsPath: filepath.Join(userData, "settings.json"), PolishedAt: map[string]time.Time{}}
	a.Settings = model.DefaultSettings()
	if data, err := os.ReadFile(a.settingsPath); err == nil {
		_ = json.Unmarshal(data, &a.Settings)
	}
	defaults := model.DefaultSettings()
	for field, value := range map[*string]string{
		&a.Settings.Appearance: defaults.Appearance, &a.Settings.DefaultView: defaults.DefaultView,
		&a.Settings.Storage: defaults.Storage, &a.Settings.EdgeSide: defaults.EdgeSide,
		&a.Settings.AIEngine: defaults.AIEngine, &a.Settings.LocalModel: defaults.LocalModel,
	} {
		if *field == "" {
			*field = value
		}
	}
	initialSettings := a.Settings
	a.savedSettings.Store(&initialSettings)
	dir, err := a.LibraryDir(a.Settings.Storage, a.Settings.StorageFolder)
	if err != nil {
		return nil, err
	}
	a.Store, err = store.Open(dir, deviceID(userData), a.Post)
	if err != nil {
		return nil, err
	}
	a.Store.OnChange(a.Invalidate)
	a.Store.OnSaveError(func(err error) {
		a.SayErr("无法保存书签："+err.Error(), "重试", func() { _ = a.Store.Flush() })
	})
	a.View = a.Settings.DefaultView
	a.restorePlace()
	return a, nil
}

// LibraryDir resolves a storage setting to a folder.
func (a *App) LibraryDir(storage, folder string) (string, error) {
	switch storage {
	case model.StorageICloud:
		dir := ICloudDir()
		if dir == "" {
			return "", fmt.Errorf("没有找到 iCloud Drive")
		}
		return filepath.Join(dir, "Cooool Mark"), nil
	case model.StorageFolder:
		if folder == "" {
			return "", fmt.Errorf("没有选择文件夹")
		}
		return folder, nil
	}
	userData, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(userData, "Library"), nil
}

// ICloudDir is iCloud Drive's folder on this machine, "" if none
// (macOS: ~/Library/Mobile Documents/com~apple~CloudDocs; Windows: the
// iCloud for Windows folder).
func ICloudDir() string {
	home, _ := os.UserHomeDir()
	var dirs []string
	switch runtime.GOOS {
	case "darwin":
		dirs = []string{filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")}
	case "windows":
		dirs = []string{filepath.Join(home, "iCloudDrive"), filepath.Join(home, "iCloud Drive")}
	}
	for _, d := range dirs {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}

func deviceID(userData string) string {
	path := filepath.Join(userData, "device-id")
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return strings.TrimSpace(string(b))
	}
	id := model.NewID()
	_ = os.WriteFile(path, []byte(id), 0o644)
	return id
}

// AddWindow registers a window for Invalidate.
func (a *App) AddWindow(w *mygo.Window) {
	a.windows = append(a.windows, w)
	w.OnClosed(func() {
		for i, x := range a.windows {
			if x == w {
				a.windows = append(a.windows[:i], a.windows[i+1:]...)
				break
			}
		}
	})
}

// AddRegularWindow registers a regular user window (Main, Settings, Editor, Import)
// so background / Dock policy tracks whether any primary window is open.
func (a *App) AddRegularWindow(w *mygo.Window) {
	a.AddWindow(w)
	a.regularWins = append(a.regularWins, w)
	w.OnClosed(func() {
		for i, x := range a.regularWins {
			if x == w {
				a.regularWins = append(a.regularWins[:i], a.regularWins[i+1:]...)
				break
			}
		}
		a.SyncDockPolicy()
	})
}

// HasRegularWindow reports whether any regular window is currently open.
func (a *App) HasRegularWindow() bool {
	for _, w := range a.regularWins {
		if w != nil && !w.IsDestroyed() {
			return true
		}
	}
	return false
}

// PrepareRegularWindow ensures the macOS Dock icon is active before showing a regular window.
func (a *App) PrepareRegularWindow() {
	a.StealthBackground = false
	if runtime.GOOS == "darwin" && mygo.App.ActivationPolicy() != mygo.ActivationPolicyRegular {
		mygo.App.Dock.Show()
	}
}

// SyncDockPolicy shows or hides the macOS Dock icon based on open regular windows,
// active summon modes, and the user's background settings.
func (a *App) SyncDockPolicy() {
	if runtime.GOOS != "darwin" {
		return
	}
	if a.HasRegularWindow() {
		a.PrepareRegularWindow()
		return
	}
	if (a.Settings.HideDockWhenClosed && a.Settings.HasSummonMode()) || a.StealthBackground {
		if mygo.App.ActivationPolicy() != mygo.ActivationPolicyAccessory {
			mygo.App.Dock.Hide()
		}
		return
	}
	if mygo.App.ActivationPolicy() != mygo.ActivationPolicyRegular {
		mygo.App.Dock.Show()
	}
}

// Invalidate redraws every window.
func (a *App) Invalidate() {
	for _, w := range a.windows {
		w.Invalidate()
	}
}

// Post queues fn in submission order on the main thread (any goroutine).
func (a *App) Post(fn func()) {
	a.postMu.Lock()
	start := len(a.postQueue) == 0
	a.postQueue = append(a.postQueue, fn)
	a.postMu.Unlock()
	if !start {
		return
	}
	go mygo.RunOnMain(func() {
		a.postMu.Lock()
		batch := a.postQueue
		a.postQueue = nil
		a.postMu.Unlock()
		for _, fn := range batch {
			fn()
		}
		a.Invalidate()
	})
}

// SaveSettings writes settings.json and applies what changed.
func (a *App) SaveSettings() {
	previous := a.savedSettings.Load()
	if err := a.writeSettings(a.Settings); err != nil {
		if previous != nil {
			if a.Settings.DefaultView != previous.DefaultView {
				a.View = previous.DefaultView
			}
			a.Settings = *previous
			if a.AI != nil {
				a.AI.Configure(*previous)
			}
		}
		a.SayErr("无法保存设置："+err.Error(), "", nil)
		return
	}
	a.clearPlaceTimer()
	if previous != nil && previous.DefaultView != a.Settings.DefaultView {
		a.View = a.Settings.DefaultView
	}
	if a.AI != nil {
		a.AI.Configure(a.Settings)
	}
	if a.Win.ApplyModes != nil {
		a.Win.ApplyModes()
	}
	a.SyncDockPolicy()
	if a.Settings.AutoBackup && (previous == nil || !previous.AutoBackup) {
		a.DailyBackup()
	}
	a.Invalidate()
}

// UseWebView reads the last saved setting safely from a background fetch.
func (a *App) UseWebView() bool {
	s := a.savedSettings.Load()
	return s != nil && s.UseWebView
}

func (a *App) writeSettings(s model.Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := store.WriteAtomic(a.settingsPath, data); err != nil {
		return err
	}
	a.savedSettings.Store(&s)
	return nil
}

func (a *App) DailyBackup() {
	if a.Settings.AutoBackup {
		if err := a.Store.DailyBackup(); err != nil {
			a.SayErr("自动备份失败："+err.Error(), "重试", a.DailyBackup)
		}
	}
}

// SetStorage moves the library to another location (merging with a
// library already there) and saves the setting.
func (a *App) SetStorage(storage, folder string) error {
	dir, err := a.LibraryDir(storage, folder)
	if err != nil {
		return err
	}
	next := a.Settings
	next.Storage, next.StorageFolder = storage, folder
	commit := func() error { return a.writeSettings(next) }
	if filepath.Clean(dir) == filepath.Clean(a.Store.Dir()) {
		err = commit()
	} else {
		err = a.Store.MoveTo(dir, commit)
	}
	if err != nil {
		return err
	}
	a.Settings = next
	a.DailyBackup()
	a.Invalidate()
	return nil
}

// ---- place ----

func (a *App) restorePlace() {
	a.Group, a.Sub = a.Settings.LastGroup, a.Settings.LastSub
	a.EnsurePlace()
}

// EnsurePlace points Group/Sub at something that exists.
func (a *App) EnsurePlace() {
	lib := a.Store.Lib()
	g := lib.Group(a.Group)
	if g == nil && len(lib.Groups) > 0 {
		g = &lib.Groups[0]
	}
	if g == nil {
		a.Group, a.Sub = "", ""
		return
	}
	a.Group = g.ID
	if lib.Sub(g.ID, a.Sub) == nil {
		a.Sub = ""
		if len(g.Subs) > 0 {
			a.Sub = g.Subs[0].ID
		}
	}
}

// Go shows a group (and a sub-group, "" for its first).
func (a *App) Go(groupID, subID string) {
	a.Group, a.Sub = groupID, subID
	a.EnsurePlace()
	a.Settings.LastGroup, a.Settings.LastSub = a.Group, a.Sub
	a.clearPlaceTimer()
	if err := a.patchSavedSettings(func(s *model.Settings) {
		s.LastGroup, s.LastSub = a.Group, a.Sub
	}); err != nil {
		a.SayErr("无法记住当前分组："+err.Error(), "", nil)
	}
	a.Invalidate()
}

// SetSub updates the active sub-group within the current group and schedules
// persisting it to settings.json (debounced so scrolling stays smooth).
func (a *App) SetSub(subID string) {
	if subID == "" || subID == a.Sub {
		return
	}
	if a.Store.Lib().Sub(a.Group, subID) == nil {
		return
	}
	a.Sub = subID
	a.Settings.LastGroup, a.Settings.LastSub = a.Group, a.Sub
	a.schedulePlaceSave()
}

// SetView switches between "list" and "grid" and persists the choice.
func (a *App) SetView(view string) {
	if view != "grid" {
		view = "list"
	}
	if a.View == view && a.Settings.DefaultView == view {
		return
	}
	prevView, prevDefault := a.View, a.Settings.DefaultView
	a.View = view
	a.Settings.DefaultView = view
	if err := a.patchSavedSettings(func(s *model.Settings) {
		s.DefaultView = view
	}); err != nil {
		a.View = prevView
		a.Settings.DefaultView = prevDefault
		a.SayErr("无法保存视图设置："+err.Error(), "", nil)
		return
	}
	a.Invalidate()
}

// ToggleView switches between list and grid view and persists the choice.
func (a *App) ToggleView() {
	if a.View == "grid" {
		a.SetView("list")
	} else {
		a.SetView("grid")
	}
}

// SetModeGroup remembers the last selected group for a summon mode.
func (a *App) SetModeGroup(mode, groupID string) {
	var target *string
	switch mode {
	case model.ModeNotch:
		target = &a.Settings.NotchGroup
	case model.ModeTray:
		target = &a.Settings.TrayGroup
	case model.ModeEdge:
		target = &a.Settings.EdgeGroup
	default:
		return
	}
	if *target == groupID {
		return
	}
	*target = groupID
	_ = a.patchSavedSettings(func(s *model.Settings) {
		switch mode {
		case model.ModeNotch:
			s.NotchGroup = groupID
		case model.ModeTray:
			s.TrayGroup = groupID
		case model.ModeEdge:
			s.EdgeGroup = groupID
		}
	})
}

// FlushSettings writes any pending debounced settings (such as last place) now.
func (a *App) FlushSettings() {
	a.EnsurePlace()
	dirty := a.placeDirty || a.Settings.LastGroup != a.Group || a.Settings.LastSub != a.Sub
	a.clearPlaceTimer()
	if !dirty {
		return
	}
	a.Settings.LastGroup, a.Settings.LastSub = a.Group, a.Sub
	_ = a.patchSavedSettings(func(s *model.Settings) {
		s.LastGroup, s.LastSub = a.Group, a.Sub
	})
}

func (a *App) clearPlaceTimer() {
	if a.placeTimer != nil {
		a.placeTimer.Stop()
		a.placeTimer = nil
	}
	a.placeDirty = false
}

func (a *App) schedulePlaceSave() {
	a.placeDirty = true
	if a.placeTimer != nil {
		a.placeTimer.Stop()
	}
	a.placeTimer = time.AfterFunc(400*time.Millisecond, func() {
		a.Post(func() {
			a.placeTimer = nil
			if !a.placeDirty {
				return
			}
			a.placeDirty = false
			a.Settings.LastGroup, a.Settings.LastSub = a.Group, a.Sub
			_ = a.patchSavedSettings(func(s *model.Settings) {
				s.LastGroup, s.LastSub = a.Group, a.Sub
			})
		})
	})
}

func (a *App) patchSavedSettings(fn func(s *model.Settings)) error {
	previous := a.savedSettings.Load()
	if previous == nil {
		return nil
	}
	next := *previous
	fn(&next)
	return a.writeSettings(next)
}

// Choices lists every group / sub-group, for pickers and the AI.
func (a *App) Choices() []GroupChoice {
	var out []GroupChoice
	for _, g := range a.Store.Lib().Groups {
		for _, s := range g.Subs {
			out = append(out, GroupChoice{GroupID: g.ID, SubID: s.ID, Path: g.Name + " / " + s.Name})
		}
	}
	return out
}

// PathOf is "group / sub" of a bookmark.
func (a *App) PathOf(b *model.Bookmark) string {
	lib := a.Store.Lib()
	g, s := lib.Group(b.GroupID), lib.Sub(b.GroupID, b.SubID)
	if g == nil || s == nil {
		return "未分组"
	}
	return g.Name + " / " + s.Name
}

// ---- toast ----

// Say shows a toast for 1.8 s, or 5 s with an action.
func (a *App) Say(text string) { a.toast(Toast{Text: text}, 1800*time.Millisecond) }

// SayErr shows an error toast with an optional retry.
func (a *App) SayErr(text, action string, do func()) {
	a.toast(Toast{Text: text, Action: action, Do: do, Err: true}, 5*time.Second)
}

// SayUndo shows a toast with 撤销.
func (a *App) SayUndo(text string, undo func()) {
	a.toast(Toast{Text: text, Action: "撤销", Do: undo}, 5*time.Second)
}

func (a *App) toast(t Toast, d time.Duration) {
	a.toastSeq++
	t.seq = a.toastSeq
	t.Until = time.Now().Add(d)
	a.Toast = t
	seq := t.seq
	time.AfterFunc(d, func() {
		a.Post(func() {
			if a.Toast.seq == seq {
				a.Toast = Toast{}
			}
		})
	})
	a.Invalidate()
}

// ToastAction runs the toast's action and dismisses it.
func (a *App) ToastAction() {
	do := a.Toast.Do
	a.Toast = Toast{}
	if do != nil {
		do()
	}
	a.Invalidate()
}

// ---- opening ----

// Open opens a bookmark in the default browser. A template needs
// its query (call with the query; "" opens the URL with {query} removed).
func (a *App) Open(b *model.Bookmark, query string) bool {
	url := b.URL
	if b.IsTemplate() {
		url = strings.ReplaceAll(url, "{query}", urlQueryEscape(query))
	}
	if err := a.OpenURL(url); err != nil {
		a.SayErr("打不开「"+b.Title+"」", "重试", func() { a.Open(b, query) })
		return false
	}
	if a.Win.HideSummoned != nil {
		a.Win.HideSummoned()
	}
	return true
}

// OpenURL opens a URL in the system default browser.
func (a *App) OpenURL(url string) error {
	return mygo.Shell.OpenExternal(url)
}

func urlQueryEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == ' ':
			b.WriteString("%20")
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// Background runs work off the main thread with a timeout.
func Background(d time.Duration, fn func(ctx context.Context)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		fn(ctx)
	}()
}
