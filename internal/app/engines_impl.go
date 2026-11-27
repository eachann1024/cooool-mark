package app

import (
	"cmp"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/mozillazg/go-pinyin"
	"golang.org/x/net/publicsuffix"

	"cooool-mark/internal/model"
)

var _ SearchEngines = (*App)(nil)

// Engines returns the templates shown under every search, ordered by
// SearchRank (>0, ascending) first and then library order.
func (a *App) Engines() []*model.Bookmark {
	if a == nil || a.Store == nil {
		return nil
	}
	return orderedTemplates(a.Store.Lib(), false)
}

// AllTemplates returns every live template in the same order, including
// templates with SearchOff set.
func (a *App) AllTemplates() []*model.Bookmark {
	if a == nil || a.Store == nil {
		return nil
	}
	return orderedTemplates(a.Store.Lib(), true)
}

func orderedTemplates(lib *model.Library, includeOff bool) []*model.Bookmark {
	if lib == nil {
		return nil
	}
	groupOrder := make([]int, len(lib.Groups))
	for i := range groupOrder {
		groupOrder[i] = i
	}
	slices.SortStableFunc(groupOrder, func(i, j int) int {
		return cmp.Compare(lib.Groups[i].Order, lib.Groups[j].Order)
	})
	groupRank := make(map[string]int, len(lib.Groups))
	subRank := make(map[string]map[string]int, len(lib.Groups))
	for rank, gi := range groupOrder {
		g := &lib.Groups[gi]
		groupRank[g.ID] = rank
		subOrder := make([]int, len(g.Subs))
		for i := range subOrder {
			subOrder[i] = i
		}
		slices.SortStableFunc(subOrder, func(i, j int) int {
			return cmp.Compare(g.Subs[i].Order, g.Subs[j].Order)
		})
		sm := make(map[string]int, len(g.Subs))
		for srank, si := range subOrder {
			sm[g.Subs[si].ID] = srank
		}
		subRank[g.ID] = sm
	}

	type libPos struct {
		tier int // 0: valid group+sub, 1: valid group without sub, 2: orphan
		g, s int
		ord  float64
	}
	posOf := func(b *model.Bookmark) libPos {
		gr, ok := groupRank[b.GroupID]
		if !ok {
			return libPos{tier: 2, ord: b.Order}
		}
		if sr, ok := subRank[b.GroupID][b.SubID]; ok {
			return libPos{tier: 0, g: gr, s: sr, ord: b.Order}
		}
		return libPos{tier: 1, g: gr, ord: b.Order}
	}

	var out []*model.Bookmark
	for i := range lib.Bookmarks {
		b := &lib.Bookmarks[i]
		if b.InTrash() || !b.IsTemplate() {
			continue
		}
		if !includeOff && b.SearchOff {
			continue
		}
		out = append(out, b)
	}

	slices.SortStableFunc(out, func(a, b *model.Bookmark) int {
		aRanked := a.SearchRank > 0
		bRanked := b.SearchRank > 0
		if aRanked != bRanked {
			if aRanked {
				return -1
			}
			return 1
		}
		if aRanked && a.SearchRank != b.SearchRank {
			return cmp.Compare(a.SearchRank, b.SearchRank)
		}
		pa, pb := posOf(a), posOf(b)
		if c := cmp.Compare(pa.tier, pb.tier); c != 0 {
			return c
		}
		if c := cmp.Compare(pa.g, pb.g); c != 0 {
			return c
		}
		if c := cmp.Compare(pa.s, pb.s); c != 0 {
			return c
		}
		return cmp.Compare(pa.ord, pb.ord)
	})
	return out
}

// EngineByKeyword matches kw case-insensitively against live templates' Keywords.
func (a *App) EngineByKeyword(kw string) *model.Bookmark {
	if a == nil || a.Store == nil || kw == "" {
		return nil
	}
	for _, b := range a.AllTemplates() {
		for _, k := range b.Keywords {
			if k != "" && strings.EqualFold(k, kw) {
				return b
			}
		}
	}
	return nil
}

// ParseLock reports whether input starts with "<keyword> " for a known keyword.
func (a *App) ParseLock(input string) (b *model.Bookmark, rest string, ok bool) {
	i := strings.IndexByte(input, ' ')
	if i <= 0 {
		return nil, "", false
	}
	b = a.EngineByKeyword(input[:i])
	if b == nil {
		return nil, "", false
	}
	return b, input[i+1:], true
}

// KeywordOwner returns the live bookmark other than exceptID that already uses kw.
func (a *App) KeywordOwner(kw, exceptID string) *model.Bookmark {
	if a == nil || a.Store == nil {
		return nil
	}
	kw = strings.TrimSpace(kw)
	if kw == "" {
		return nil
	}
	lib := a.Store.Lib()
	if lib == nil {
		return nil
	}
	for i := range lib.Bookmarks {
		b := &lib.Bookmarks[i]
		if b.InTrash() || b.ID == exceptID {
			continue
		}
		for _, k := range b.Keywords {
			if k != "" && strings.EqualFold(k, kw) {
				return b
			}
		}
	}
	return nil
}

// SetKeywords stores NormalizeKeywords(kws), bumps UpdatedAt, and saves.
func (a *App) SetKeywords(id string, kws []string) {
	if a == nil || a.Store == nil {
		return
	}
	lib := a.Store.Lib()
	if lib == nil {
		return
	}
	norm := NormalizeKeywords(kws)
	cur := lib.Bookmark(id)
	if cur == nil || cur.InTrash() || slices.Equal(cur.Keywords, norm) {
		return
	}
	a.Store.Update(func(l *model.Library) {
		if b := l.Bookmark(id); b != nil && !b.InTrash() {
			b.Keywords = norm
			b.UpdatedAt = time.Now()
		}
	})
}

// SetEngineVisible sets SearchOff = !on, bumps UpdatedAt, and saves.
func (a *App) SetEngineVisible(id string, on bool) {
	if a == nil || a.Store == nil {
		return
	}
	lib := a.Store.Lib()
	if lib == nil {
		return
	}
	cur := lib.Bookmark(id)
	if cur == nil || cur.InTrash() || cur.SearchOff == !on {
		return
	}
	a.Store.Update(func(l *model.Library) {
		if b := l.Bookmark(id); b != nil && !b.InTrash() {
			b.SearchOff = !on
			b.UpdatedAt = time.Now()
		}
	})
}

// MoveEngine moves a template up (delta < 0) or down in AllTemplates() order
// (hidden ones included) by rewriting SearchRank of every template to 1..n in
// the new order, then saves. Out-of-range moves are no-ops.
func (a *App) MoveEngine(id string, delta int) {
	if a == nil || a.Store == nil || delta == 0 {
		return
	}
	list := a.AllTemplates()
	idx := -1
	for i, b := range list {
		if b.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	target := idx + delta
	if target < 0 || target >= len(list) {
		return
	}
	a.Store.Update(func(l *model.Library) {
		engines := orderedTemplates(l, true)
		cur := -1
		for i, b := range engines {
			if b.ID == id {
				cur = i
				break
			}
		}
		if cur < 0 {
			return
		}
		dst := cur + delta
		if dst < 0 || dst >= len(engines) {
			return
		}
		moved := engines[cur]
		if dst < cur {
			copy(engines[dst+1:cur+1], engines[dst:cur])
		} else {
			copy(engines[cur:dst], engines[cur+1:dst+1])
		}
		engines[dst] = moved

		now := time.Now()
		for i, b := range engines {
			want := float64(i + 1)
			if b.SearchRank != want {
				b.SearchRank = want
				b.UpdatedAt = now
			}
		}
	})
}

var searchParamNames = []string{
	"q", "query", "wd", "word", "keyword", "keywords",
	"search_text", "search", "s", "k", "kw", "text",
}

// DetectSearchParam finds the first non-empty query parameter matching a known
// search parameter name, returning slices that reconstruct rawURL verbatim.
func DetectSearchParam(rawURL string) (p SearchParam, ok bool) {
	if rawURL == "" || strings.Contains(rawURL, "{query}") || strings.ContainsAny(rawURL, " \t\n\r") {
		return SearchParam{}, false
	}
	qMark := strings.IndexByte(rawURL, '?')
	if qMark < 0 {
		return SearchParam{}, false
	}
	hash := strings.IndexByte(rawURL, '#')
	if hash >= 0 && hash < qMark {
		return SearchParam{}, false
	}
	checkURL := rawURL
	if !strings.Contains(checkURL, "://") {
		checkURL = "https://" + checkURL
	}
	u, err := url.Parse(checkURL)
	if err != nil || u.Host == "" {
		return SearchParam{}, false
	}
	qEnd := len(rawURL)
	if hash > qMark {
		qEnd = hash
	}
	type paramSpan struct {
		key      string
		valStart int
		valEnd   int
	}
	var spans []paramSpan
	pos := qMark + 1
	for pos <= qEnd {
		amp := strings.IndexByte(rawURL[pos:qEnd], '&')
		segEnd := qEnd
		if amp >= 0 {
			segEnd = pos + amp
		}
		seg := rawURL[pos:segEnd]
		if eq := strings.IndexByte(seg, '='); eq > 0 {
			valStart := pos + eq + 1
			rawVal := rawURL[valStart:segEnd]
			if rawVal != "" {
				decoded, err := url.QueryUnescape(rawVal)
				if err != nil || strings.TrimSpace(decoded) != "" {
					spans = append(spans, paramSpan{
						key:      strings.ToLower(seg[:eq]),
						valStart: valStart,
						valEnd:   segEnd,
					})
				}
			}
		}
		if amp < 0 {
			break
		}
		pos = segEnd + 1
	}
	for _, name := range searchParamNames {
		for _, sp := range spans {
			if sp.key == name {
				return SearchParam{
					Pre:   rawURL[:sp.valStart],
					Value: rawURL[sp.valStart:sp.valEnd],
					Post:  rawURL[sp.valEnd:],
				}, true
			}
		}
	}
	return SearchParam{}, false
}

// NormalizeKeywords trims, lower-cases, drops empty / space-containing words
// and duplicates, keeping order.
func NormalizeKeywords(kws []string) []string {
	if len(kws) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(kws))
	var out []string
	for _, kw := range kws {
		s := strings.ToLower(strings.TrimSpace(kw))
		if s == "" || strings.ContainsFunc(s, unicode.IsSpace) {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

var knownHostInitials = map[string]string{
	"google":        "g",
	"baidu":         "bd",
	"bing":          "b",
	"duckduckgo":    "ddg",
	"yahoo":         "y",
	"github":        "gh",
	"gitlab":        "gl",
	"stackoverflow": "so",
	"wikipedia":     "wk",
	"youtube":       "yt",
	"bilibili":      "bl",
	"douban":        "db",
	"zhihu":         "zh",
	"weibo":         "wb",
	"xiaohongshu":   "xhs",
	"taobao":        "tb",
	"tmall":         "tm",
	"jd":            "jd",
	"amazon":        "az",
	"reddit":        "rd",
	"twitter":       "tw",
	"npmjs":         "npm",
	"pypi":          "py",
	"crates":        "rs",
	"docker":        "dk",
	"sspai":         "ssp",
	"v2ex":          "v2",
	"juejin":        "jj",
	"csdn":          "csdn",
}

// SuggestKeywords proposes up to 3 normalized trigger words from title and rawURL:
// a 2-character CJK title prefix, the domain stem, and its initials when known.
func SuggestKeywords(title, rawURL string) []string {
	var candidates []string
	trimmed := strings.TrimSpace(title)
	rs := []rune(trimmed)
	if len(rs) >= 2 && isCJK(rs[0]) && isCJK(rs[1]) {
		candidates = append(candidates, string(rs[:2]))
	}
	stem := domainStem(rawURL)
	if stem != "" {
		candidates = append(candidates, stem)
	}
	if init := suggestInitials(trimmed, rs, stem); init != "" {
		candidates = append(candidates, init)
	}
	out := NormalizeKeywords(candidates)
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

func domainStem(rawURL string) string {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	if eTLD1, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil && eTLD1 != "" {
		if dot := strings.IndexByte(eTLD1, '.'); dot > 0 {
			return eTLD1[:dot]
		}
		return eTLD1
	}
	if dot := strings.IndexByte(host, '.'); dot > 0 {
		return host[:dot]
	}
	return host
}

var suggestPinyinArgs = pinyin.Args{Style: pinyin.Normal}

func suggestInitials(title string, rs []rune, stem string) string {
	if init, ok := knownHostInitials[stem]; ok {
		return init
	}
	if len(rs) >= 2 && unicode.Is(unicode.Han, rs[0]) && unicode.Is(unicode.Han, rs[1]) {
		var b strings.Builder
		for _, r := range rs[:2] {
			pys := pinyin.SinglePinyin(r, suggestPinyinArgs)
			if len(pys) == 0 || pys[0] == "" {
				return ""
			}
			b.WriteByte(pys[0][0])
		}
		return b.String()
	}
	return asciiTitleInitials(title)
}

func asciiTitleInitials(title string) string {
	var b strings.Builder
	var prev byte
	inWord := false
	for i := 0; i < len(title); i++ {
		c := title[i]
		if c >= 'A' && c <= 'Z' {
			if !inWord || (prev >= 'a' && prev <= 'z') {
				b.WriteByte(c + ('a' - 'A'))
			}
			inWord = true
			prev = c
		} else if c >= 'a' && c <= 'z' || (c >= '0' && c <= '9') {
			if !inWord {
				b.WriteByte(c)
			}
			inWord = true
			prev = c
		} else {
			inWord = false
			prev = 0
		}
	}
	s := b.String()
	if len(s) < 2 || len(s) > 4 {
		return ""
	}
	return s
}
