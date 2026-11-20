package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
)

type modelSpec struct {
	id       string
	label    string
	filename string
	url      string
}

var localModelSpecs = []modelSpec{
	{
		id:       "qwen3.5-0.8b",
		label:    "Qwen3.5-0.8B（本地）",
		filename: "Qwen3.5-0.8B-Q4_K_M.gguf",
		url:      "https://huggingface.co/unsloth/Qwen3.5-0.8B-GGUF/resolve/main/Qwen3.5-0.8B-Q4_K_M.gguf",
	},
	{
		id:       "qwen3.5-2b",
		label:    "Qwen3.5-2B（本地）",
		filename: "Qwen3.5-2B-Q4_K_M.gguf",
		url:      "https://huggingface.co/unsloth/Qwen3.5-2B-GGUF/resolve/main/Qwen3.5-2B-Q4_K_M.gguf",
	},
	{
		id:       "qwen3.5-4b",
		label:    "Qwen3.5-4B（本地）",
		filename: "Qwen3.5-4B-Q4_K_M.gguf",
		url:      "https://huggingface.co/unsloth/Qwen3.5-4B-GGUF/resolve/main/Qwen3.5-4B-Q4_K_M.gguf",
	},
}

func resolveModelSpec(name string) modelSpec {
	if sp, ok := lookupModelSpec(name); ok {
		return sp
	}
	return localModelSpecs[0]
}

func lookupModelSpec(name string) (modelSpec, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return modelSpec{}, false
	}
	for _, sp := range localModelSpecs {
		if sp.id == n {
			return sp, true
		}
	}
	return modelSpec{}, false
}

type downloadState struct {
	modelID     string
	downloading bool
	paused      bool
	downloaded  int64
	total       int64
	errMsg      string
	cancel      context.CancelFunc
	done        chan struct{}
}

type partMeta struct {
	Total int64 `json:"total"`
}

func modelsDir() (string, error) {
	ud, err := userDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(ud, "models"), nil
}

func modelPaths(spec modelSpec) (finalPath, partPath, metaPath string, err error) {
	dir, err := modelsDir()
	if err != nil {
		return "", "", "", err
	}
	finalPath = filepath.Join(dir, spec.filename)
	partPath = finalPath + ".part"
	metaPath = finalPath + ".part.meta"
	return finalPath, partPath, metaPath, nil
}

func modelFileIfReady(spec modelSpec) (string, bool) {
	finalPath, _, _, err := modelPaths(spec)
	if err != nil {
		return "", false
	}
	st, err := os.Stat(finalPath)
	if err != nil || st.IsDir() || st.Size() == 0 {
		return "", false
	}
	return finalPath, true
}

func (e *engine) findReadyLocalSpec(preferred string) (modelSpec, bool) {
	pref := resolveModelSpec(preferred)
	if _, ok := modelFileIfReady(pref); ok {
		return pref, true
	}
	for _, sp := range localModelSpecs {
		if sp.id == pref.id {
			continue
		}
		if _, ok := modelFileIfReady(sp); ok {
			return sp, true
		}
	}
	return modelSpec{}, false
}

func (e *engine) LocalModelStatus(modelName string) app.AIStatus {
	spec, ok := lookupModelSpec(modelName)
	if !ok {
		return app.AIStatus{}
	}
	return e.localStatusFor(spec)
}

func (e *engine) localStatusFor(spec modelSpec) app.AIStatus {
	st := app.AIStatus{
		Engine:     "local",
		EngineName: spec.label,
	}
	if _, ok := modelFileIfReady(spec); ok {
		st.Ready = true
		st.Progress = 1
		return st
	}

	e.mu.Lock()
	dl := e.dl
	var snap downloadState
	hasDL := dl != nil && dl.modelID == spec.id
	if hasDL {
		snap = *dl
	}
	e.mu.Unlock()

	if hasDL {
		st.Downloading = snap.downloading
		st.Paused = snap.paused
		st.Downloaded = snap.downloaded
		st.Total = snap.total
		st.Err = snap.errMsg
		if snap.total > 0 {
			st.Progress = float64(snap.downloaded) / float64(snap.total)
			if st.Progress > 1 {
				st.Progress = 1
			}
		}
		return st
	}

	// Check if a paused .part file remains from a previous session.
	_, partPath, metaPath, err := modelPaths(spec)
	if err == nil {
		if info, err := os.Stat(partPath); err == nil && !info.IsDir() && info.Size() > 0 {
			st.Paused = true
			st.Downloaded = info.Size()
			if total := readPartMeta(metaPath); total > 0 {
				st.Total = total
				st.Progress = float64(info.Size()) / float64(total)
			}
		}
	}
	return st
}

func (e *engine) Download(modelName string) error {
	if strings.TrimSpace(modelName) == "" {
		modelName = e.currentSettings().LocalModel
		if modelName == "" {
			modelName = "qwen3.5-2b"
		}
	}
	spec, ok := lookupModelSpec(modelName)
	if !ok {
		return fmt.Errorf("未知的本地模型: %s", modelName)
	}
	if _, ready := modelFileIfReady(spec); ready {
		e.notify()
		return nil
	}

	finalPath, partPath, metaPath, err := modelPaths(spec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return err
	}

	e.downloads.Lock()
	defer e.downloads.Unlock()

	e.mu.Lock()
	prev := e.dl
	if prev != nil && prev.modelID == spec.id && prev.downloading {
		e.mu.Unlock()
		return nil
	}
	if prev != nil && prev.cancel != nil {
		prev.cancel()
	}
	e.mu.Unlock()
	if prev != nil {
		<-prev.done
	}
	if _, ready := modelFileIfReady(spec); ready {
		return nil
	}

	var existingSize int64
	if info, err := os.Stat(partPath); err == nil && !info.IsDir() {
		existingSize = info.Size()
	}
	ctx, cancel := context.WithCancel(context.Background())
	dl := &downloadState{
		modelID:     spec.id,
		downloading: true,
		downloaded:  existingSize,
		total:       readPartMeta(metaPath),
		cancel:      cancel,
		done:        make(chan struct{}),
	}
	e.mu.Lock()
	e.dl = dl
	s := e.settings
	e.mu.Unlock()

	e.notify()
	go func() {
		defer close(dl.done)
		defer cancel()
		e.runDownload(ctx, dl, s, spec, finalPath, partPath, metaPath)
	}()
	return nil
}

func (e *engine) Pause() {
	e.mu.Lock()
	dl := e.dl
	if dl == nil || !dl.downloading {
		e.mu.Unlock()
		return
	}
	cancel := dl.cancel
	dl.downloading = false
	dl.paused = true
	dl.cancel = nil
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	e.notify()
}

func (e *engine) Remove(modelName string) error {
	if strings.TrimSpace(modelName) == "" {
		modelName = e.currentSettings().LocalModel
	}
	spec, ok := lookupModelSpec(modelName)
	if !ok {
		return fmt.Errorf("未知的本地模型: %s", modelName)
	}

	e.downloads.Lock()
	defer e.downloads.Unlock()

	e.mu.Lock()
	dl := e.dl
	if dl != nil && dl.modelID == spec.id {
		if dl.cancel != nil {
			dl.cancel()
		}
		e.dl = nil
	}
	e.mu.Unlock()
	if dl != nil && dl.modelID == spec.id {
		<-dl.done
	}

	finalPath, partPath, metaPath, err := modelPaths(spec)
	if err != nil {
		return err
	}
	e.llama.stopIfModel(finalPath)

	for _, p := range []string{finalPath, partPath, metaPath} {
		if rmErr := os.Remove(p); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return rmErr
		}
	}
	e.notify()
	return nil
}

func (e *engine) runDownload(
	ctx context.Context,
	dl *downloadState,
	s model.Settings,
	spec modelSpec,
	finalPath, partPath, metaPath string,
) {
	fail := func(err error) {
		e.mu.Lock()
		if e.dl == dl {
			dl.downloading = false
			if ctx.Err() == context.Canceled {
				dl.paused = true
				dl.errMsg = ""
			} else {
				dl.paused = dl.downloaded > 0
				dl.errMsg = err.Error()
			}
			dl.cancel = nil
		}
		e.mu.Unlock()
		e.notify()
	}

	var offset int64
	if info, err := os.Stat(partPath); err == nil && !info.IsDir() {
		offset = info.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.url, nil)
	if err != nil {
		fail(err)
		return
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	client := newHTTPClient(s, 0)
	resp, err := client.Do(req)
	if err != nil {
		fail(err)
		return
	}
	defer resp.Body.Close()
	if ctx.Err() != nil {
		fail(ctx.Err())
		return
	}

	var total int64
	var openFlags int
	switch resp.StatusCode {
	case http.StatusPartialContent:
		total = parseTotalFromContentRange(resp.Header.Get("Content-Range"))
		if total <= 0 && resp.ContentLength > 0 {
			total = offset + resp.ContentLength
		}
		openFlags = os.O_CREATE | os.O_WRONLY | os.O_APPEND

	case http.StatusOK:
		offset = 0
		total = max(0, resp.ContentLength)
		openFlags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC

	case http.StatusRequestedRangeNotSatisfiable:
		if crTotal := parseTotalFromContentRange(resp.Header.Get("Content-Range")); crTotal > 0 && offset == crTotal {
			if err := os.Rename(partPath, finalPath); err != nil {
				fail(err)
				return
			}
			_ = os.Remove(metaPath)
			e.finishDownload(dl, crTotal)
			return
		}
		if offset == 0 {
			fail(fmt.Errorf("下载模型失败（HTTP %d）", resp.StatusCode))
			return
		}
		// Restart from byte 0 if range was invalid.
		resp.Body.Close()
		_ = os.Remove(partPath)
		_ = os.Remove(metaPath)
		e.mu.Lock()
		if e.dl == dl {
			dl.downloaded = 0
			dl.total = 0
		}
		e.mu.Unlock()
		e.restartFromZero(ctx, dl, s, spec, finalPath, partPath, metaPath)
		return

	default:
		fail(fmt.Errorf("下载模型失败（HTTP %d）", resp.StatusCode))
		return
	}

	if total > 0 {
		writePartMeta(metaPath, total)
	}
	e.mu.Lock()
	if e.dl == dl {
		dl.downloaded = offset
		dl.total = total
	}
	e.mu.Unlock()
	e.notify()

	f, err := os.OpenFile(partPath, openFlags, 0o644)
	if err != nil {
		fail(err)
		return
	}

	buf := make([]byte, 64*1024)
	lastNotify := time.Now()
	written := offset
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				_ = f.Close()
				fail(wErr)
				return
			}
			written += int64(n)
			e.mu.Lock()
			if e.dl == dl {
				dl.downloaded = written
			}
			e.mu.Unlock()
			if time.Since(lastNotify) >= 200*time.Millisecond {
				lastNotify = time.Now()
				e.notify()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = f.Close()
			fail(readErr)
			return
		}
	}

	if err := f.Close(); err != nil {
		fail(err)
		return
	}
	if ctx.Err() != nil {
		fail(ctx.Err())
		return
	}
	if total > 0 && written != total {
		fail(fmt.Errorf("下载不完整（%d / %d 字节）", written, total))
		return
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		fail(err)
		return
	}
	_ = os.Remove(metaPath)
	e.finishDownload(dl, written)
}

func (e *engine) restartFromZero(
	ctx context.Context,
	dl *downloadState,
	s model.Settings,
	spec modelSpec,
	finalPath, partPath, metaPath string,
) {
	e.runDownload(ctx, dl, s, spec, finalPath, partPath, metaPath)
}

func (e *engine) finishDownload(dl *downloadState, total int64) {
	e.mu.Lock()
	if e.dl == dl {
		dl.downloading = false
		dl.paused = false
		dl.downloaded = total
		dl.total = total
		dl.errMsg = ""
		dl.cancel = nil
	}
	e.mu.Unlock()
	e.notify()
}

func parseTotalFromContentRange(header string) int64 {
	// Format: "bytes 0-1023/2048" or "bytes */2048"
	slash := strings.LastIndexByte(header, '/')
	if slash < 0 || slash+1 >= len(header) {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(header[slash+1:]), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func readPartMeta(path string) int64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var m partMeta
	if json.Unmarshal(b, &m) != nil {
		return 0
	}
	return m.Total
}

func writePartMeta(path string, total int64) {
	b, _ := json.Marshal(partMeta{Total: total})
	_ = os.WriteFile(path, b, 0o644)
}
