// Package importer reads browser bookmark files and our JSON backups.
package importer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
)

const maxImportBytes = 64 << 20

type importer struct{}

// New returns the Importer.
func New() app.Importer { return importer{} }

func (importer) Parse(path string, lib *model.Library) (*app.ImportPlan, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New(locales.ImportErrIsDir())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New(locales.ImportErrNotRegularFile())
	}
	if info.Size() > maxImportBytes {
		return nil, errors.New(locales.ImportErrTooLarge())
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxImportBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxImportBytes {
		return nil, errors.New(locales.ImportErrTooLarge())
	}
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	ext := strings.ToLower(filepath.Ext(path))

	var plan *app.ImportPlan
	switch {
	case ext == ".json" || (len(trimmed) > 0 && trimmed[0] == '{'):
		plan, err = parseJSONBackup(data)
	default:
		plan, err = parseNetscapeHTML(data)
	}
	if err != nil {
		return nil, err
	}
	plan.File = path
	RefreshStats(plan, lib)
	return plan, nil
}

// RefreshStats recomputes Total and Dups for plan against lib without
// re-reading the source file.
func RefreshStats(plan *app.ImportPlan, lib *model.Library) {
	if plan == nil {
		return
	}
	seen := existingURLSet(lib)
	plan.Total, plan.Dups = 0, 0
	for _, f := range plan.Folders {
		computeStats(f, seen)
		plan.Total += f.Count
		plan.Dups += f.Dups
	}
}

func (importer) Apply(lib *model.Library, plan *app.ImportPlan, opts app.ImportOptions) []string {
	if lib == nil || plan == nil {
		return nil
	}
	if opts.Mode == app.ImportRestore && plan.Backup != nil {
		cp := CloneLibrary(plan.Backup)
		*lib = *cp
		var ids []string
		for _, b := range lib.Live() {
			ids = append(ids, b.ID)
		}
		return ids
	}

	now := time.Now()
	seen := existingURLSet(lib)
	subMaxOrder := map[string]float64{}
	for _, b := range lib.Live() {
		k := b.GroupID + "\x00" + b.SubID
		if b.Order > subMaxOrder[k] {
			subMaxOrder[k] = b.Order
		}
	}

	var added []string

	findGroup := func(name string) *model.Group {
		for i := range lib.Groups {
			if lib.Groups[i].Name == name {
				return &lib.Groups[i]
			}
		}
		return nil
	}
	createGroup := func(name string) *model.Group {
		order := 1.0
		if n := len(lib.Groups); n > 0 {
			order = lib.Groups[n-1].Order + 1
		}
		lib.Groups = append(lib.Groups, model.Group{
			ID:        model.NewID(),
			Name:      name,
			Order:     order,
			UpdatedAt: now,
		})
		return &lib.Groups[len(lib.Groups)-1]
	}
	ensureSub := func(g *model.Group, name string) *model.Sub {
		for i := range g.Subs {
			if g.Subs[i].Name == name {
				return &g.Subs[i]
			}
		}
		order := 1.0
		if n := len(g.Subs); n > 0 {
			order = g.Subs[n-1].Order + 1
		}
		g.Subs = append(g.Subs, model.Sub{
			ID:        model.NewID(),
			Name:      name,
			Order:     order,
			UpdatedAt: now,
		})
		g.UpdatedAt = now
		return &g.Subs[len(g.Subs)-1]
	}

	filterItems := func(items []model.Bookmark) []model.Bookmark {
		var out []model.Bookmark
		for _, it := range items {
			u := model.EnsureURLScheme(it.URL)
			if u == "" {
				continue
			}
			norm := model.NormalizeURL(u)
			if opts.SkipDups && norm != "" && seen[norm] {
				continue
			}
			if norm != "" {
				seen[norm] = true
			}
			it.URL = u
			out = append(out, it)
		}
		return out
	}

	appendItems := func(ensureGroup func() *model.Group, subName string, items []model.Bookmark) {
		filtered := filterItems(items)
		if len(filtered) == 0 {
			return
		}
		g := ensureGroup()
		sub := ensureSub(g, subName)
		key := g.ID + "\x00" + sub.ID
		order := subMaxOrder[key]
		for _, it := range filtered {
			order++
			title := strings.TrimSpace(it.Title)
			if title == "" {
				title = it.URL
			}
			created := it.CreatedAt
			if created.IsZero() {
				created = now
			}
			id := model.NewID()
			lib.Bookmarks = append(lib.Bookmarks, model.Bookmark{
				ID:        id,
				Title:     title,
				URL:       it.URL,
				Icon:      defaultIcon(title, it.URL),
				GroupID:   g.ID,
				SubID:     sub.ID,
				Order:     order,
				CreatedAt: created,
				UpdatedAt: now,
			})
			added = append(added, id)
		}
		subMaxOrder[key] = order
	}

	// ImportMerge (default)
	for _, top := range plan.Folders {
		groupName := strings.TrimSpace(top.Name)
		if groupName == "" {
			groupName = locales.Ungrouped()
		}
		var target *model.Group
		ensureTarget := func() *model.Group {
			if target != nil {
				return target
			}
			if g := findGroup(groupName); g != nil {
				target = g
			} else {
				target = createGroup(groupName)
			}
			return target
		}

		if top.Checked && len(top.Items) > 0 {
			appendItems(ensureTarget, locales.Ungrouped(), top.Items)
		}

		var walkSub func(f *app.ImportFolder, prefix string)
		walkSub = func(f *app.ImportFolder, prefix string) {
			name := strings.TrimSpace(f.Name)
			if name == "" {
				name = locales.Untitled()
			}
			subName := name
			if prefix != "" {
				subName = prefix + " / " + name
			}
			if f.Checked && len(f.Items) > 0 {
				appendItems(ensureTarget, subName, f.Items)
			}
			for _, ch := range f.Children {
				walkSub(ch, subName)
			}
		}
		for _, sub := range top.Children {
			walkSub(sub, "")
		}
	}
	return added
}

func (importer) Export(lib *model.Library, path, format string) error {
	if lib == nil {
		return errors.New("library is nil")
	}
	if format == "" {
		if strings.ToLower(filepath.Ext(path)) == ".json" {
			format = "json"
		} else {
			format = "html"
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	switch strings.ToLower(format) {
	case "json":
		return exportJSON(lib, path)
	case "html":
		return exportNetscapeHTML(lib, path)
	default:
		return fmt.Errorf("unsupported export format %q", format)
	}
}

func existingURLSet(lib *model.Library) map[string]bool {
	m := map[string]bool{}
	if lib == nil {
		return m
	}
	for _, b := range lib.Live() {
		if n := model.NormalizeURL(b.URL); n != "" {
			m[n] = true
		}
	}
	return m
}

func computeStats(f *app.ImportFolder, seen map[string]bool) {
	f.Count = 0
	f.Dups = 0
	for _, it := range f.Items {
		u := strings.TrimSpace(it.URL)
		if u == "" {
			continue
		}
		f.Count++
		if n := model.NormalizeURL(u); n != "" {
			if seen[n] {
				f.Dups++
			} else {
				seen[n] = true
			}
		}
	}
	for _, ch := range f.Children {
		computeStats(ch, seen)
		f.Count += ch.Count
		f.Dups += ch.Dups
	}
}

// CloneLibrary returns a deep copy of lib for safe Store.Replace calls.
func CloneLibrary(lib *model.Library) *model.Library {
	if lib == nil {
		return nil
	}
	cp := *lib
	cp.Groups = make([]model.Group, len(lib.Groups))
	for i, g := range lib.Groups {
		cp.Groups[i] = g
		cp.Groups[i].Subs = append([]model.Sub(nil), g.Subs...)
	}
	cp.Bookmarks = make([]model.Bookmark, len(lib.Bookmarks))
	for i, b := range lib.Bookmarks {
		cp.Bookmarks[i] = b
		if b.DeletedAt != nil {
			t := *b.DeletedAt
			cp.Bookmarks[i].DeletedAt = &t
		}
	}
	cp.Tombstones = append([]model.Tombstone(nil), lib.Tombstones...)
	return &cp
}

var tileColors = []string{"#2b2a27", "#0a6f8e", "#5b4a8a", "#24292f", "#a3262b", "#b2441b", "#2f5d8a", "#7a4a1e", "#b4295e", "#1f5f99", "#2f6fd0", "#b8510f", "#b0302a", "#4f52b2", "#2f62d6", "#b8590f"}

func defaultIcon(title, url string) model.Icon {
	s := strings.TrimSpace(title)
	if s == "" {
		s = strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
		s = strings.TrimPrefix(s, "www.")
	}
	letter := "?"
	for _, r := range s {
		letter = strings.ToUpper(string(r))
		break
	}
	h := 0
	for _, r := range url + title {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return model.Icon{Letter: letter, Color: tileColors[h%len(tileColors)]}
}
