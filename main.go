package main

import (
	"context"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"cooool-mark/internal/ai"
	"cooool-mark/internal/app"
	"cooool-mark/internal/fetch"
	"cooool-mark/internal/importer"
	"cooool-mark/internal/mode"
	"cooool-mark/internal/search"
	"cooool-mark/internal/theme"
	"cooool-mark/internal/view/editor"
	"cooool-mark/internal/view/mainwin"
	"cooool-mark/internal/view/manage"
	"cooool-mark/internal/view/settings"
)

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
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	app.SetDefaultIcon(theme.DefaultIcon)

	bgFlag := hasBackgroundFlag(os.Args[1:])
	peeked := app.PeekSettings()
	if runtime.GOOS == "darwin" && (bgFlag || (peeked.LaunchInBackground && peeked.HideDockWhenClosed && peeked.HasSummonMode())) {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}

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
		a.AI.OnChange(func() { a.Post(func() {}) })
		a.AI.Configure(a.Settings)

		a.Win = app.Windows{
			ShowMain:        func() { mainwin.Show(a) },
			ShowSearch:      func(q string) { mainwin.ShowSearch(a, q) },
			ShowQuickSearch: func() { mode.ToggleQuickSearch(a) },
			ShowEditor:      func(r app.EditorRequest) { editor.Show(a, r) },
			ShowSettings:    func(p string) { settings.Show(a, p) },
			ShowImport:      func(p string) { manage.ShowImport(a, p) },
			ApplyModes:      func() { mode.Apply(a) },
			HideSummoned:    mode.HideSummoned,
		}

		mygo.App.SetMenu(appMenu(a))

		ctx, cancel := context.WithCancel(context.Background())
		go a.Store.Watch(ctx, func(added int) {
			if added > 0 {
				a.Say("已从同步合并 " + strconv.Itoa(added) + " 条")
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

		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			settings.Flush()
			a.FlushSettings()
			if err := a.Store.Flush(); err != nil && !e.SessionEnding {
				e.PreventDefault()
				a.Win.ShowMain()
				return
			}
			mode.Close()
		})
		mygo.App.OnQuitCanceled(func() { mode.Apply(a) })
		mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
			cancel()
			settings.Flush()
			a.FlushSettings()
			_ = a.Store.Flush()
		})

		mygo.App.OnWindowAllClosed(func() {
			if runtime.GOOS != "darwin" && !a.Settings.HasSummonMode() {
				mygo.App.Quit()
			}
		})
		mygo.App.OnActivate(func(bool) {
			if !a.HasRegularWindow() && a.Win.ShowMain != nil {
				a.Win.ShowMain()
			}
		})
		mygo.App.OnSecondInstance(func(args []string, _ string) {
			if !hasBackgroundFlag(args) && a.Win.ShowMain != nil {
				a.Win.ShowMain()
			}
		})

		mode.Apply(a)
		startSilent := bgFlag || mygo.App.WasOpenedAtLogin() || a.Settings.LaunchInBackground
		if !startSilent {
			a.Win.ShowMain()
		} else {
			a.SyncDockPolicy()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func appMenu(a *app.App) *mygo.Menu {
	click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { fn() }
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "新建书签", Accelerator: "CmdOrCtrl+N", Click: click(func() {
				a.Win.ShowEditor(app.EditorRequest{GroupID: a.Group, SubID: a.Sub})
			})},
			{Label: "查找", Accelerator: "CmdOrCtrl+F", Click: click(func() { a.Win.ShowSearch("") })},
			mygo.Separator(),
			{Label: "导入…", Click: click(func() { a.Win.ShowImport("") })},
			mygo.Separator(),
			{Role: mygo.RoleClose},
			mygo.Separator(),
			{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: click(func() { a.Win.ShowSettings("general") })},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	})
}
