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
	"cooool-mark/internal/model"
)

const defaultPolishTimeout = 60 * time.Second

type engine struct {
	a         *app.App
	mu        sync.Mutex
	settings  model.Settings
	listeners []func()
	downloads sync.Mutex
	dl        *downloadState
	llama     *llamaManager
}

// New returns the AI service.
func New(a *app.App) app.AI {
	s := model.DefaultSettings()
	if a != nil {
		s = a.Settings
	}
	e := &engine{
		a:        a,
		settings: s,
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
	e.mu.Unlock()

	if oldLocal != s.LocalModel || (oldEngine == model.AILocal && s.AIEngine != model.AILocal) {
		e.llama.stop()
	}
	e.notify()
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
		return app.AIStatus{Engine: model.AIOff, EngineName: "未启用"}

	case model.AILocal:
		spec := resolveModelSpec(s.LocalModel)
		return e.localStatusFor(spec)

	case model.AIOpenAI:
		name := "OpenAI 兼容"
		if m := strings.TrimSpace(s.OpenAIModel); m != "" {
			name = "OpenAI（" + m + "）"
		}
		rawURL := strings.TrimSpace(s.OpenAIURL)
		rawModel := strings.TrimSpace(s.OpenAIModel)
		ready := rawURL != "" && rawModel != ""
		errMsg := ""
		switch {
		case !ready:
			errMsg = "请配置 OpenAI API 地址和模型名称"
		case !isValidHTTPURL(rawURL):
			ready = false
			errMsg = "API 地址需以 http:// 或 https:// 开头"
		}
		return app.AIStatus{
			Engine:     model.AIOpenAI,
			EngineName: name,
			Ready:      ready,
			Err:        errMsg,
		}

	default:
		return app.AIStatus{Engine: model.AIOff, EngineName: "未启用"}
	}
}

func (e *engine) Polish(ctx context.Context, in app.PolishInput) (app.Polish, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultPolishTimeout)
		defer cancel()
	}

	s := e.currentSettings()
	switch s.AIEngine {
	case model.AIOff, "":
		return app.Polish{}, errors.New("AI 润色未启用")

	case model.AILocal:
		spec := resolveModelSpec(s.LocalModel)
		path, ok := modelFileIfReady(spec)
		if !ok {
			return app.Polish{}, errors.New("本地模型尚未下载")
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
