// Package update checks for new Cooool Mark releases from the published update feed.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/netproxy"
)

const (
	FeedURL       = "https://raw.githubusercontent.com/eachann1024/cooool-mark/main/updates/feed.json"
	GitHubRepoURL = "https://github.com/eachann1024/cooool-mark"
	LatestAPIURL  = "https://api.github.com/repos/eachann1024/cooool-mark/releases/latest"
)

// ReleaseInfo describes the latest available release.
type ReleaseInfo struct {
	Version     string
	Tag         string
	PublishedAt string
	HTMLURL     string
	DownloadURL string
	AssetName   string
	NotesZH     []string
	NotesEN     []string
}

// Notes returns release notes in the preferred language, falling back to the other.
func (r *ReleaseInfo) Notes(chinese bool) []string {
	if r == nil {
		return nil
	}
	if chinese && len(r.NotesZH) > 0 {
		return r.NotesZH
	}
	if !chinese && len(r.NotesEN) > 0 {
		return r.NotesEN
	}
	if len(r.NotesZH) > 0 {
		return r.NotesZH
	}
	return r.NotesEN
}

type feedDoc struct {
	LatestVersion string        `json:"latest_version"`
	LatestTag     string        `json:"latest_tag"`
	Releases      []feedRelease `json:"releases"`
}

type feedRelease struct {
	Version     string        `json:"version"`
	Tag         string        `json:"tag"`
	PublishedAt string        `json:"published_at"`
	HTMLURL     string        `json:"html_url"`
	ZH          feedLangNotes `json:"zh"`
	EN          feedLangNotes `json:"en"`
	Assets      []feedAsset   `json:"assets"`
}

type feedLangNotes struct {
	Highlights   string   `json:"highlights"`
	New          []string `json:"new"`
	Improvements []string `json:"improvements"`
	Fixes        []string `json:"fixes"`
	Other        []string `json:"other"`
}

func (n feedLangNotes) flatten() []string {
	var out []string
	if h := strings.TrimSpace(n.Highlights); h != "" {
		out = append(out, h)
	}
	for _, list := range [][]string{n.New, n.Improvements, n.Fixes, n.Other} {
		for _, item := range list {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

type feedAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

// Check queries the release feed and returns info for the latest release, plus
// whether it is strictly newer than currentVersion.
func Check(ctx context.Context, currentVersion string) (*ReleaseInfo, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	client := &http.Client{
		Transport: netproxy.Transport().Clone(),
		Timeout:   15 * time.Second,
	}

	info, err := fetchFromFeed(ctx, client)
	if err != nil {
		fallback, ferr := fetchFromGitHubAPI(ctx, client)
		if ferr != nil {
			return nil, false, err
		}
		info = fallback
	}
	if info == nil || info.Version == "" {
		return nil, false, nil
	}
	return info, CompareVersions(info.Version, currentVersion) > 0, nil
}

func fetchFromFeed(ctx context.Context, client *http.Client) (*ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, FeedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Cooool-Mark")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var doc feedDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Releases) == 0 {
		return nil, nil
	}
	rel := doc.Releases[0]
	ver := strings.TrimPrefix(strings.TrimSpace(rel.Version), "v")
	if ver == "" {
		ver = strings.TrimPrefix(strings.TrimSpace(doc.LatestVersion), "v")
	}
	tag := strings.TrimSpace(rel.Tag)
	if tag == "" {
		tag = doc.LatestTag
	}
	htmlURL := strings.TrimSpace(rel.HTMLURL)
	if htmlURL == "" && tag != "" {
		htmlURL = GitHubRepoURL + "/releases/tag/" + tag
	}
	asset := pickAsset(rel.Assets)
	info := &ReleaseInfo{
		Version:     ver,
		Tag:         tag,
		PublishedAt: rel.PublishedAt,
		HTMLURL:     htmlURL,
		NotesZH:     rel.ZH.flatten(),
		NotesEN:     rel.EN.flatten(),
	}
	if asset != nil {
		info.DownloadURL = asset.DownloadURL
		info.AssetName = asset.Name
	}
	return info, nil
}

func fetchFromGitHubAPI(ctx context.Context, client *http.Client) (*ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LatestAPIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Cooool-Mark")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var gh struct {
		TagName     string      `json:"tag_name"`
		PublishedAt string      `json:"published_at"`
		HTMLURL     string      `json:"html_url"`
		Assets      []feedAsset `json:"assets"`
	}
	if err := json.Unmarshal(data, &gh); err != nil {
		return nil, err
	}
	ver := strings.TrimPrefix(strings.TrimSpace(gh.TagName), "v")
	asset := pickAsset(gh.Assets)
	info := &ReleaseInfo{
		Version:     ver,
		Tag:         gh.TagName,
		PublishedAt: gh.PublishedAt,
		HTMLURL:     gh.HTMLURL,
	}
	if asset != nil {
		info.DownloadURL = asset.DownloadURL
		info.AssetName = asset.Name
	}
	return info, nil
}

func pickAsset(assets []feedAsset) *feedAsset {
	var prefs []string
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			prefs = []string{"macos-arm64.dmg", "darwin-arm64"}
		} else {
			prefs = []string{"macos-x64.dmg", "darwin-amd64"}
		}
	case "windows":
		prefs = []string{"windows-x64-setup.exe", "windows-x64", ".exe"}
	case "linux":
		prefs = []string{"linux-x64.AppImage", "linux-x64.deb", ".AppImage", ".deb"}
	}
	for _, want := range prefs {
		for i := range assets {
			if strings.Contains(assets[i].Name, want) && assets[i].DownloadURL != "" {
				return &assets[i]
			}
		}
	}
	return nil
}

// CompareVersions compares two semver strings like "1.2.3" or "v1.2.3".
// Returns > 0 if a > b, < 0 if a < b, 0 if equal.
func CompareVersions(a, b string) int {
	pa := parseSemver(a)
	pb := parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseSemver(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if idx := strings.IndexAny(s, "-+"); idx >= 0 {
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}
