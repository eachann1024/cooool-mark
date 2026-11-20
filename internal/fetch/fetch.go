// Package fetch reads page titles and icons, and the active browser tab.
package fetch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/netproxy"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
)

const (
	defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	maxHTMLBytes     = 2 << 20
	maxIconBytes     = 1 << 20
	maxManifestBytes = 256 << 10
)

var errRedirectBlocked = errors.New("redirect blocked")

type fetcher struct{ a *app.App }

// New returns the Fetcher.
func New(a *app.App) app.Fetcher { return &fetcher{a: a} }

func (f *fetcher) Fetch(ctx context.Context, rawURL string) (app.PageMeta, error) {
	targetURL, err := normalizeFetchURL(rawURL)
	if err != nil {
		return app.PageMeta{URL: rawURL}, err
	}

	client, err := newHTTPClient()
	if err != nil {
		return app.PageMeta{URL: targetURL}, err
	}

	meta := app.PageMeta{URL: targetURL}
	var icons []iconCandidate
	var manifestURLs []string

	finalURL, parsed, httpErr := f.fetchHTML(ctx, client, targetURL)
	if finalURL != "" {
		meta.URL = finalURL
	}
	if httpErr == nil {
		meta.Title = parsed.title
		meta.Description = parsed.description
		meta.SiteName = parsed.siteName
		icons = append(icons, parsed.icons...)
		manifestURLs = parsed.manifests
	}

	canUseWebView := !errors.Is(httpErr, errRedirectBlocked) && !parsed.nonHTML && (f.a == nil || f.a.UseWebView())
	if meta.Title == "" && canUseWebView {
		if wm, werr := fetchWithWebView(ctx, meta.URL); werr == nil {
			if wm.FinalURL != "" {
				meta.URL = wm.FinalURL
			}
			if wm.Title != "" {
				meta.Title = wm.Title
			}
			if meta.Description == "" && wm.Description != "" {
				meta.Description = wm.Description
			}
			if meta.SiteName == "" && wm.SiteName != "" {
				meta.SiteName = wm.SiteName
			}
			if len(wm.Icons) > 0 {
				icons = append(icons, wm.Icons...)
			}
		} else if httpErr != nil {
			return meta, httpErr
		}
	} else if httpErr != nil && meta.Title == "" {
		return meta, httpErr
	}

	meta.Title = cleanTitle(meta.Title, meta.SiteName, meta.URL)

	// Try top-priority icons (apple-touch-icon, SVG) first; if none work,
	// expand manifest icons and finally fall back to /favicon.ico.
	sortIconCandidates(icons)
	for _, cand := range icons {
		if cand.priority > prioSVG {
			break
		}
		if data, ext, ok := downloadIcon(ctx, client, cand.url); ok {
			meta.Icon, meta.IconExt = data, ext
			return meta, nil
		}
	}

	for _, murl := range manifestURLs {
		icons = append(icons, fetchManifestIcons(ctx, client, murl)...)
	}
	if u, err := url.Parse(meta.URL); err == nil && u.Scheme != "" && u.Host != "" {
		icons = append(icons, iconCandidate{
			url:      u.Scheme + "://" + u.Host + "/favicon.ico",
			priority: prioFallbackFavicon,
		})
	}
	sortIconCandidates(icons)

	seen := map[string]bool{}
	for _, cand := range icons {
		if seen[cand.url] {
			continue
		}
		seen[cand.url] = true
		if data, ext, ok := downloadIcon(ctx, client, cand.url); ok {
			meta.Icon, meta.IconExt = data, ext
			break
		}
	}
	return meta, nil
}

func normalizeFetchURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("empty url")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("missing host")
	}
	return u.String(), nil
}

func newHTTPClient() (*http.Client, error) {
	tr := netproxy.Transport().Clone()
	tr.DialContext = (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 8 * time.Second
	tr.ResponseHeaderTimeout = 10 * time.Second

	return &http.Client{
		Transport: tr,
		Timeout:   15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("%w: stopped after 10 redirects", errRedirectBlocked)
			}
			scheme := strings.ToLower(req.URL.Scheme)
			if scheme != "http" && scheme != "https" {
				return fmt.Errorf("%w: unsupported redirect scheme %q", errRedirectBlocked, req.URL.Scheme)
			}
			if req.URL.Host == "" {
				return fmt.Errorf("%w: redirect missing host", errRedirectBlocked)
			}
			return nil
		},
	}, nil
}

type parsedHTML struct {
	title       string
	description string
	siteName    string
	icons       []iconCandidate
	manifests   []string
	nonHTML     bool
}

func (f *fetcher) fetchHTML(ctx context.Context, client *http.Client, targetURL string) (string, parsedHTML, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return targetURL, parsedHTML{}, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if locales.IsChinese() {
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	} else {
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	}

	resp, err := client.Do(req)
	if err != nil {
		return targetURL, parsedHTML{}, err
	}
	defer resp.Body.Close()

	finalURL := targetURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	disp := strings.ToLower(resp.Header.Get("Content-Disposition"))
	ct := resp.Header.Get("Content-Type")
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	nonHTML := strings.Contains(disp, "attachment") ||
		(mediaType != "" && mediaType != "text/html" && mediaType != "application/xhtml+xml")

	if resp.StatusCode >= 400 {
		return finalURL, parsedHTML{nonHTML: nonHTML}, fmt.Errorf("http %d", resp.StatusCode)
	}
	if nonHTML {
		return finalURL, parsedHTML{nonHTML: true}, nil
	}
	limited := io.LimitReader(resp.Body, maxHTMLBytes)
	reader, err := charset.NewReader(limited, ct)
	if err != nil {
		reader = limited
	}
	doc, err := html.Parse(reader)
	if err != nil {
		return finalURL, parsedHTML{}, err
	}

	baseURL, _ := url.Parse(finalURL)
	return finalURL, extractHTMLMeta(doc, baseURL), nil
}

func extractHTMLMeta(doc *html.Node, base *url.URL) parsedHTML {
	var out parsedHTML
	var ogTitle, twTitle, htmlTitle string
	var metaDesc, ogDesc, twDesc string
	var ogSite, appName string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if htmlTitle == "" && n.FirstChild != nil {
					var sb strings.Builder
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.TextNode {
							sb.WriteString(c.Data)
						}
					}
					htmlTitle = collapseSpaces(sb.String())
				}
			case "meta":
				name := strings.ToLower(strings.TrimSpace(attr(n, "name")))
				prop := strings.ToLower(strings.TrimSpace(attr(n, "property")))
				content := collapseSpaces(attr(n, "content"))
				if content != "" {
					switch {
					case prop == "og:title" && ogTitle == "":
						ogTitle = content
					case name == "twitter:title" && twTitle == "":
						twTitle = content
					case name == "description" && metaDesc == "":
						metaDesc = content
					case prop == "og:description" && ogDesc == "":
						ogDesc = content
					case name == "twitter:description" && twDesc == "":
						twDesc = content
					case prop == "og:site_name" && ogSite == "":
						ogSite = content
					case name == "application-name" && appName == "":
						appName = content
					}
				}
			case "link":
				rel := strings.ToLower(strings.TrimSpace(attr(n, "rel")))
				href := strings.TrimSpace(attr(n, "href"))
				typ := strings.ToLower(strings.TrimSpace(attr(n, "type")))
				sizes := strings.ToLower(strings.TrimSpace(attr(n, "sizes")))
				if href != "" {
					abs := resolveURL(base, href)
					if abs != "" {
						if hasRelToken(rel, "manifest") {
							out.manifests = append(out.manifests, abs)
						} else if cand, ok := classifyLinkIcon(rel, abs, typ, sizes); ok {
							out.icons = append(out.icons, cand)
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	out.title = firstNonEmpty(htmlTitle, ogTitle, twTitle)
	out.description = firstNonEmpty(ogDesc, metaDesc, twDesc)
	out.siteName = firstNonEmpty(ogSite, appName)
	return out
}

const (
	prioAppleTouch      = 1
	prioSVG             = 2
	prioManifest        = 3
	prioIcon            = 4
	prioFallbackFavicon = 5
)

type iconCandidate struct {
	url      string
	priority int
	size     int
	order    int
}

func classifyLinkIcon(rel, absURL, typ, sizes string) (iconCandidate, bool) {
	if absURL == "" {
		return iconCandidate{}, false
	}
	isApple := hasRelToken(rel, "apple-touch-icon") || hasRelToken(rel, "apple-touch-icon-precomposed")
	isIcon := hasRelToken(rel, "icon") || hasRelToken(rel, "shortcut") || hasRelToken(rel, "mask-icon")
	if !isApple && !isIcon {
		return iconCandidate{}, false
	}

	sz := parseIconSize(sizes)
	switch {
	case isApple:
		return iconCandidate{url: absURL, priority: prioAppleTouch, size: sz}, true
	case typ == "image/svg+xml" || strings.HasSuffix(strings.ToLower(urlPathOnly(absURL)), ".svg"):
		return iconCandidate{url: absURL, priority: prioSVG, size: sz}, true
	default:
		return iconCandidate{url: absURL, priority: prioIcon, size: sz}, true
	}
}

func sortIconCandidates(cands []iconCandidate) {
	for i := range cands {
		cands[i].order = i
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].priority != cands[j].priority {
			return cands[i].priority < cands[j].priority
		}
		if cands[i].size != cands[j].size {
			return cands[i].size > cands[j].size
		}
		return cands[i].order < cands[j].order
	})
}

func fetchManifestIcons(ctx context.Context, client *http.Client, manifestURL string) []iconCandidate {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		return nil
	}
	var manifest struct {
		Icons []struct {
			Src   string `json:"src"`
			Sizes string `json:"sizes"`
			Type  string `json:"type"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	base, _ := url.Parse(manifestURL)
	var out []iconCandidate
	for _, ic := range manifest.Icons {
		abs := resolveURL(base, strings.TrimSpace(ic.Src))
		if abs == "" {
			continue
		}
		prio := prioManifest
		if strings.Contains(strings.ToLower(ic.Type), "svg") || strings.HasSuffix(strings.ToLower(urlPathOnly(abs)), ".svg") {
			prio = prioSVG
		}
		out = append(out, iconCandidate{
			url:      abs,
			priority: prio,
			size:     parseIconSize(ic.Sizes),
		})
	}
	return out
}

func downloadIcon(ctx context.Context, client *http.Client, iconURL string) ([]byte, string, bool) {
	if strings.HasPrefix(iconURL, "data:") {
		return decodeDataURI(iconURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iconURL, nil)
	if err != nil {
		return nil, "", false
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIconBytes))
	if err != nil || len(data) == 0 {
		return nil, "", false
	}
	return normalizeIconData(data, resp.Header.Get("Content-Type"), iconURL)
}

func decodeDataURI(raw string) ([]byte, string, bool) {
	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return nil, "", false
	}
	meta := raw[len("data:"):comma]
	payload := raw[comma+1:]
	isB64 := strings.HasSuffix(meta, ";base64")
	mime := strings.TrimSuffix(meta, ";base64")

	var data []byte
	var err error
	if isB64 {
		data, err = base64.StdEncoding.DecodeString(payload)
	} else {
		var u string
		u, err = url.PathUnescape(payload)
		data = []byte(u)
	}
	if err != nil || len(data) == 0 {
		return nil, "", false
	}
	return normalizeIconData(data, mime, "")
}

func normalizeIconData(data []byte, contentType, rawURL string) ([]byte, string, bool) {
	ext := detectImageExt(data, contentType, rawURL)
	if ext == "" {
		return nil, "", false
	}
	if ext == ".ico" {
		if pngData, ok := icoToPNG(data); ok {
			return pngData, ".png", true
		}
	}
	return data, ext, true
}

func detectImageExt(data []byte, contentType, rawURL string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	trimmed := bytes.TrimSpace(data)

	switch {
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return ".png"
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return ".jpg"
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		return ".gif"
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return ".webp"
	case bytes.HasPrefix(data, []byte{0, 0, 1, 0}) || bytes.HasPrefix(data, []byte{0, 0, 2, 0}):
		return ".ico"
	case bytes.HasPrefix(trimmed, []byte("<svg")) || (bytes.HasPrefix(trimmed, []byte("<?xml")) && bytes.Contains(trimmed, []byte("<svg"))):
		return ".svg"
	}

	switch ct {
	case "image/png":
		return ".png"
	case "image/svg+xml":
		return ".svg"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}

	p := strings.ToLower(urlPathOnly(rawURL))
	for _, ext := range []string{".png", ".svg", ".ico", ".jpg", ".jpeg", ".webp", ".gif"} {
		if strings.HasSuffix(p, ext) {
			if ext == ".jpeg" {
				return ".jpg"
			}
			return ext
		}
	}
	return ""
}

// cleanTitle trims redundant trailing site suffixes like " - GitHub" when
// the suffix matches siteName or the host's brand label.
func cleanTitle(title, siteName, rawURL string) string {
	title = collapseSpaces(title)
	if title == "" {
		return ""
	}
	var matchers []string
	if sn := collapseSpaces(siteName); sn != "" {
		matchers = append(matchers, strings.ToLower(sn))
	}
	if u, err := url.Parse(rawURL); err == nil && u.Hostname() != "" {
		host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
		matchers = append(matchers, host)
		if parts := strings.Split(host, "."); len(parts) >= 2 {
			matchers = append(matchers, parts[0])
		}
	}

	bestIdx := -1
	bestSepLen := 0
	for _, sep := range []string{" - ", " | ", " · ", " — ", " – ", " :: "} {
		if idx := strings.LastIndex(title, sep); idx > bestIdx {
			bestIdx = idx
			bestSepLen = len(sep)
		}
	}
	if bestIdx <= 0 {
		return title
	}
	prefix := strings.TrimSpace(title[:bestIdx])
	suffix := strings.ToLower(strings.TrimSpace(title[bestIdx+bestSepLen:]))
	if len([]rune(prefix)) < 2 || suffix == "" {
		return title
	}
	for _, m := range matchers {
		if m != "" && suffix == m {
			return prefix
		}
	}
	return title
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func hasRelToken(rel, token string) bool {
	for _, f := range strings.Fields(rel) {
		if strings.EqualFold(f, token) {
			return true
		}
	}
	return false
}

func resolveURL(base *url.URL, ref string) string {
	if strings.HasPrefix(ref, "data:") {
		return ref
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.String()
}

func urlPathOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

func parseIconSize(sizes string) int {
	best := 0
	for _, tok := range strings.Fields(sizes) {
		if tok == "any" {
			return 1024
		}
		parts := strings.SplitN(strings.ToLower(tok), "x", 2)
		if len(parts) == 2 {
			w, err1 := strconv.Atoi(parts[0])
			h, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil && w*h > best {
				best = w * h
			}
		}
	}
	return best
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
