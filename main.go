package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"cooool-mark/internal/ai"
	"cooool-mark/internal/app"
	"cooool-mark/internal/fetch"
	"cooool-mark/internal/importer"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/mode"
	"cooool-mark/internal/search"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/update"
	"cooool-mark/internal/view/editor"
	"cooool-mark/internal/view/mainwin"
	"cooool-mark/internal/view/manage"
	"cooool-mark/internal/view/settings"
)

const githubRepoURL = update.GitHubRepoURL

func hasBackgroundFlag(args []string) bool {
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "--background", "-background", "--hidden", "-hidden", "--silent", "-silent":
			return true
		}
	}
	return false
}

func main() {
	mygo.App.SetName("Cooool Mark")
	if strings.TrimSpace(mygo.App.Version()) == "" {
		mygo.App.SetVersion(app.BuildVersion)
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	app.SetDefaultIcon(theme.DefaultIcon)

	bgFlag := hasBackgroundFlag(os.Args[1:])
	peeked := app.PeekSettings()
	mygo.Theme.SetSource(app.AppearanceThemeSource(peeked.Appearance))
	if runtime.GOOS == "darwin" && peeked.HasSummonMode() && (bgFlag || (peeked.LaunchInBackground && peeked.HideDockWhenClosed)) {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}

	// Register OS handoff hooks before Run so cold-start URLs, files, and
	// notification clicks are never lost.
	mygo.App.OnOpenURL(queueOrHandleURL)
	mygo.App.OnOpenFile(queueOrHandleFile)
	mygo.App.OnNotificationClick(queueOrHandleNotificationClick)

	mygo.App.WhenReady(func() {
		a, err := app.New()
		if err != nil {
			log.Fatal(err)
		}
		a.StealthBackground = bgFlag
		a.Search = search.New()
		a.Fetch = fetch.New(a)
		a.AI = ai.New(a)
		a.Import = importer.New()
		a.AI.OnChange(func() {
			a.Post(func() {
				var prog float64
				for _, id := range []string{"qwen3.5-0.8b", "qwen3.5-2b", "qwen3.5-4b"} {
					if st := a.AI.LocalModelStatus(id); st.Downloading && st.Progress > 0 && st.Progress < 1 {
						prog = st.Progress
						break
					}
				}
				a.SetDownloadProgress(prog)
			})
		})
		a.AI.OnDownloadComplete(func(modelName string) {
			a.Post(func() {
				a.SetDownloadProgress(0)
				a.Say(locales.ToastDownloadDone(modelName))
				if !mygo.App.IsActive() {
					a.Notify("model-"+modelName, locales.NotifyModelReadyTitle(), locales.NotifyModelReadyBody(modelName), func() {
						if a.Win.ShowSettings != nil {
							a.Win.ShowSettings("ai")
						}
					})
				}
			})
		})
		a.AI.Configure(a.Settings)

		refreshChrome := func() {
			mygo.App.SetMenu(appMenu(a))
			mygo.App.Dock.SetMenu(dockMenu(a))
			syncJumpList()
			mainwin.RefreshChrome()
			editor.RefreshChrome()
			manage.RefreshChrome()
			settings.RefreshChrome()
			mode.RefreshChrome(a)
			a.Invalidate()
		}

		a.Win = app.Windows{
			ShowMain:           func() { mainwin.Show(a) },
			FocusSearch:        func() { mainwin.FocusSearch(a) },
			ShowSearch:         func(q string) { mainwin.ShowSearch(a, q) },
			ShowQuickSearch:    func() { mode.ToggleQuickSearch(a) },
			ShowEditor:         func(r app.EditorRequest) { editor.Show(a, r) },
			ShowSettings:       func(p string) { settings.Show(a, p) },
			ShowImport:         func(p string) { manage.ShowImport(a, p) },
			ShowAbout:          func() { showAboutPanel() },
			Export:             func(format string) { settings.Export(a, format) },
			ApplyModes:         func() { mode.Apply(a) },
			HasActiveSummon:    mode.HasActiveSummonMode,
			ApplyShortcuts:        func() { applyGlobalShortcuts(a) },
			SetShortcut:           func(slot, accel string) bool { return trySetShortcut(a, slot, accel) },
			CancelPendingShortcut: func(slot string) { cancelPendingShortcut(a, slot) },
			SetShortcutsPaused:    func(paused bool) { setGlobalShortcutsPaused(a, paused) },
			RefreshChrome:      refreshChrome,
			ToggleNotch:        func() { mode.ToggleNotch(a) },
			ToggleEdge:         func() { mode.ToggleEdge(a) },
			ToggleTray:         func() { mode.ToggleTray(a) },
			HideSummoned:       mode.HideSummoned,
		}

		ensureURLSchemeRegistered()
		a.SyncAppearance()
		a.SyncKeepRunning()
		refreshChrome()
		applyGlobalShortcuts(a)

		mygo.App.OnLocaleChanged(func(string) {
			refreshChrome()
		})
		mygo.Theme.OnUpdated(func() {
			a.Invalidate()
		})
		mygo.App.OnDidBecomeActive(func() {
			mygo.ClearNotifications()
		})

		ctx, cancel := context.WithCancel(context.Background())
		go a.Store.Watch(ctx, func(added int) {
			if added > 0 {
				a.Say(locales.ToastSynced(added))
				if !mygo.App.IsActive() {
					a.Notify("sync-merged", locales.NotifySyncTitle(), locales.NotifySyncBody(added), func() {
						if a.Win.ShowMain != nil {
							a.Win.ShowMain()
						}
					})
				}
			}
		})
		a.DailyBackup()
		go func() {
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					a.Post(func() {
						if ctx.Err() == nil {
							a.DailyBackup()
						}
					})
				}
			}
		}()
		time.AfterFunc(8*time.Second, func() {
			a.Post(func() {
				if ctx.Err() == nil {
					a.CheckUpdate(false)
				}
			})
		})

		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			if e.SessionEnding {
				editor.AllowClose()
				settings.Flush()
			} else {
				if editor.PromptUnsavedOnQuit() {
					e.PreventDefault()
					return
				}
				if err := settings.FlushBeforeQuit(); err != nil {
					e.PreventDefault()
					if a.Win.ShowSettings != nil {
						a.Win.ShowSettings("ai")
					}
					a.SayErr(locales.AiKeySaveErr(), "", nil)
					return
				}
			}
			a.FlushSettings()
			if err := a.Store.Flush(); err != nil && !e.SessionEnding {
				e.PreventDefault()
				a.Win.ShowMain()
				return
			}
			clearGlobalShortcuts()
			mode.Close()
		})
		mygo.App.OnQuitCanceled(func() {
			mode.Apply(a)
			applyGlobalShortcuts(a)
		})
		mygo.App.OnWillQuit(func(e *mygo.QuitEvent) {
			cancel()
			clearGlobalShortcuts()
			settings.FlushWait(e != nil && e.SessionEnding)
			a.FlushSettings()
			_ = a.Store.Flush()
			a.Close()
		})

		mygo.App.OnWindowAllClosed(func() {
			if runtime.GOOS != "darwin" && !a.HasActiveSummon() {
				mygo.App.Quit()
			}
		})
		mygo.App.OnActivate(func(bool) {
			if !a.HasRegularWindow() && a.Win.ShowMain != nil {
				a.Win.ShowMain()
			}
		})
		mygo.App.OnSecondInstance(func(args []string, wd string) {
			if hasBackgroundFlag(args) {
				return
			}
			summary := inspectLaunchArgs(args, wd)
			if !summary.hasRealTarget && a.Win.ShowMain != nil {
				a.Win.ShowMain()
			}
			if len(summary.missingFiles) > 0 {
				a.SayErr(locales.ToastFileMissing(filepath.Base(summary.missingFiles[0])), "", nil)
			}
		})

		mode.Apply(a)
		wd, _ := os.Getwd()
		argSummary := inspectLaunchArgs(os.Args[1:], wd)
		handledPending := flushPendingLaunchEvents(a)
		wantSilent := bgFlag || mygo.App.WasOpenedAtLogin() || a.Settings.LaunchInBackground
		canStaySilent := a.HasActiveSummon() || runtime.GOOS == "darwin"

		if handledPending || argSummary.hasRealTarget {
			a.SyncDockPolicy()
		} else if wantSilent && canStaySilent {
			a.SyncDockPolicy()
		} else {
			a.Win.ShowMain()
		}
		if len(argSummary.missingFiles) > 0 {
			ensureVisibleForError(a)
			a.SayErr(locales.ToastFileMissing(filepath.Base(argSummary.missingFiles[0])), "", nil)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// ensureURLSchemeRegistered registers the coooolmark:// URL scheme only for
// portable/AppImage/dev launches where an OS installer did not already install
// a system-level protocol handler, avoiding orphaned HKCU or XDG shadow files.
func ensureURLSchemeRegistered() {
	switch runtime.GOOS {
	case "windows":
		if hasInstalledWindowsScheme() {
			cleanupStaleWindowsHKCUScheme()
			return
		}
		if exe, err := os.Executable(); err == nil {
			if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "uninstall.exe")); err == nil {
				return
			}
		}
		if !mygo.App.IsURLSchemeRegistered("coooolmark") {
			_ = mygo.App.RegisterURLScheme("coooolmark")
		}
	case "linux":
		if os.Getenv("APPIMAGE") == "" {
			for _, sysDesktop := range []string{
				"/usr/share/applications/cooool-mark.desktop",
				"/usr/share/applications/app.cooool.mark.desktop",
			} {
				if _, err := os.Stat(sysDesktop); err == nil {
					return
				}
			}
		}
		if !mygo.App.IsURLSchemeRegistered("coooolmark") {
			_ = mygo.App.RegisterURLScheme("coooolmark")
		}
	}
}

func showAboutPanel() {
	mygo.App.ShowAboutPanel(mygo.AboutPanelOptions{
		ApplicationName:    "Cooool Mark",
		ApplicationVersion: strings.TrimPrefix(app.Version(), "v"),
		Credits:            locales.AboutTagline() + "\n" + locales.AboutCredits(),
	})
}

func dockMenu(a *app.App) *mygo.Menu {
	click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { fn() }
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.MenuOpenMain(), Click: click(func() { a.Win.ShowMain() })},
		{Label: locales.MenuNewBookmark(), Click: click(func() {
			a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
		})},
		{Label: locales.MenuQuickAddClipboard(), Click: click(func() {
			a.QuickAddFromClipboard()
		})},
		{Label: locales.MenuQuickSearch(), Click: click(func() { a.Win.ShowQuickSearch() })},
		mygo.Separator(),
		{Label: locales.MenuSettings(), Click: click(func() { a.Win.ShowSettings("general") })},
	})
}

func syncJumpList() {
	if runtime.GOOS != "windows" {
		return
	}
	_ = mygo.App.JumpList.SetTasks([]mygo.JumpListTask{
		{Title: locales.MenuNewBookmark(), Args: []string{"coooolmark://add"}},
		{Title: locales.MenuSearchBookmarks(), Args: []string{"coooolmark://search"}},
		{Title: locales.MenuSettings(), Args: []string{"coooolmark://settings"}},
	})
}

func appMenu(a *app.App) *mygo.Menu {
	click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { fn() }
	}

	var top []*mygo.MenuItem
	if runtime.GOOS == "darwin" {
		top = append(top, &mygo.MenuItem{
			Role: mygo.RoleAppMenu,
			Submenu: []*mygo.MenuItem{
				{Label: locales.MenuAbout(), Click: click(func() { a.Win.ShowAbout() })},
				{Label: locales.MenuCheckUpdates(), Click: click(func() { a.CheckUpdate(true) })},
				mygo.Separator(),
				{Label: locales.MenuSettings(), Accelerator: "CmdOrCtrl+,", Click: click(func() { a.Win.ShowSettings("general") })},
				mygo.Separator(),
				{Role: mygo.RoleServices},
				mygo.Separator(),
				{Role: mygo.RoleHide},
				{Role: mygo.RoleHideOthers},
				{Role: mygo.RoleUnhide},
				mygo.Separator(),
				{Role: mygo.RoleQuit},
			},
		})
	}

	fileItems := []*mygo.MenuItem{
		{Label: locales.MenuNewBookmark(), Accelerator: "CmdOrCtrl+N", Click: click(func() {
			a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
		})},
		{Label: locales.MenuQuickAddClipboard(), Click: click(func() {
			a.QuickAddFromClipboard()
		})},
		mygo.Separator(),
		{Label: locales.MenuFind(), Accelerator: "CmdOrCtrl+F", Click: click(func() {
			if a.Win.FocusSearch != nil {
				a.Win.FocusSearch()
			} else if a.Win.ShowSearch != nil {
				a.Win.ShowSearch("")
			}
		})},
		{Label: locales.MenuQuickSearch(), Click: click(func() { a.Win.ShowQuickSearch() })},
		mygo.Separator(),
		{Label: locales.MenuImport(), Click: click(func() { a.Win.ShowImport("") })},
		{Label: locales.MenuExport(), Submenu: []*mygo.MenuItem{
			{Label: locales.MenuExportHTML(), Click: click(func() { a.Win.Export("html") })},
			{Label: locales.MenuExportJSON(), Click: click(func() { a.Win.Export("json") })},
		}},
		mygo.Separator(),
		{Role: mygo.RoleClose},
	}
	if runtime.GOOS != "darwin" {
		fileItems = append(fileItems,
			mygo.Separator(),
			&mygo.MenuItem{Label: locales.MenuSettings(), Accelerator: "CmdOrCtrl+,", Click: click(func() { a.Win.ShowSettings("general") })},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleQuit},
		)
	}
	top = append(top, &mygo.MenuItem{Label: locales.MenuFile(), Submenu: fileItems})

	top = append(top, &mygo.MenuItem{Role: mygo.RoleEditMenu})

	top = append(top, &mygo.MenuItem{
		Label: locales.MenuView(),
		Submenu: []*mygo.MenuItem{
			{Label: locales.MenuToggleView(), Accelerator: "CmdOrCtrl+L", Click: click(func() { a.ToggleView() })},
			mygo.Separator(),
			{
				Label:   locales.MenuShowNotes(),
				Type:    mygo.MenuItemCheckbox,
				Checked: a.Settings.ShowNote,
				Click: click(func() {
					a.Settings.ShowNote = !a.Settings.ShowNote
					a.SaveSettings()
				}),
			},
			{
				Label:   locales.MenuShowTags(),
				Type:    mygo.MenuItemCheckbox,
				Checked: a.Settings.ShowTags,
				Click: click(func() {
					a.Settings.ShowTags = !a.Settings.ShowTags
					a.SaveSettings()
				}),
			},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
		},
	})

	if runtime.GOOS == "darwin" {
		top = append(top, &mygo.MenuItem{
			Role: mygo.RoleWindow,
			Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleMinimize},
				{Role: mygo.RoleZoom},
				mygo.Separator(),
				{Label: locales.MenuOpenMain(), Click: click(func() { a.Win.ShowMain() })},
				mygo.Separator(),
				{Role: mygo.RoleFront},
			},
		})
	} else {
		top = append(top, &mygo.MenuItem{
			Role: mygo.RoleWindowMenu,
			Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleMinimize},
				{Label: locales.MenuOpenMain(), Click: click(func() { a.Win.ShowMain() })},
				{Role: mygo.RoleClose},
			},
		})
	}

	helpItems := []*mygo.MenuItem{
		{Label: locales.MenuCheckUpdates(), Click: click(func() { a.CheckUpdate(true) })},
		{Label: locales.MenuRevealDataFolder(), Click: click(func() { _ = mygo.Shell.OpenPath(a.Store.Dir()) })},
		mygo.Separator(),
		{Label: locales.MenuGitHubRepo(), Click: click(func() { _ = a.OpenURL(githubRepoURL) })},
	}
	if runtime.GOOS == "darwin" {
		top = append(top, &mygo.MenuItem{Role: mygo.RoleHelp, Submenu: helpItems})
	} else {
		helpItems = append(helpItems,
			mygo.Separator(),
			&mygo.MenuItem{Label: locales.MenuAbout(), Click: click(func() { a.Win.ShowAbout() })},
		)
		top = append(top, &mygo.MenuItem{Label: locales.MenuHelp(), Submenu: helpItems})
	}

	return mygo.NewMenu(top)
}
