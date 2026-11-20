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
	"cooool-mark/internal/model"
)

const (
	llamaIdleTimeout    = 5 * time.Minute
	llamaFallbackTag    = "b12549"
	llamaReleasesAPIURL = "https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=10"
)

type llamaManager struct {
	e          *engine
	mu         sync.Mutex
	binMu      sync.Mutex
	cmd        *exec.Cmd
	port       int
	modelPath  string
	activeReqs int
	lastUsed   time.Time
	idleTimer  *time.Timer
	doneCh     chan struct{}
}

func newLlamaManager(e *engine) *llamaManager {
	return &llamaManager{e: e}
}

func (m *llamaManager) polish(ctx context.Context, s model.Settings, modelPath string, in app.PolishInput) (app.Polish, error) {
	port, err := m.acquire(ctx, s, modelPath)
	if err != nil {
		return app.Polish{}, err
	}
	defer m.release()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	extra := map[string]any{
		"response_format": map[string]any{"type": "json_object"},
		"chat_template_kwargs": map[string]any{
			"enable_thinking": false,
		},
	}
	return polishOpenAICompat(ctx, s, baseURL, "", filepath.Base(modelPath), in, extra)
}

func (m *llamaManager) acquire(ctx context.Context, s model.Settings, modelPath string) (int, error) {
	binPath, err := m.ensureBinary(ctx, s)
	if err != nil {
		return 0, err
	}

	m.mu.Lock()
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}
	if m.cmd != nil && m.modelPath == modelPath && m.isAliveLocked() {
		m.activeReqs++
		m.lastUsed = time.Now()
		port := m.port
		m.mu.Unlock()
		return port, nil
	}
	m.stopLocked()

	port, err := pickFreePort()
	if err != nil {
		m.mu.Unlock()
		return 0, err
	}

	cmd := exec.Command(binPath,
		"-m", modelPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"-c", "2048",
	)
	cmd.Dir = filepath.Dir(binPath)
	hideWindow(cmd)
	var errBuf limitedBuffer
	cmd.Stderr = &errBuf

	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		return 0, fmt.Errorf("启动 llama-server 失败: %w", err)
	}

	doneCh := make(chan struct{})
	m.cmd = cmd
	m.port = port
	m.modelPath = modelPath
	m.activeReqs = 1
	m.lastUsed = time.Now()
	m.doneCh = doneCh
	go func() {
		_ = cmd.Wait()
		close(doneCh)
	}()
	m.mu.Unlock()

	if err := waitLlamaHealth(ctx, port, doneCh); err != nil {
		m.mu.Lock()
		if m.cmd == cmd {
			m.activeReqs = 0
			m.stopLocked()
		}
		m.mu.Unlock()
		detail := strings.TrimSpace(errBuf.String())
		if detail != "" {
			return 0, fmt.Errorf("%w: %s", err, detail)
		}
		return 0, err
	}
	return port, nil
}

func (m *llamaManager) release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeReqs > 0 {
		m.activeReqs--
	}
	m.lastUsed = time.Now()
	if m.activeReqs == 0 && m.cmd != nil {
		if m.idleTimer != nil {
			m.idleTimer.Stop()
		}
		cmd := m.cmd
		m.idleTimer = time.AfterFunc(llamaIdleTimeout, func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.cmd == cmd && m.activeReqs == 0 && time.Since(m.lastUsed) >= llamaIdleTimeout {
				m.stopLocked()
			}
		})
	}
}

func (m *llamaManager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *llamaManager) stopIfModel(modelPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.modelPath == modelPath {
		m.stopLocked()
	}
}

func (m *llamaManager) isAliveLocked() bool {
	if m.cmd == nil || m.doneCh == nil {
		return false
	}
	select {
	case <-m.doneCh:
		return false
	default:
		return true
	}
}

func (m *llamaManager) stopLocked() {
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
	m.cmd = nil
	m.port = 0
	m.modelPath = ""
	m.doneCh = nil
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
		return "", fmt.Errorf("不支持的平台: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func (m *llamaManager) ensureBinary(ctx context.Context, s model.Settings) (string, error) {
	ud, err := userDataDir()
	if err != nil {
		return "", err
	}
	binDir := filepath.Join(ud, "bin")
	binPath := filepath.Join(binDir, llamaBinName())
	if st, err := os.Stat(binPath); err == nil && !st.IsDir() {
		return binPath, nil
	}

	m.binMu.Lock()
	defer m.binMu.Unlock()
	if st, err := os.Stat(binPath); err == nil && !st.IsDir() {
		return binPath, nil
	}

	suffix, err := llamaAssetSuffix()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}

	assetURL := resolveLlamaAssetURL(ctx, s, suffix)
	tmpFile := filepath.Join(binDir, "llama-download.tmp")
	defer os.Remove(tmpFile)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	client := newHTTPClient(s, 0)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载 llama.cpp 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载 llama.cpp 失败（HTTP %d）", resp.StatusCode)
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

	if strings.HasSuffix(suffix, ".zip") {
		err = extractZip(tmpFile, binDir)
	} else {
		err = extractTarGz(tmpFile, binDir)
	}
	if err != nil {
		return "", fmt.Errorf("解压 llama.cpp 失败: %w", err)
	}

	if st, err := os.Stat(binPath); err != nil || st.IsDir() {
		return "", errors.New("llama.cpp 发行包中未找到 llama-server")
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(binPath, 0o755)
	}
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
			return errors.New("llama-server 启动后意外退出")
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
