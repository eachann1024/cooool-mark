package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"cooool-mark/internal/model"
	"cooool-mark/internal/store"
)

// ---- bookmarks ----

// SaveBookmark adds b (ID "") or replaces the bookmark with its ID. It
// returns the id. New bookmarks go to the end of their sub-group. With AI
// auto-polish on and the bookmark not yet polished, a background pass
// fills empty fields afterwards (never blocking the save).
func (a *App) SaveBookmark(b model.Bookmark) string {
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
		a.Say("已保存到「" + a.PathOf(&b) + "」")
		a.Polish(b.ID, false)
	}
	return b.ID
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

// Polish asks the AI to fill a saved bookmark's empty title/note/tags.
// A manual request can replace fields and change its sub-group. Results
// land quietly; the row flashes "已润色" through PolishedAt.
func (a *App) Polish(id string, force bool) {
	if a.AI == nil || (!force && !a.Settings.AIAutoPolish) || !a.AI.Status().Ready {
		return
	}
	b := a.Store.Lib().Bookmark(id)
	if b == nil || (b.Polished && !force) {
		return
	}
	dir, updated := a.Store.Dir(), b.UpdatedAt
	in := PolishInput{URL: b.URL, Title: b.Title, Description: b.Note, Choices: a.Choices()}
	Background(60*time.Second, func(ctx context.Context) {
		p, err := a.AI.Polish(ctx, in)
		a.Post(func() {
			current := a.Store.Lib().Bookmark(id)
			if current == nil || current.InTrash() || a.Store.Dir() != dir || !current.UpdatedAt.Equal(updated) {
				return
			}
			if err != nil {
				if force {
					a.SayErr("润色失败", "重试", func() { a.Polish(id, true) })
				}
				return
			}
			a.Store.Update(func(l *model.Library) {
				b := l.Bookmark(id)
				if b == nil {
					return
				}
				if p.Title != "" && (force || b.Title == "" || b.Title == b.URL) {
					b.Title = p.Title
				}
				if p.Note != "" && (force || b.Note == "") {
					b.Note = p.Note
				}
				if len(p.Tags) > 0 && (force || len(b.Tags) == 0) {
					b.Tags = p.Tags
				}
				if force && p.GroupID != "" && l.Sub(p.GroupID, p.SubID) != nil {
					if b.GroupID != p.GroupID || b.SubID != p.SubID {
						b.Order = nextOrder(l, p.GroupID, p.SubID)
						b.GroupID, b.SubID = p.GroupID, p.SubID
					}
				}
				b.Polished = true
				b.UpdatedAt = time.Now()
			})
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

// Move moves bookmarks to a sub-group (single membership: they leave
// where they were), with undo.
func (a *App) Move(ids []string, groupID, subID string) {
	if a.Store.Lib().Sub(groupID, subID) == nil {
		a.SayErr("目标子分组已找不到", "", nil)
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
	text := "已移到「" + path + "」"
	if len(ids) > 1 {
		text = "已移动 " + itoa(len(ids)) + " 项"
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
				cp := b
				cp.Tags = append([]string(nil), b.Tags...)
				saved = append(saved, cp)
			}
		}
		store.Purge(l, ids...)
	})
	if len(saved) == 0 {
		return
	}
	text := "已删除「" + saved[0].Title + "」"
	if len(saved) > 1 {
		text = "已删除 " + itoa(len(saved)) + " 项"
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
		Subs: []model.Sub{{ID: model.NewID(), Name: "常用", UpdatedAt: now}}}
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
		out := l.Groups[:0]
		for _, x := range l.Groups {
			if x.ID != id {
				out = append(out, x)
			}
		}
		l.Groups = out
		l.Tombstones = append(l.Tombstones, model.Tombstone{ID: id, DeletedAt: time.Now()})
	})
	a.EnsurePlace()
	a.SayUndo("已删除「"+saved.Name+"」", func() {
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
			out := g.Subs[:0]
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
	a.SayUndo("已删除「"+saved.Name+"」", func() {
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
			Subs: []model.Sub{{ID: subID, Name: "常用", UpdatedAt: now}}}
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
				cp := b
				cp.Tags = append([]string(nil), b.Tags...)
				removed = append(removed, cp)
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
	out := l.Tombstones[:0]
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
