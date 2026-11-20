package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
)

func parseJSONBackup(data []byte) (*app.ImportPlan, error) {
	clean := bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	var lib model.Library
	if err := json.Unmarshal(clean, &lib); err != nil {
		return nil, fmt.Errorf("解析 JSON 备份失败：%w", err)
	}
	if lib.SchemaVersion > model.SchemaVersion {
		return nil, fmt.Errorf("备份版本过新（schema %d）", lib.SchemaVersion)
	}
	if lib.SchemaVersion == 0 && len(lib.Groups) == 0 && len(lib.Bookmarks) == 0 {
		return nil, fmt.Errorf("不是有效的书签备份文件")
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
			Name:    "未分组",
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

func exportJSON(lib *model.Library, path string) error {
	live := lib.Live()
	bms := make([]model.Bookmark, len(live))
	for i, b := range live {
		bms[i] = *b
	}
	out := model.Library{
		SchemaVersion: model.SchemaVersion,
		Revision:      lib.Revision,
		UpdatedAt:     time.Now(),
		DeviceID:      lib.DeviceID,
		Groups:        lib.Groups,
		Bookmarks:     bms,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
