package ai

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
)

const (
	llamaIdleTimeout    = 5 * time.Minute
	llamaFallbackTag    = "b12549"
	llamaReleasesAPIURL = "https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=10"
)

type llamaInstance struct {
	cmd        *exec.Cmd
	port       int
	modelPath  string
	doneCh     chan struct{}
	readyCh    chan struct{}
	readyErr   error
	activeReqs int
	lastUsed   time.Time
	idleTimer  *time.Timer
}

func (inst *llamaInstance) isAlive() bool {
	if inst == nil || inst.cmd == nil || inst.doneCh == nil {
		return false
	}
	select {
	case <-inst.doneCh:
		return false
	default:
		return true
	}
}

type llamaManager struct {
	e      *engine
	mu     sync.Mutex
	binMu  sync.Mutex
	closed bool
	cur    *llamaInstance
}

func newLlamaManager(e *engine) *llamaManager {
	return &llamaManager{e: e}
}

func (m *llamaManager) polish(ctx context.Context, s model.Settings, modelPath string, in app.PolishInput) (app.Polish, error) {
	inst, err := m.acquire(ctx, s, modelPath)
	if err != nil {
		return app.Polish{}, err
	}
	defer m.release(inst)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", inst.port)
	extra := map[string]any{
		"response_format": polishJSONSchema(len(in.Choices) > 0),
		"chat_template_kwargs": map[string]any{
			"enable_thinking": false,
		},
	}
	out, err := polishOpenAICompat(ctx, s, baseURL, "", filepath.Base(modelPath), in, extra)
	if err != nil {
		return app.Polish{}, err
	}
	// Small local models (0.8B/2B) hallucinate descriptions when given only a
	// URL and bare site/brand name without webpage text.
	if !app.HasGroundedNoteContext(in.URL, in.Title, in.Description, in.Excerpt) {
		out.Note = ""
		return out, nil
	}
	// With real page text, an empty note means the model echoed the input or
	// answered in the wrong language; sample once more before giving up.
	if out.Note == "" {
		extra["temperature"] = 0.6
		if again, err := polishOpenAICompat(ctx, s, baseURL, "", filepath.Base(modelPath), in, extra); err == nil && again.Note != "" {
			if again.Title == "" {
				again.Title = out.Title
			}
			out = again
		}
	}
	return out, nil
}

func (m *llamaManager) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *llamaManager) acquire(ctx context.Context, s model.Settings, modelPath string) (*llamaInstance, error) {
	if m.isClosed() {
		return nil, context.Canceled
	}
	binPath, err := m.ensureBinary(ctx, s)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, context.Canceled
	}
	if ctx.Err() != nil {
		m.mu.Unlock()
		return nil, ctx.Err()
	}
	// Re-verify settings after potentially slow binary download so disabling
	// local AI or switching local models in Settings never spawns a stale server.
	if m.e != nil {
		cur := m.e.currentSettings()
		if cur.AIEngine != model.AILocal {
			m.mu.Unlock()
			return nil, context.Canceled
		}
		if curPath, ok := modelFileIfReady(resolveModelSpec(cur.LocalModel)); !ok || curPath != modelPath {
			m.mu.Unlock()
			return nil, context.Canceled
		}
	}

	var inst *llamaInstance
	if m.cur != nil && m.cur.modelPath == modelPath && m.cur.isAlive() {
		inst = m.cur
		if inst.idleTimer != nil {
			inst.idleTimer.Stop()
			inst.idleTimer = nil
		}
		inst.activeReqs++
		inst.lastUsed = time.Now()
		m.mu.Unlock()
	} else {
		m.stopInstanceLocked(m.cur)

		port, portErr := pickFreePort()
		if portErr != nil {
			m.mu.Unlock()
			return nil, portErr
		}

		cmd := exec.Command(binPath,
			"-m", modelPath,
			"--host", "127.0.0.1",
			"--port", strconv.Itoa(port),
			"-c", "4096",
		)
		cmd.Dir = filepath.Dir(binPath)
		hideWindow(cmd)
		var errBuf limitedBuffer
		cmd.Stderr = &errBuf

		if startErr := cmd.Start(); startErr != nil {
			m.mu.Unlock()
			return nil, errors.New(locales.AiErrStartLlama(startErr.Error()))
		}

		inst = &llamaInstance{
			cmd:        cmd,
			port:       port,
			modelPath:  modelPath,
			doneCh:     make(chan struct{}),
			readyCh:    make(chan struct{}),
			activeReqs: 1,
			lastUsed:   time.Now(),
		}
		m.cur = inst

		go func() {
			_ = cmd.Wait()
			close(inst.doneCh)
		}()

		parentCtx := context.Background()
		if m.e != nil && m.e.ctx != nil {
			parentCtx = m.e.ctx
		}
		go func() {
			startCtx, startCancel := context.WithTimeout(parentCtx, 45*time.Second)
			defer startCancel()
			waitErr := waitLlamaHealth(startCtx, inst.port, inst.doneCh)
			if waitErr != nil {
				detail := strings.TrimSpace(errBuf.String())
				if detail != "" {
					waitErr = fmt.Errorf("%w: %s", waitErr, detail)
				}
				inst.readyErr = waitErr
				m.mu.Lock()
				m.stopInstanceLocked(inst)
				m.mu.Unlock()
			}
			close(inst.readyCh)
		}()
		m.mu.Unlock()
	}

	select {
	case <-inst.readyCh:
		if inst.readyErr != nil {
			m.release(inst)
			return nil, inst.readyErr
		}
		return inst, nil
	case <-ctx.Done():
		m.release(inst)
		return nil, ctx.Err()
	}
}

func (m *llamaManager) release(inst *llamaInstance) {
	if inst == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if inst.activeReqs > 0 {
		inst.activeReqs--
	}
	inst.lastUsed = time.Now()
	if inst.activeReqs == 0 && m.cur == inst && inst.isAlive() {
		if inst.idleTimer != nil {
			inst.idleTimer.Stop()
		}
		inst.idleTimer = time.AfterFunc(llamaIdleTimeout, func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.cur == inst && inst.activeReqs == 0 && time.Since(inst.lastUsed) >= llamaIdleTimeout {
				m.stopInstanceLocked(inst)
			}
		})
	}
}

func (m *llamaManager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopInstanceLocked(m.cur)
}

func (m *llamaManager) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.stopInstanceLocked(m.cur)
}

func (m *llamaManager) stopIfModel(modelPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur != nil && m.cur.modelPath == modelPath {
		m.stopInstanceLocked(m.cur)
	}
}

func (m *llamaManager) stopInstanceLocked(inst *llamaInstance) {
	if inst == nil {
		return
	}
	if inst.idleTimer != nil {
		inst.idleTimer.Stop()
		inst.idleTimer = nil
	}
	if inst.cmd != nil && inst.cmd.Process != nil {
		_ = inst.cmd.Process.Kill()
	}
	if m.cur == inst {
		m.cur = nil
	}
}

func llamaBinName() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}

func llamaAssetSuffix() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		return "-bin-macos-arm64.tar.gz", nil
	case "darwin/amd64":
		return "-bin-macos-x64.tar.gz", nil
	case "windows/amd64":
		return "-bin-win-cpu-x64.zip", nil
	case "windows/arm64":
		return "-bin-win-cpu-arm64.zip", nil
	case "linux/amd64":
		return "-bin-ubuntu-x64.tar.gz", nil
	case "linux/arm64":
		return "-bin-ubuntu-arm64.tar.gz", nil
	default:
		return "", errors.New(locales.AiErrUnsupportedPlatform(runtime.GOOS + "/" + runtime.GOARCH))
	}
}

func llamaInstalled(binDir, binPath string) bool {
	st, err := os.Stat(binPath)
	if err != nil || st.IsDir() || st.Size() == 0 {
		return false
	}
	// .llama-ready is removed before moving files and atomically renamed into
	// place only after every file is in binDir, so its presence is the
	// authoritative commit marker even if a crash left .llama-installing behind.
	if marker, err := os.Stat(filepath.Join(binDir, ".llama-ready")); err == nil && !marker.IsDir() {
		return true
	}
	// Without .llama-ready, .llama-installing or a legacy download temp file
	// indicates an incomplete installation.
	if _, err := os.Stat(filepath.Join(binDir, ".llama-installing")); err == nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(binDir, "llama-download.tmp")); err == nil {
		return false
	}
	// Preserve offline compatibility for legacy pre-marker installations.
	return runtime.GOOS == "windows" || st.Mode()&0o111 != 0
}

func (m *llamaManager) ensureBinary(ctx context.Context, s model.Settings) (string, error) {
	ud, err := userDataDir()
	if err != nil {
		return "", err
	}
	binDir := filepath.Join(ud, "bin")
	binPath := filepath.Join(binDir, llamaBinName())
	if llamaInstalled(binDir, binPath) {
		return binPath, nil
	}

	m.binMu.Lock()
	defer m.binMu.Unlock()
	if llamaInstalled(binDir, binPath) {
		return binPath, nil
	}

	suffix, err := llamaAssetSuffix()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}

	stageDir, err := os.MkdirTemp(ud, "llama-stage-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stageDir)

	assetURL := resolveLlamaAssetURL(ctx, s, suffix)
	tmpFile := filepath.Join(stageDir, "llama-download.tmp")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	client := newHTTPClient(s, 0)
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New(locales.AiErrDownloadLlama(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New(locales.AiErrDownloadLlamaHTTP(resp.StatusCode))
	}

	out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}

	extractDir := filepath.Join(stageDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return "", err
	}
	if strings.HasSuffix(suffix, ".zip") {
		err = extractZip(tmpFile, extractDir)
	} else {
		err = extractTarGz(tmpFile, extractDir)
	}
	if err != nil {
		return "", errors.New(locales.AiErrExtractLlama(err.Error()))
	}

	stagedBin := filepath.Join(extractDir, llamaBinName())
	if st, err := os.Stat(stagedBin); err != nil || st.IsDir() || st.Size() == 0 {
		return "", errors.New(locales.AiErrNoLlamaBin())
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(stagedBin, 0o755)
	}

	markerPath := filepath.Join(binDir, ".llama-ready")
	installingPath := filepath.Join(binDir, ".llama-installing")
	if err := os.WriteFile(installingPath, []byte("installing\n"), 0o644); err != nil {
		return "", err
	}
	_ = os.Remove(markerPath)

	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		src := filepath.Join(extractDir, entry.Name())
		dst := filepath.Join(binDir, entry.Name())
		_ = os.Remove(dst)
		if err := os.Rename(src, dst); err != nil {
			return "", err
		}
	}

	markerTmp := filepath.Join(stageDir, ".llama-ready")
	if err := os.WriteFile(markerTmp, []byte("ok\n"), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(markerTmp, markerPath); err != nil {
		return "", err
	}
	_ = os.Remove(installingPath)
	_ = os.Remove(filepath.Join(binDir, "llama-download.tmp"))
	return binPath, nil
}

func resolveLlamaAssetURL(ctx context.Context, s model.Settings, suffix string) string {
	fallback := fmt.Sprintf("https://github.com/ggml-org/llama.cpp/releases/download/%s/llama-%s%s", llamaFallbackTag, llamaFallbackTag, suffix)

	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, llamaReleasesAPIURL, nil)
	if err != nil {
		return fallback
	}
	req.Header.Set("User-Agent", "cooool-mark")
	resp, err := newHTTPClient(s, 8*time.Second).Do(req)
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fallback
	}

	var releases []struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&releases); err != nil {
		return fallback
	}
	for _, rel := range releases {
		for _, a := range rel.Assets {
			if strings.HasPrefix(a.Name, "llama-") && strings.HasSuffix(a.Name, suffix) && a.URL != "" {
				return a.URL
			}
		}
	}
	return fallback
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		base := filepath.Base(hdr.Name)
		if base == "." || base == "/" || base == ".." {
			continue
		}
		target := filepath.Join(destDir, base)
		switch hdr.Typeflag {
		case tar.TypeReg:
			mode := os.FileMode(hdr.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			_ = os.Remove(target)
			_ = os.Symlink(filepath.Base(hdr.Linkname), target)
		}
	}
	return nil
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if base == "." || base == "/" || base == ".." {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, base)
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func pickFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

func waitLlamaHealth(ctx context.Context, port int, doneCh <-chan struct{}) error {
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-doneCh:
			return errors.New(locales.AiErrLlamaExited())
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

type limitedBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	const maxLen = 4096
	if len(b.buf) >= maxLen {
		return len(p), nil
	}
	rem := maxLen - len(b.buf)
	if len(p) > rem {
		b.buf = append(b.buf, p[:rem]...)
	} else {
		b.buf = append(b.buf, p...)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
