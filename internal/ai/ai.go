// Package ai polishes bookmarks using local llama.cpp or OpenAI-compatible endpoints.
package ai

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/netproxy"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
)

const defaultPolishTimeout = 60 * time.Second

type engine struct {
	a              *app.App
	ctx            context.Context
	cancel         context.CancelFunc
	localCtx       context.Context
	localCancel    context.CancelFunc
	mu             sync.Mutex
	settings       model.Settings
	listeners      []func()
	onDownloadDone []func(string)
	downloads      sync.Mutex
	dl             *downloadState
	llama          *llamaManager
}

// New returns the AI service.
func New(a *app.App) app.AI {
	s := model.DefaultSettings()
	if a != nil {
		s = a.Settings
	}
	ctx, cancel := context.WithCancel(context.Background())
	localCtx, localCancel := context.WithCancel(ctx)
	e := &engine{
		a:           a,
		ctx:         ctx,
		cancel:      cancel,
		localCtx:    localCtx,
		localCancel: localCancel,
		settings:    s,
	}
	e.llama = newLlamaManager(e)
	return e
}

func (e *engine) OnChange(fn func()) {
	if fn == nil {
		return
	}
	e.mu.Lock()
	e.listeners = append(e.listeners, fn)
	e.mu.Unlock()
}

func (e *engine) OnDownloadComplete(fn func(modelName string)) {
	if fn == nil {
		return
	}
	e.mu.Lock()
	e.onDownloadDone = append(e.onDownloadDone, fn)
	e.mu.Unlock()
}

func (e *engine) notifyDownloadComplete(modelName string) {
	e.mu.Lock()
	fns := append(([]func(string))(nil), e.onDownloadDone...)
	e.mu.Unlock()
	for _, fn := range fns {
		fn(modelName)
	}
}

func (e *engine) Close() {
	if e.cancel != nil {
		e.cancel()
	}
	e.Pause()
	if e.llama != nil {
		e.llama.close()
	}
}

func (e *engine) notify() {
	e.mu.Lock()
	fns := append(([]func())(nil), e.listeners...)
	e.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func (e *engine) Configure(s model.Settings) {
	e.mu.Lock()
	oldLocal := e.settings.LocalModel
	oldEngine := e.settings.AIEngine
	e.settings = s
	var cancelLocal context.CancelFunc
	resetLocal := oldLocal != s.LocalModel || (oldEngine == model.AILocal && s.AIEngine != model.AILocal)
	if resetLocal {
		cancelLocal = e.localCancel
		e.localCtx, e.localCancel = context.WithCancel(e.ctx)
	}
	e.mu.Unlock()

	if cancelLocal != nil {
		cancelLocal()
	}
	if resetLocal {
		e.llama.stop()
	}
	e.notify()
}

func (e *engine) currentSettingsAndLocalCtx() (model.Settings, context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.settings, e.localCtx
}

func (e *engine) currentSettings() model.Settings {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.settings
}

func (e *engine) Status() app.AIStatus {
	s := e.currentSettings()

	switch s.AIEngine {
	case model.AIOff, "":
		return app.AIStatus{Engine: model.AIOff, EngineName: locales.AiStatusOff()}

	case model.AILocal:
		spec := resolveModelSpec(s.LocalModel)
		return e.localStatusFor(spec)

	case model.AIOpenAI:
		name := locales.AiEngineOpenAI()
		if m := strings.TrimSpace(s.OpenAIModel); m != "" {
			name = locales.AiOpenAINamed(m)
		}
		rawURL := strings.TrimSpace(s.OpenAIURL)
		rawModel := strings.TrimSpace(s.OpenAIModel)
		ready := rawURL != "" && rawModel != ""
		errMsg := ""
		switch {
		case !ready:
			errMsg = locales.AiErrConfigureOpenAI()
		case !isValidHTTPURL(rawURL):
			ready = false
			errMsg = locales.AiErrURLScheme()
		}
		return app.AIStatus{
			Engine:     model.AIOpenAI,
			EngineName: name,
			Ready:      ready,
			Err:        errMsg,
		}

	default:
		return app.AIStatus{Engine: model.AIOff, EngineName: locales.AiStatusOff()}
	}
}

func (e *engine) Polish(ctx context.Context, in app.PolishInput) (app.Polish, error) {
	if e.ctx.Err() != nil {
		return app.Polish{}, e.ctx.Err()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		ctx, cancel = context.WithTimeout(ctx, defaultPolishTimeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()
	stopCancel := context.AfterFunc(e.ctx, cancel)
	defer stopCancel()

	s, lctx := e.currentSettingsAndLocalCtx()
	switch s.AIEngine {
	case model.AIOff, "":
		return app.Polish{}, errors.New(locales.AiErrNotEnabled())

	case model.AILocal:
		if lctx != nil {
			stopLocal := context.AfterFunc(lctx, cancel)
			defer stopLocal()
		}
		spec := resolveModelSpec(s.LocalModel)
		path, ok := modelFileIfReady(spec)
		if !ok {
			return app.Polish{}, errors.New(locales.AiErrLocalNotDownloaded())
		}
		return e.llama.polish(ctx, s, path, in)

	case model.AIOpenAI:
		return polishOpenAI(ctx, s, in)

	default:
		return app.Polish{}, errors.ErrUnsupported
	}
}

func userDataDir() (string, error) {
	return mygo.App.Path(mygo.PathUserData)
}

func newHTTPClient(_ model.Settings, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: netproxy.Transport().Clone(),
		Timeout:   timeout,
	}
}

func isValidHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Host != ""
}
