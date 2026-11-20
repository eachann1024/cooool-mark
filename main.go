package main

import (
	"context"
	"log"
	"strconv"
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

func main() {
	mygo.App.SetName("Cooool Mark")
	app.SetDefaultIcon(theme.DefaultIcon)

	mygo.App.WhenReady(func() {
		a, err := app.New()
		if err != nil {
			log.Fatal(err)
		}
		a.Search = search.New()
		a.Fetch = fetch.New(a)
		a.AI = ai.New(a)
		a.Import = importer.New()
		a.AI.OnChange(func() { a.Post(func() {}) })
		a.AI.Configure(a.Settings)

		a.Win = app.Windows{
			ShowMain:     func() { mainwin.Show(a) },
			ShowSearch:   func(q string) { mainwin.ShowSearch(a, q) },
			ShowEditor:   func(r app.EditorRequest) { editor.Show(a, r) },
			ShowSettings: func(p string) { settings.Show(a, p) },
			ShowImport:   func(p string) { manage.ShowImport(a, p) },
			ApplyModes:   func() { mode.Apply(a) },
			HideSummoned: mode.HideSummoned,
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
			_ = a.Store.Flush()
		})

		mode.Apply(a)
		a.Win.ShowMain()
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
			{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: click(func() { a.Win.ShowSettings("general") })},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	})
}
