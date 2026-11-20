package fetch

import (
	"context"
	"errors"
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
		},
	})
	defer w.Close()

	var (
		doneOnce sync.Once
		failOnce sync.Once
		loaded   = make(chan struct{})
		failed   = make(chan error, 1)
	)

	offLoad := w.Page().OnDidFinishLoad(func() {
		doneOnce.Do(func() { close(loaded) })
	})
	defer offLoad()

	offFail := w.Page().OnDidFailLoad(func(le *mygo.LoadError) {
		failOnce.Do(func() {
			if le != nil {
				failed <- errors.New(le.Description)
			} else {
				failed <- errors.New("webview load failed")
			}
		})
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
	out := webMeta{
		FinalURL:    strVal(m["url"]),
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
			href := strVal(lm["href"])
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
