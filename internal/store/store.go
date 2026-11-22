// Package store keeps the library in one open folder that sync services
// can carry: <dir>/bookmarks.json, <dir>/icons/<hash>.png and
// <dir>/backups/. Writes are debounced and atomic; a change written by
// another device is merged record by record (newest UpdatedAt wins,
// tombstones win over older records).
//
// Every method but the watcher's goroutine runs on the main thread; the
// watcher hands merges back through the post function given to Open.
package store

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync/atomic"
	"time"

	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
)

const (
	fileName    = "bookmarks.json"
	saveDelay   = 400 * time.Millisecond
	pollEvery   = time.Second
	backupsKept = 14
)

// Store owns the library.
type Store struct {
	dir    atomic.Pointer[string]
	device string
	post   func(func()) // runs fn on the main thread
	lib    *model.Library

	timer       *time.Timer
	dirty       bool
	lastSum     string // hash of the bytes we last wrote or read
	onChange    []func()
	onSaveError func(error)
	err         error
}

// Open loads (or creates) the library in dir. post must run a function on
// the main thread, as mygo.RunOnMain or Window.Update does.
func Open(dir, deviceID string, post func(func())) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "icons"), 0o755); err != nil {
		return nil, err
	}
	s := &Store{device: deviceID, post: post}
	s.dir.Store(&dir)
	lib, sum, err := read(filepath.Join(dir, fileName))
	switch {
	case errors.Is(err, os.ErrNotExist):
		lib = Fresh()
		s.dirty = true
	case err != nil:
		return nil, err
	}
	s.lib, s.lastSum = lib, sum
	if s.dirty {
		if err := s.Flush(); err != nil {
			return nil, err
		}
	}
	s.dropLegacyTrash()
	return s, nil
}

// Fresh is the library of a first start: one group with one sub-group,
// so a new bookmark always has a home.
func Fresh() *model.Library {
	now := time.Now()
	return &model.Library{
		SchemaVersion: model.SchemaVersion,
		UpdatedAt:     now,
		Groups: []model.Group{{
			ID: model.NewID(), Name: locales.DefaultGroup(), UpdatedAt: now,
			Subs: []model.Sub{{ID: model.NewID(), Name: locales.DefaultSub(), UpdatedAt: now}},
		}},
	}
}

// Dir is the library folder.
func (s *Store) Dir() string { return *s.dir.Load() }

// IconsDir is where fetched icons go.
func (s *Store) IconsDir() string { return filepath.Join(s.Dir(), "icons") }

// IconPath resolves a bookmark's Icon.Image to a full path ("" if none).
func (s *Store) IconPath(icon model.Icon) string {
	if icon.Image == "" {
		return ""
	}
	return filepath.Join(s.Dir(), filepath.FromSlash(icon.Image))
}

// SaveIcon stores image bytes under icons/<sha1>.<ext> and returns the
// value for Icon.Image.
func (s *Store) SaveIcon(data []byte, ext string) (string, error) {
	sum := sha1.Sum(data)
	name := "icons/" + hex.EncodeToString(sum[:10]) + ext
	path := filepath.Join(s.Dir(), filepath.FromSlash(name))
	if _, err := os.Stat(path); err == nil {
		return name, nil
	}
	return name, WriteAtomic(path, data)
}

// Lib is the library. Read it on the main thread; change it with Update.
func (s *Store) Lib() *model.Library { return s.lib }

// Err is the last save error, if any.
func (s *Store) Err() error { return s.err }

// OnChange registers fn, called on the main thread after every change,
// local (Update) or merged from another device.
func (s *Store) OnChange(fn func()) { s.onChange = append(s.onChange, fn) }

func (s *Store) OnSaveError(fn func(error)) { s.onSaveError = fn }

func (s *Store) changed() {
	for _, fn := range s.onChange {
		fn()
	}
}

// Update changes the library and schedules a save. fn must set UpdatedAt
// on the records it changes (Touch helps).
func (s *Store) Update(fn func(l *model.Library)) {
	fn(s.lib)
	s.lib.Revision++
	s.lib.UpdatedAt = time.Now()
	s.lib.DeviceID = s.device
	s.schedule()
	s.changed()
}

// Replace swaps the whole library (import with "restore"), keeping a
// snapshot in backups/ first.
func (s *Store) Replace(lib *model.Library) error {
	if err := s.Flush(); err != nil {
		return err
	}
	if _, err := s.Backup("before-restore"); err != nil {
		return err
	}
	now := time.Now()
	restored := make(map[string]bool)
	for i := range lib.Groups {
		g := &lib.Groups[i]
		restored[g.ID] = true
		g.UpdatedAt = now
		for j := range g.Subs {
			restored[g.Subs[j].ID] = true
			g.Subs[j].UpdatedAt = now
		}
	}
	for i := range lib.Bookmarks {
		b := &lib.Bookmarks[i]
		restored[b.ID] = true
		b.UpdatedAt = now
	}
	tombstones := make(map[string]time.Time)
	deleted := func(id string, at time.Time) {
		if !restored[id] && at.After(tombstones[id]) {
			tombstones[id] = at
		}
	}
	for _, t := range lib.Tombstones {
		deleted(t.ID, t.DeletedAt)
	}
	for _, t := range s.lib.Tombstones {
		deleted(t.ID, t.DeletedAt)
	}
	for _, g := range s.lib.Groups {
		deleted(g.ID, now)
		for _, sub := range g.Subs {
			deleted(sub.ID, now)
		}
	}
	for _, b := range s.lib.Bookmarks {
		deleted(b.ID, now)
	}
	lib.Tombstones = sortedTombstones(tombstones)
	previous := s.lib
	s.lib = lib
	s.lib.Revision = previous.Revision + 1
	s.lib.SchemaVersion = model.SchemaVersion
	s.lib.UpdatedAt = now
	s.lib.DeviceID = s.device
	s.dirty = true
	if err := s.persist(); err != nil {
		s.lib, s.dirty = previous, false
		return err
	}
	s.changed()
	return nil
}

func (s *Store) schedule() {
	s.dirty = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(saveDelay, func() { s.post(func() { _ = s.Flush() }) })
}

// Flush writes the library now (call before quitting).
func (s *Store) Flush() error {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if !s.dirty {
		return nil
	}
	other, sum, err := read(filepath.Join(s.Dir(), fileName))
	switch {
	case err == nil && sum != s.lastSum:
		if s.mergeIncoming(other, sum) {
			s.changed()
		}
		if !s.dirty {
			return nil
		}
	case err != nil && !errors.Is(err, os.ErrNotExist):
		s.reportSaveError(err)
		return err
	}
	return s.persist()
}

func marshalLibrary(lib *model.Library) ([]byte, error) {
	cp := *lib
	if cp.Groups == nil {
		cp.Groups = []model.Group{}
	}
	if cp.Bookmarks == nil {
		cp.Bookmarks = []model.Bookmark{}
	}
	return json.MarshalIndent(cp, "", "  ")
}

func (s *Store) persist() error {
	data, err := marshalLibrary(s.lib)
	if err != nil {
		s.reportSaveError(err)
		return err
	}
	sum := hash(data)
	if sum == s.lastSum {
		s.err = nil
		s.dirty = false
		return nil
	}
	err = WriteAtomic(filepath.Join(s.Dir(), fileName), data)
	if err == nil {
		s.err = nil
		s.dirty = false
		s.lastSum = sum
	} else {
		s.reportSaveError(err)
	}
	return err
}

func (s *Store) reportSaveError(err error) {
	s.err = err
	if s.onSaveError != nil {
		s.onSaveError(err)
	}
}

// Watch polls bookmarks.json for writes by other devices (sync services
// replace the file, which file events report unreliably across iCloud,
// OneDrive and Syncthing) and merges them. It returns when ctx ends.
func (s *Store) Watch(ctx context.Context, merged func(added int)) {
	var lastDir *string
	var lastFile os.FileInfo
	t := time.NewTicker(pollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		dir := s.dir.Load()
		path := filepath.Join(*dir, fileName)
		st, err := os.Stat(path)
		if err != nil || (dir == lastDir && sameVersion(st, lastFile)) {
			continue
		}
		other, sum, err := read(path)
		if err != nil {
			continue
		}
		lastDir, lastFile = dir, st
		s.post(func() {
			if ctx.Err() != nil || dir != s.dir.Load() || sum == s.lastSum {
				return
			}
			current, err := os.Stat(path)
			if err != nil || !sameVersion(st, current) {
				return
			}
			before := len(s.lib.Live())
			changed := s.mergeIncoming(other, sum)
			if s.dirty {
				s.schedule() // write the union back only if it adds changes
			}
			if !changed {
				return
			}
			s.changed()
			if merged != nil {
				merged(len(s.lib.Live()) - before)
			}
		})
	}
}

func (s *Store) mergeIncoming(other *model.Library, sum string) bool {
	union := Merge(s.lib, other)
	s.lastSum = sum
	s.dirty = !sameContent(other, union)
	if !s.dirty {
		s.err = nil
	}
	if sameContent(s.lib, union) {
		return false
	}
	union.Revision++
	s.lib = union
	return true
}

// Libraries can serialize in different insertion orders and have different
// device, revision and save timestamps without containing different records.
func sameContent(a, b *model.Library) bool {
	return slices.EqualFunc(a.Groups, b.Groups, func(x, y model.Group) bool {
		return x.ID == y.ID && x.Name == y.Name && x.Order == y.Order &&
			x.UpdatedAt.Equal(y.UpdatedAt) && slices.EqualFunc(x.Subs, y.Subs, func(x, y model.Sub) bool {
			return x.ID == y.ID && x.Name == y.Name && x.Order == y.Order && x.UpdatedAt.Equal(y.UpdatedAt)
		})
	}) && sameRecords(a.Bookmarks, b.Bookmarks, func(b model.Bookmark) string { return b.ID }, sameBookmark) &&
		sameRecords(a.Tombstones, b.Tombstones, func(t model.Tombstone) string { return t.ID },
			func(x, y model.Tombstone) bool { return x.DeletedAt.Equal(y.DeletedAt) })
}

func sameBookmark(a, b model.Bookmark) bool {
	return a.ID == b.ID && a.Title == b.Title && a.URL == b.URL && a.Note == b.Note &&
		a.Icon == b.Icon && a.GroupID == b.GroupID &&
		a.SubID == b.SubID && a.Order == b.Order && a.CreatedAt.Equal(b.CreatedAt) &&
		a.UpdatedAt.Equal(b.UpdatedAt) && a.Polished == b.Polished &&
		(a.DeletedAt == nil && b.DeletedAt == nil || a.DeletedAt != nil && b.DeletedAt != nil && a.DeletedAt.Equal(*b.DeletedAt))
}

func sameRecords[T any](a, b []T, id func(T) string, equal func(T, T) bool) bool {
	if len(a) != len(b) {
		return false
	}
	records := make(map[string]T, len(a))
	for _, v := range a {
		records[id(v)] = v
	}
	for _, v := range b {
		if x, ok := records[id(v)]; !ok || !equal(x, v) {
			return false
		}
	}
	return true
}

func sameVersion(a, b os.FileInfo) bool {
	return b != nil && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && os.SameFile(a, b)
}

// MoveTo copies the library and its icons to dir and uses it from then
// on. If dir already holds a library (another device's), the two merge.
func (s *Store) MoveTo(dir string, commit func() error) error {
	if err := s.Flush(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "icons"), 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.IconsDir())
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		dst := filepath.Join(dir, "icons", e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := copyFile(filepath.Join(s.IconsDir(), e.Name()), dst); err != nil {
			return err
		}
	}
	oldDir, oldLib, oldSum := s.dir.Load(), s.lib, s.lastSum
	if other, _, err := read(filepath.Join(dir, fileName)); err == nil {
		s.lib = Merge(s.lib, other)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.dir.Store(&dir)
	s.lastSum = ""
	s.dirty = true
	if err := s.persist(); err != nil {
		s.dir.Store(oldDir)
		s.lib, s.lastSum, s.dirty = oldLib, oldSum, false
		return err
	}
	if err := commit(); err != nil {
		s.dir.Store(oldDir)
		s.lib, s.lastSum = oldLib, oldSum
		return err
	}
	s.changed()
	return nil
}

// Backup writes a copy to backups/<date>-<label>.json, keeps the newest
// backupsKept and returns its path.
func (s *Store) Backup(label string) (string, error) {
	dir := filepath.Join(s.Dir(), "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := marshalLibrary(s.lib)
	if err != nil {
		return "", err
	}
	name := time.Now().Format("2006-01-02")
	if label != "" {
		name += "-" + label
		if label == "before-restore" {
			name += "-" + time.Now().Format("150405.000000000")
		}
	}
	path := filepath.Join(dir, name+".json")
	if err := WriteAtomic(path, data); err != nil {
		return "", err
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) > backupsKept {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, n := range names[:len(names)-backupsKept] {
			_ = os.Remove(filepath.Join(dir, n))
		}
	}
	return path, nil
}

// DailyBackup makes today's backup once.
func (s *Store) DailyBackup() error {
	path := filepath.Join(s.Dir(), "backups", time.Now().Format("2006-01-02")+".json")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	_, err := s.Backup("")
	return err
}

// dropLegacyTrash removes any legacy deletedAt records from older files.
func (s *Store) dropLegacyTrash() {
	out := s.lib.Bookmarks[:0]
	dropped := false
	for _, b := range s.lib.Bookmarks {
		if b.InTrash() {
			dropped = true
			continue
		}
		out = append(out, b)
	}
	if dropped {
		s.lib.Bookmarks = out
		s.lib.Revision++
		s.dirty = true
		_ = s.Flush()
	}
}

// Purge removes bookmarks for good, leaving tombstones.
func Purge(l *model.Library, ids ...string) {
	gone := map[string]bool{}
	for _, id := range ids {
		gone[id] = true
	}
	now := time.Now()
	out := l.Bookmarks[:0]
	for _, b := range l.Bookmarks {
		if gone[b.ID] {
			l.Tombstones = append(l.Tombstones, model.Tombstone{ID: b.ID, DeletedAt: now})
			continue
		}
		out = append(out, b)
	}
	l.Bookmarks = out
}

// Merge returns the union of two libraries: per record the newer
// UpdatedAt wins; a tombstone newer than a record removes it. Groups merge
// by id, their sub-groups by id too.
func Merge(a, b *model.Library) *model.Library {
	out := &model.Library{
		SchemaVersion: model.SchemaVersion,
		Revision:      max(a.Revision, b.Revision),
		UpdatedAt:     time.Now(),
		DeviceID:      a.DeviceID,
	}
	tomb := map[string]time.Time{}
	for _, t := range append(append([]model.Tombstone{}, a.Tombstones...), b.Tombstones...) {
		if t.DeletedAt.After(tomb[t.ID]) {
			tomb[t.ID] = t.DeletedAt
		}
	}
	dead := func(id string, updated time.Time) bool {
		t, ok := tomb[id]
		return ok && !updated.After(t)
	}

	groups := map[string]model.Group{}
	var gorder []string
	for _, g := range append(append([]model.Group{}, a.Groups...), b.Groups...) {
		cur, ok := groups[g.ID]
		if !ok {
			gorder = append(gorder, g.ID)
			groups[g.ID] = g
			continue
		}
		newer, older := g, cur
		if cur.UpdatedAt.After(g.UpdatedAt) {
			newer, older = cur, g
		}
		subs := map[string]model.Sub{}
		var sorder []string
		for _, s := range append(append([]model.Sub{}, newer.Subs...), older.Subs...) {
			if c, ok := subs[s.ID]; !ok {
				sorder = append(sorder, s.ID)
				subs[s.ID] = s
			} else if s.UpdatedAt.After(c.UpdatedAt) {
				subs[s.ID] = s
			}
		}
		newer.Subs = newer.Subs[:0:0]
		for _, id := range sorder {
			if !dead(id, subs[id].UpdatedAt) {
				newer.Subs = append(newer.Subs, subs[id])
			}
		}
		groups[g.ID] = newer
	}
	for _, id := range gorder {
		if g := groups[id]; !dead(id, g.UpdatedAt) {
			g.Subs = slices.DeleteFunc(slices.Clone(g.Subs), func(s model.Sub) bool { return dead(s.ID, s.UpdatedAt) })
			sort.SliceStable(g.Subs, func(i, j int) bool {
				a, b := g.Subs[i], g.Subs[j]
				return a.Order < b.Order || a.Order == b.Order && a.ID < b.ID
			})
			out.Groups = append(out.Groups, g)
		}
	}
	sort.SliceStable(out.Groups, func(i, j int) bool {
		a, b := out.Groups[i], out.Groups[j]
		return a.Order < b.Order || a.Order == b.Order && a.ID < b.ID
	})
	// A moved sub-group keeps its ID. Drop its older copy under the
	// previous parent when a synced file has pre-move records.
	type owner struct {
		group string
		at    time.Time
	}
	owners := make(map[string]owner)
	for _, g := range out.Groups {
		for _, sub := range g.Subs {
			best, exists := owners[sub.ID]
			if !exists || sub.UpdatedAt.After(best.at) || sub.UpdatedAt.Equal(best.at) && g.ID < best.group {
				owners[sub.ID] = owner{g.ID, sub.UpdatedAt}
			}
		}
	}
	for i := range out.Groups {
		g := &out.Groups[i]
		g.Subs = slices.DeleteFunc(g.Subs, func(sub model.Sub) bool { return owners[sub.ID].group != g.ID })
	}

	bms := map[string]model.Bookmark{}
	var border []string
	for _, bm := range append(append([]model.Bookmark{}, a.Bookmarks...), b.Bookmarks...) {
		if c, ok := bms[bm.ID]; !ok {
			border = append(border, bm.ID)
			bms[bm.ID] = bm
		} else if bm.UpdatedAt.After(c.UpdatedAt) {
			bms[bm.ID] = bm
		}
	}
	for _, id := range border {
		if bm := bms[id]; !dead(id, bm.UpdatedAt) {
			if target, ok := owners[bm.SubID]; ok {
				bm.GroupID = target.group
			}
			out.Bookmarks = append(out.Bookmarks, bm)
		}
	}
	out.Tombstones = sortedTombstones(tomb)
	return out
}

func sortedTombstones(tomb map[string]time.Time) []model.Tombstone {
	var result []model.Tombstone
	for id, at := range tomb {
		result = append(result, model.Tombstone{ID: id, DeletedAt: at})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func read(path string) (*model.Library, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var lib model.Library
	if err := json.Unmarshal(data, &lib); err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	if lib.SchemaVersion > model.SchemaVersion {
		return nil, "", fmt.Errorf("%s was written by a newer Cooool Mark (schema %d)", path, lib.SchemaVersion)
	}
	return &lib, hash(data), nil
}

func hash(b []byte) string { s := sha1.Sum(b); return hex.EncodeToString(s[:]) }

func WriteAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	return WriteAtomic(dst, data)
}
