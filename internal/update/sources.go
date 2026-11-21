package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/netproxy"
)

// SourceMode controls how installer downloads choose between acceleration mirrors and native GitHub.
type SourceMode string

const (
	SourceAuto   SourceMode = "auto"
	SourceMirror SourceMode = "mirror"
	SourceGitHub SourceMode = "github"
)

// NormalizeSourceMode turns a persisted string into a valid SourceMode.
func NormalizeSourceMode(raw string) SourceMode {
	switch SourceMode(strings.ToLower(strings.TrimSpace(raw))) {
	case SourceMirror:
		return SourceMirror
	case SourceGitHub:
		return SourceGitHub
	default:
		return SourceAuto
	}
}

const (
	PublicRepo           = "eachann1024/cooool-mark"
	PublicReleasesWebURL = "https://github.com/eachann1024/cooool-mark/releases"
	maxFeedBytes         = 4 * 1024 * 1024
	headerTimeout        = 12 * time.Second
	stallTimeout         = 20 * time.Second
)

var allowlistedInstallers = map[string]bool{
	"cooool-mark-macos-arm64.dmg":       true,
	"cooool-mark-macos-x64.dmg":         true,
	"cooool-mark-windows-x64-setup.exe": true,
	"cooool-mark-linux-x64.deb":         true,
	"cooool-mark-linux-x64.AppImage":    true,
}

// LabeledURL pairs a candidate endpoint with its channel kind.
type LabeledURL struct {
	URL     string
	Channel string // "mirror" | "github"
	Kind    string // "feed" | "github-api" | "asset"
}

// IsTrustedReleaseAsset reports whether an installer is on the allowlist, lives at the official
// release URL for tag, and carries a valid SHA-256.
func IsTrustedReleaseAsset(asset ReleaseAsset, tag string) bool {
	if !allowlistedInstallers[asset.Name] || asset.EffectiveSHA256() == "" {
		return false
	}
	cleanTag := strings.TrimSpace(tag)
	vTag := cleanTag
	if cleanTag != "" && !strings.HasPrefix(strings.ToLower(cleanTag), "v") {
		vTag = "v" + cleanTag
	}
	expected := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", PublicRepo, url.PathEscape(cleanTag), asset.Name)
	expectedV := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", PublicRepo, url.PathEscape(vTag), asset.Name)
	return asset.DownloadURL == expected || asset.DownloadURL == expectedV
}

func newHTTPClient(respHeaderTimeout time.Duration) *http.Client {
	transport := netproxy.Transport().Clone()
	transport.ResponseHeaderTimeout = respHeaderTimeout
	return &http.Client{Transport: transport}
}

var sharedHTTPClient = newHTTPClient(headerTimeout)

type ghAPIRawRelease struct {
	TagName     string         `json:"tag_name"`
	HTMLURL     string         `json:"html_url"`
	PublishedAt string         `json:"published_at"`
	Body        string         `json:"body"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	Assets      []ReleaseAsset `json:"assets"`
}

func userAgent() string { return "CooolMark-Updater/" + CurrentVersion() }

// feedCandidates lists the authoritative metadata endpoints. Mirrors only ever carry installer
// bytes; the expected digest always comes from the official repository.
func feedCandidates() []LabeledURL {
	return []LabeledURL{
		{URL: fmt.Sprintf("https://raw.githubusercontent.com/%s/main/updates/feed.json?t=%d", PublicRepo, time.Now().Unix()), Channel: "github", Kind: "feed"},
		{URL: fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=30", PublicRepo), Channel: "github", Kind: "github-api"},
	}
}

// CandidateAssetURLs expands an official release asset URL into ordered mirror and native download URLs.
func CandidateAssetURLs(canonicalURL string, mode SourceMode) []LabeledURL {
	if !strings.HasPrefix(canonicalURL, "https://github.com/"+PublicRepo+"/releases/download/") {
		return nil
	}
	mirrorA := LabeledURL{URL: "https://gh-proxy.com/" + canonicalURL, Channel: "mirror", Kind: "asset"}
	mirrorB := LabeledURL{URL: "https://ghproxy.net/" + canonicalURL, Channel: "mirror", Kind: "asset"}
	native := LabeledURL{URL: canonicalURL, Channel: "github", Kind: "asset"}
	switch mode {
	case SourceMirror:
		return []LabeledURL{mirrorA, mirrorB, native}
	case SourceGitHub:
		return []LabeledURL{native}
	default:
		return []LabeledURL{mirrorA, native, mirrorB}
	}
}

func fetchMetadata(ctx context.Context, endpoint string) ([]byte, int64, error) {
	started := time.Now()
	reqCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("Cache-Control", "no-cache")

	client := *sharedHTTPClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 || next.URL.Scheme != "https" || next.URL.Host != req.URL.Host {
			return errors.New("untrusted release metadata redirect")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, time.Since(started).Milliseconds(), err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, time.Since(started).Milliseconds(), httpStatusError{code: resp.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes+1))
	elapsed := time.Since(started).Milliseconds()
	if err != nil {
		return nil, elapsed, err
	}
	if len(raw) > maxFeedBytes {
		return nil, elapsed, errors.New("update metadata exceeds the size limit")
	}
	return raw, elapsed, nil
}

type httpStatusError struct{ code int }

func (e httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.code) }

func fetchOneCandidate(ctx context.Context, cand LabeledURL) ([]ReleaseEntry, []byte, int64, error) {
	raw, elapsed, err := fetchMetadata(ctx, cand.URL)
	if err != nil {
		return nil, nil, elapsed, err
	}
	if cand.Kind == "feed" {
		entries, parseErr := ParseFeedBytes(raw)
		if parseErr != nil {
			return nil, nil, elapsed, parseErr
		}
		return entries, raw, elapsed, nil
	}
	entries, parseErr := parseGitHubAPIReleases(raw)
	if parseErr != nil {
		return nil, nil, elapsed, parseErr
	}
	feedBytes, _ := json.Marshal(FeedDocument{
		SchemaVersion: 1,
		Repo:          PublicRepo,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		LatestVersion: entries[0].Version,
		LatestTag:     entries[0].Tag,
		Releases:      entries,
	})
	return entries, feedBytes, elapsed, nil
}

func trustedAssets(assets []ReleaseAsset, tag string) []ReleaseAsset {
	filtered := make([]ReleaseAsset, 0, len(assets))
	for _, asset := range assets {
		if IsTrustedReleaseAsset(asset, tag) {
			asset.SHA256 = asset.EffectiveSHA256()
			filtered = append(filtered, asset)
		}
	}
	return filtered
}

// ParseFeedBytes validates and decodes updates/feed.json.
func ParseFeedBytes(raw []byte) ([]ReleaseEntry, error) {
	var doc FeedDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if len(doc.Releases) == 0 {
		return nil, errors.New("empty update feed")
	}
	out := make([]ReleaseEntry, 0, len(doc.Releases))
	for idx, r := range doc.Releases {
		entry, ok := normalizeEntry(r)
		if !ok {
			continue
		}
		entry.Assets = trustedAssets(r.Assets, entry.Tag)
		if idx == 0 && len(r.Assets) > 0 && len(entry.Assets) == 0 {
			logWarn("check.feed_assets_rejected", "version", entry.Version, "tag", entry.Tag, "raw_assets", len(r.Assets))
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil, errors.New("no valid releases in feed")
	}
	slices.SortFunc(out, func(a, b ReleaseEntry) int { return CompareVersion(b.Version, a.Version) })
	for _, entry := range out {
		if len(entry.Assets) > 0 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("release feed starting at %s has no trusted installer assets", out[0].Tag)
}

func releaseFromGitHub(item ghAPIRawRelease) (*ReleaseEntry, bool) {
	ver, tag, _, ok := ParseReleaseTag(item.TagName)
	if !ok || item.Draft || item.Prerelease {
		return nil, false
	}
	zh, en := ParseReleaseBody(item.Body)
	htmlURL := item.HTMLURL
	if htmlURL == "" {
		htmlURL = fmt.Sprintf("https://github.com/%s/releases/tag/%s", PublicRepo, url.PathEscape(tag))
	}
	return &ReleaseEntry{
		Version:     ver,
		Tag:         tag,
		PublishedAt: item.PublishedAt,
		HTMLURL:     htmlURL,
		Zh:          zh,
		En:          en,
		Assets:      trustedAssets(item.Assets, tag),
	}, true
}

func parseGitHubAPIReleases(raw []byte) ([]ReleaseEntry, error) {
	var list []ghAPIRawRelease
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]ReleaseEntry, 0, len(list))
	for _, item := range list {
		if entry, ok := releaseFromGitHub(item); ok {
			out = append(out, *entry)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no published releases found")
	}
	slices.SortFunc(out, func(a, b ReleaseEntry) int { return CompareVersion(b.Version, a.Version) })
	return out, nil
}

var errReleaseNotFound = errors.New("release tag not found")

// FetchReleaseByTag retrieves one published release directly from GitHub by exact tag.
func FetchReleaseByTag(ctx context.Context, tag string) (*ReleaseEntry, error) {
	cleanTag := strings.TrimSpace(tag)
	if cleanTag == "" {
		return nil, errReleaseNotFound
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", PublicRepo, url.PathEscape(cleanTag))
	raw, _, err := fetchMetadata(ctx, endpoint)
	if err != nil {
		var statusErr httpStatusError
		if errors.As(err, &statusErr) && statusErr.code == http.StatusNotFound {
			return nil, fmt.Errorf("%w: HTTP 404", errReleaseNotFound)
		}
		return nil, err
	}
	var item ghAPIRawRelease
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	if item.Draft || item.Prerelease {
		return nil, errors.New("release is not published")
	}
	entry, ok := releaseFromGitHub(item)
	if !ok {
		return nil, errors.New("release tag is invalid")
	}
	return entry, nil
}

// FetchReleases retrieves authoritative release metadata directly from GitHub.
func FetchReleases(ctx context.Context) (entries []ReleaseEntry, channel string, latencyMs int64, err error) {
	var lastErr error
	for _, cand := range feedCandidates() {
		if ctx.Err() != nil {
			return nil, "", 0, ctx.Err()
		}
		found, _, elapsed, fetchErr := fetchOneCandidate(ctx, cand)
		if fetchErr == nil && len(found) > 0 {
			logInfo("check.candidate_ok", "kind", cand.Kind, "latency_ms", elapsed, "releases", len(found), "latest", found[0].Version)
			return found, cand.Channel, elapsed, nil
		}
		logWarn("check.candidate_failover", "kind", cand.Kind, "latency_ms", elapsed, "err", fetchErr)
		lastErr = fetchErr
	}
	if lastErr == nil {
		lastErr = errors.New("all update sources failed")
	}
	logError("check.all_failed", "err", lastErr)
	return nil, "", 0, lastErr
}

type stalledBody struct {
	body    io.ReadCloser
	cancel  context.CancelFunc
	timeout time.Duration
	lastNs  atomic.Int64
	stalled atomic.Bool
	closed  atomic.Bool
	timer   *time.Timer
}

func openDownloadStream(ctx context.Context, rawURL string) (io.ReadCloser, int64, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, 0, err
	}
	if u.Scheme != "https" {
		return nil, 0, fmt.Errorf("update download requires HTTPS: %s", rawURL)
	}
	reqCtx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		cancel()
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent())
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		cancel()
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return nil, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	sb := &stalledBody{body: resp.Body, cancel: cancel, timeout: stallTimeout}
	sb.lastNs.Store(time.Now().UnixNano())
	sb.timer = time.AfterFunc(stallTimeout/4, sb.watch)
	return sb, resp.ContentLength, nil
}

func (s *stalledBody) watch() {
	if s.closed.Load() {
		return
	}
	if time.Since(time.Unix(0, s.lastNs.Load())) >= s.timeout {
		s.stalled.Store(true)
		s.cancel()
		return
	}
	s.timer.Reset(s.timeout / 4)
}

func (s *stalledBody) Read(p []byte) (int, error) {
	n, err := s.body.Read(p)
	if n > 0 {
		s.lastNs.Store(time.Now().UnixNano())
	}
	if err != nil && s.stalled.Load() {
		return n, errors.New("download stream stalled (no data for 20s)")
	}
	return n, err
}

func (s *stalledBody) Close() error {
	s.closed.Store(true)
	if s.timer != nil {
		s.timer.Stop()
	}
	s.cancel()
	return s.body.Close()
}
