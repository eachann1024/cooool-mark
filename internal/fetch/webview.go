package fetch

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

type webMeta struct {
	FinalURL    string
	Title       string
	Description string
	SiteName    string
	Icons       []iconCandidate
}

func fetchWithWebView(ctx context.Context, targetURL string) (webMeta, error) {
	if !mygo.App.IsReady() {
		return webMeta{}, errors.New("webview unavailable before app ready")
	}
	cleanURL, err := normalizeFetchURL(targetURL)
	if err != nil {
		return webMeta{}, err
	}
	targetURL = cleanURL

	wctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	w := mygo.NewWindow(mygo.WindowOptions{
		Hidden:      true,
		SkipTaskbar: true,
		Width:       800,
		Height:      600,
		Page: mygo.PageOptions{
			Session:   mygo.NewEphemeralSession(),
			UserAgent: defaultUserAgent,
			Untrusted: true,
		},
	})
	defer w.Close()

	// Never allow a background metadata fetch page to open popups or hand off
	// URLs to the system browser.
	w.Page().SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions {
		return nil
	})

	var (
		doneOnce sync.Once
		failOnce sync.Once
		navMu    sync.Mutex
		navCount int
		loaded   = make(chan struct{})
		failed   = make(chan error, 1)
	)
	fail := func(err error) {
		failOnce.Do(func() {
			failed <- err
		})
	}

	offNav := w.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
		if e == nil {
			return
		}
		if _, err := normalizeFetchURL(e.URL); err != nil {
			e.PreventDefault()
			fail(errRedirectBlocked)
			return
		}
		navMu.Lock()
		navCount++
		count := navCount
		navMu.Unlock()
		if count > 10 {
			e.PreventDefault()
			fail(errRedirectBlocked)
		}
	})
	defer offNav()

	offDL := w.Page().OnWillDownload(func(e *mygo.DownloadEvent) {
		if e != nil {
			e.PreventDefault()
		}
		fail(errors.New("webview download blocked"))
	})
	defer offDL()

	offLoad := w.Page().OnDidFinishLoad(func() {
		doneOnce.Do(func() { close(loaded) })
	})
	defer offLoad()

	offFail := w.Page().OnDidFailLoad(func(le *mygo.LoadError) {
		if le != nil {
			fail(errors.New(le.Description))
		} else {
			fail(errors.New("webview load failed"))
		}
	})
	defer offFail()

	if err := w.Page().LoadURL(targetURL); err != nil {
		return webMeta{}, err
	}

	select {
	case <-loaded:
	case err := <-failed:
		return webMeta{}, err
	case <-wctx.Done():
		return webMeta{}, wctx.Err()
	}

	// Give SPA frameworks a brief moment to update document.title after load.
	var meta webMeta
	for i := 0; i < 6; i++ {
		if i > 0 {
			select {
			case <-time.After(250 * time.Millisecond):
			case <-wctx.Done():
				return meta, wctx.Err()
			}
		}
		m, err := readWebViewDOM(wctx, w.Page())
		if err == nil {
			meta = m
			if meta.Title != "" {
				break
			}
		}
	}
	return meta, nil
}

func readWebViewDOM(ctx context.Context, p *mygo.Page) (webMeta, error) {
	const script = `
		const pick = (sel, attr) => {
			const el = document.querySelector(sel);
			return el ? (el.getAttribute(attr) || '').trim() : '';
		};
		const links = [];
		for (const el of document.querySelectorAll('link[rel][href]')) {
			links.push({
				rel: (el.getAttribute('rel') || '').toLowerCase(),
				href: el.href || el.getAttribute('href') || '',
				type: (el.getAttribute('type') || '').toLowerCase(),
				sizes: (el.getAttribute('sizes') || '').toLowerCase(),
			});
		}
		return {
			url: location.href || '',
			title: (document.title || '').trim(),
			desc: pick('meta[name="description"]', 'content') || pick('meta[property="og:description"]', 'content'),
			site: pick('meta[property="og:site_name"]', 'content') || pick('meta[name="application-name"]', 'content'),
			links: links,
		};
	`
	val, err := p.EvalContext(ctx, script)
	if err != nil {
		return webMeta{}, err
	}
	m, ok := val.(map[string]any)
	if !ok {
		return webMeta{}, nil
	}
	finalURL := ""
	if rawFinal := strVal(m["url"]); rawFinal != "" {
		if norm, err := normalizeFetchURL(rawFinal); err == nil {
			finalURL = norm
		}
	}
	baseURL, _ := url.Parse(finalURL)
	out := webMeta{
		FinalURL:    finalURL,
		Title:       collapseSpaces(strVal(m["title"])),
		Description: collapseSpaces(strVal(m["desc"])),
		SiteName:    collapseSpaces(strVal(m["site"])),
	}
	if rawLinks, ok := m["links"].([]any); ok {
		for _, item := range rawLinks {
			lm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			rel := strVal(lm["rel"])
			href := resolveURL(baseURL, strVal(lm["href"]))
			typ := strVal(lm["type"])
			sizes := strVal(lm["sizes"])
			if cand, ok := classifyLinkIcon(rel, href, typ, sizes); ok {
				out.Icons = append(out.Icons, cand)
			}
		}
	}
	return out, nil
}

func strVal(v any) string {
	s, _ := v.(string)
	return s
}
