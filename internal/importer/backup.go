package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
)

func parseJSONBackup(data []byte) (*app.ImportPlan, error) {
	clean := bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	rawObj, err := parseStrictTopLevelJSONObject(clean)
	if err != nil {
		return nil, err
	}
	rawGroups, hasGroups := rawObj["groups"]
	rawBookmarks, hasBookmarks := rawObj["bookmarks"]
	if !hasGroups && !hasBookmarks {
		return nil, errors.New(locales.ImportErrInvalidBackup())
	}
	validArray := false
	if hasGroups && !bytes.Equal(bytes.TrimSpace(rawGroups), []byte("null")) {
		if err := validateJSONObjectArray(rawGroups); err != nil {
			return nil, err
		}
		validArray = true
	}
	if hasBookmarks && !bytes.Equal(bytes.TrimSpace(rawBookmarks), []byte("null")) {
		if err := validateJSONObjectArray(rawBookmarks); err != nil {
			return nil, err
		}
		validArray = true
	}
	if !validArray {
		return nil, errors.New(locales.ImportErrInvalidBackup())
	}

	var lib model.Library
	if err := json.Unmarshal(clean, &lib); err != nil {
		return nil, errors.New(locales.ImportErrParseJSON(err.Error()))
	}
	if lib.SchemaVersion > model.SchemaVersion {
		return nil, errors.New(locales.ImportErrNewerSchema(lib.SchemaVersion))
	}
	if lib.SchemaVersion == 0 && len(lib.Groups) == 0 && len(lib.Bookmarks) == 0 {
		return nil, errors.New(locales.ImportErrInvalidBackup())
	}
	for _, g := range lib.Groups {
		if strings.TrimSpace(g.ID) == "" || strings.TrimSpace(g.Name) == "" {
			return nil, errors.New(locales.ImportErrInvalidBackup())
		}
		for _, sub := range g.Subs {
			if strings.TrimSpace(sub.ID) == "" || strings.TrimSpace(sub.Name) == "" {
				return nil, errors.New(locales.ImportErrInvalidBackup())
			}
		}
	}
	for _, b := range lib.Bookmarks {
		if strings.TrimSpace(b.ID) == "" || strings.TrimSpace(b.URL) == "" {
			return nil, errors.New(locales.ImportErrInvalidBackup())
		}
	}

	live := lib.Live()
	type indexedBM struct {
		idx int
		bm  model.Bookmark
	}
	byPlace := map[string][]indexedBM{}
	for idx, b := range live {
		if strings.TrimSpace(b.URL) == "" {
			continue
		}
		k := b.GroupID + "\x00" + b.SubID
		byPlace[k] = append(byPlace[k], indexedBM{idx: idx, bm: *b})
	}
	for k := range byPlace {
		items := byPlace[k]
		sort.SliceStable(items, func(i, j int) bool { return items[i].bm.Order < items[j].bm.Order })
		byPlace[k] = items
	}

	used := make([]bool, len(live))
	var folders []*app.ImportFolder
	for _, g := range lib.Groups {
		gf := &app.ImportFolder{Name: g.Name, Checked: true}
		for _, s := range g.Subs {
			sf := &app.ImportFolder{Name: s.Name, Checked: true}
			k := g.ID + "\x00" + s.ID
			for _, it := range byPlace[k] {
				used[it.idx] = true
				sf.Items = append(sf.Items, model.Bookmark{
					Title:     it.bm.Title,
					URL:       it.bm.URL,
					CreatedAt: it.bm.CreatedAt,
				})
			}
			gf.Children = append(gf.Children, sf)
		}
		var loose []indexedBM
		for idx, b := range live {
			if b.GroupID == g.ID && !used[idx] && strings.TrimSpace(b.URL) != "" {
				used[idx] = true
				loose = append(loose, indexedBM{idx: idx, bm: *b})
			}
		}
		sort.SliceStable(loose, func(i, j int) bool { return loose[i].bm.Order < loose[j].bm.Order })
		for _, it := range loose {
			gf.Items = append(gf.Items, model.Bookmark{
				Title:     it.bm.Title,
				URL:       it.bm.URL,
				CreatedAt: it.bm.CreatedAt,
			})
		}
		folders = append(folders, gf)
	}

	var orphanEntries []indexedBM
	for idx, b := range live {
		if !used[idx] && strings.TrimSpace(b.URL) != "" {
			orphanEntries = append(orphanEntries, indexedBM{idx: idx, bm: *b})
		}
	}
	sort.SliceStable(orphanEntries, func(i, j int) bool { return orphanEntries[i].bm.Order < orphanEntries[j].bm.Order })
	if len(orphanEntries) > 0 {
		orphans := make([]model.Bookmark, 0, len(orphanEntries))
		for _, it := range orphanEntries {
			orphans = append(orphans, model.Bookmark{
				Title:     it.bm.Title,
				URL:       it.bm.URL,
				CreatedAt: it.bm.CreatedAt,
			})
		}
		folders = append(folders, &app.ImportFolder{
			Name:    locales.Ungrouped(),
			Items:   orphans,
			Checked: true,
		})
	}

	return &app.ImportPlan{
		Format:  "json",
		Folders: folders,
		Backup:  &lib,
	}, nil
}

// parseStrictTopLevelJSONObject parses a top-level JSON object while rejecting
// duplicate or case-colliding keys (e.g. "groups" + "Groups") that could bypass
// schema validation during Go's case-insensitive struct unmarshaling.
func parseStrictTopLevelJSONObject(clean []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(clean))
	tok, err := dec.Token()
	if err != nil {
		return nil, errors.New(locales.ImportErrParseJSON(err.Error()))
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, errors.New(locales.ImportErrInvalidBackup())
	}
	out := map[string]json.RawMessage{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, errors.New(locales.ImportErrParseJSON(err.Error()))
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New(locales.ImportErrInvalidBackup())
		}
		low := strings.ToLower(key)
		if _, dup := out[low]; dup {
			return nil, errors.New(locales.ImportErrInvalidBackup())
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, errors.New(locales.ImportErrParseJSON(err.Error()))
		}
		out[low] = val
	}
	endTok, err := dec.Token()
	if err != nil {
		return nil, errors.New(locales.ImportErrParseJSON(err.Error()))
	}
	if delim, ok := endTok.(json.Delim); !ok || delim != '}' {
		return nil, errors.New(locales.ImportErrInvalidBackup())
	}
	if dec.More() {
		return nil, errors.New(locales.ImportErrInvalidBackup())
	}
	return out, nil
}

func validateJSONObjectArray(raw json.RawMessage) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return errors.New(locales.ImportErrInvalidBackup())
	}
	for _, item := range items {
		trimmed := bytes.TrimSpace(item)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return errors.New(locales.ImportErrInvalidBackup())
		}
	}
	return nil
}

func exportJSON(lib *model.Library, path string) error {
	live := lib.Live()
	bms := make([]model.Bookmark, len(live))
	for i, b := range live {
		bms[i] = *b
	}
	groups := lib.Groups
	if groups == nil {
		groups = []model.Group{}
	}
	out := model.Library{
		SchemaVersion: model.SchemaVersion,
		Revision:      lib.Revision,
		UpdatedAt:     time.Now(),
		DeviceID:      lib.DeviceID,
		Groups:        groups,
		Bookmarks:     bms,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
