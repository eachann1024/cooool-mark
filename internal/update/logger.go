package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxUpdaterLogBytes int64 = 1024 * 1024

type diagLogger struct {
	mu      sync.Mutex
	dataDir string
}

var diagLog = &diagLogger{}

func setLogDataDir(dataDir string) {
	diagLog.mu.Lock()
	diagLog.dataDir = strings.TrimSpace(dataDir)
	diagLog.mu.Unlock()
}

// LogFilePath returns <dataDir>/logs/updater.log.
func LogFilePath(dataDir string) string {
	return filepath.Join(strings.TrimSpace(dataDir), "logs", "updater.log")
}

// InstallLogFilePath returns <dataDir>/logs/updater-install.log, written by the detached swap script.
func InstallLogFilePath(dataDir string) string {
	return filepath.Join(strings.TrimSpace(dataDir), "logs", "updater-install.log")
}

func (l *diagLogger) write(level, event string, kv ...any) {
	var b strings.Builder
	b.WriteString(time.Now().Format("2006-01-02T15:04:05.000Z07:00"))
	b.WriteString(" [")
	b.WriteString(level)
	b.WriteString("] ")
	b.WriteString(event)
	for i := 0; i+1 < len(kv); i += 2 {
		value := strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(kv[i+1]), "\n", " "), "\r", " ")
		b.WriteByte(' ')
		b.WriteString(fmt.Sprint(kv[i]))
		b.WriteByte('=')
		if strings.ContainsAny(value, " \t\"") {
			b.WriteString(fmt.Sprintf("%q", value))
		} else {
			b.WriteString(value)
		}
	}
	b.WriteByte('\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.dataDir == "" {
		return
	}
	logPath := LogFilePath(l.dataDir)
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxUpdaterLogBytes {
		_ = os.Rename(logPath, filepath.Join(filepath.Dir(logPath), "updater.prev.log"))
	}
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		_, _ = f.WriteString(b.String())
		_ = f.Close()
	}
}

func logInfo(event string, kv ...any)  { diagLog.write("INFO", event, kv...) }
func logWarn(event string, kv ...any)  { diagLog.write("WARN", event, kv...) }
func logError(event string, kv ...any) { diagLog.write("ERROR", event, kv...) }
