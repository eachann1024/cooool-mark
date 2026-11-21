package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

const (
	maxUpdateBytes int64 = 2 << 30
	macAppName           = "Cooool Mark"
	macBundleID          = "app.cooool.mark"
	stageDirPrefix       = ".cooool-mark-update-"
	lsregisterPath       = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

var stageDirPattern = regexp.MustCompile(`^\.cooool-mark-update-[0-9a-fA-F]+$`)

func markMacNeverIndex(dir string) {
	if runtime.GOOS != "darwin" || dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, ".metadata_never_index"), nil, 0o600)
}

// VerifiedRecord persists a verified, staged update in <dataDir>/app-updates/ready.json.
type VerifiedRecord struct {
	Version     string `json:"version"`
	Tag         string `json:"tag"`
	AssetName   string `json:"asset_name"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ArchivePath string `json:"archive_path"`
	PublishedAt string `json:"published_at,omitempty"`
	ReleaseURL  string `json:"release_url,omitempty"`
	Channel     string `json:"channel,omitempty"`
	InPlace     bool   `json:"in_place"`
}

type installStageRecord struct {
	Path        string `json:"path"`
	FromVersion string `json:"from_version,omitempty"`
	Version     string `json:"version"`
}

func updatesRoot(dataDir string) string     { return filepath.Join(dataDir, "app-updates") }
func readyRecordPath(dataDir string) string { return filepath.Join(updatesRoot(dataDir), "ready.json") }
func stageRecordPath(dataDir string) string {
	return filepath.Join(updatesRoot(dataDir), "install-stage.json")
}

// PickPlatformAsset chooses the allowlisted installer for the running OS/arch and reports whether it
// supports automatic in-place replacement, or only a manual install (pkgManaged).
func PickPlatformAsset(assets []ReleaseAsset, goos, goarch string) (picked *ReleaseAsset, inPlace bool, pkgManaged bool) {
	find := func(name string) *ReleaseAsset {
		for i := range assets {
			if assets[i].Name == name {
				cp := assets[i]
				return &cp
			}
		}
		return nil
	}
	switch goos {
	case "darwin":
		switch goarch {
		case "arm64":
			return find("cooool-mark-macos-arm64.dmg"), true, false
		case "amd64":
			return find("cooool-mark-macos-x64.dmg"), true, false
		}
	case "windows":
		if goarch == "amd64" {
			return find("cooool-mark-windows-x64-setup.exe"), true, false
		}
	case "linux":
		if goarch == "amd64" {
			if os.Getenv("APPIMAGE") != "" {
				return find("cooool-mark-linux-x64.AppImage"), true, false
			}
			if isLinuxSystemPackageManaged() {
				return nil, false, true
			}
			return find("cooool-mark-linux-x64.deb"), false, false
		}
	}
	return nil, false, true
}

func isLinuxSystemPackageManaged() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return false
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if out, err := exec.Command("pacman", "-Qo", exe).Output(); err == nil && len(out) > 0 {
		return true
	}
	if out, err := exec.Command("rpm", "-qf", exe).Output(); err == nil {
		s := strings.ToLower(strings.TrimSpace(string(out)))
		if s != "" && !strings.Contains(s, "not owned") {
			return true
		}
	}
	return false
}

// VerifyFileSHA256 checks that path matches expectedHex (64 hex chars).
func VerifyFileSHA256(path, expectedHex string) error {
	want := strings.ToLower(strings.TrimSpace(expectedHex))
	if len(want) != 64 {
		return errors.New("missing update SHA-256 checksum")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("SHA-256 mismatch: expected %s, got %s", want[:12], got[:12])
	}
	return nil
}

// DownloadProgressCallback reports live download progress, speed (MB/s) and the active channel.
type DownloadProgressCallback func(downloaded, total int64, speedMBps float64, channel string)

// DownloadAndStage downloads the asset across candidate sources, verifies its SHA-256 and, for
// in-place updates, persists ready.json. commitGuard lets the caller veto the final commit.
func DownloadAndStage(
	ctx context.Context,
	dataDir string,
	entry ReleaseEntry,
	asset ReleaseAsset,
	inPlace bool,
	mode SourceMode,
	onProgress DownloadProgressCallback,
	commitGuard func(commit func() error) error,
) (*VerifiedRecord, error) {
	expectedSHA := asset.EffectiveSHA256()
	version, _, _, validVersion := ParseReleaseTag(entry.Tag)
	if !validVersion || version != entry.Version || !IsTrustedReleaseAsset(asset, entry.Tag) || asset.Size > maxUpdateBytes {
		logError("download.untrusted_metadata", "version", entry.Version, "asset", asset.Name)
		return nil, errors.New("release asset metadata is invalid")
	}
	cands := CandidateAssetURLs(asset.DownloadURL, mode)
	if len(cands) == 0 {
		logError("download.untrusted_url", "version", entry.Version, "url", asset.DownloadURL)
		return nil, errors.New("untrusted release download URL")
	}

	verDir := filepath.Join(updatesRoot(dataDir), entry.Version)
	if err := os.MkdirAll(verDir, 0o700); err != nil {
		logError("download.mkdir_failed", "dir", verDir, "err", err)
		return nil, err
	}
	markMacNeverIndex(updatesRoot(dataDir))
	finalPath := filepath.Join(verDir, asset.Name)
	tempDir, err := os.MkdirTemp(verDir, ".download-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	partPath := filepath.Join(tempDir, asset.Name)

	logInfo("download.start", "version", entry.Version, "asset", asset.Name, "expected_bytes", asset.Size, "sha256", expectedSHA[:16], "mode", mode, "candidates", len(cands))

	var usedChannel string
	var lastErr error
	for idx, cand := range cands {
		if ctx.Err() != nil {
			logInfo("download.cancelled", "version", entry.Version, "err", ctx.Err())
			return nil, ctx.Err()
		}
		_ = os.Remove(partPath)
		started := time.Now()
		err := downloadOneSource(ctx, cand, partPath, asset.Size, expectedSHA, onProgress)
		elapsedMs := time.Since(started).Milliseconds()
		if err == nil {
			usedChannel = cand.Channel
			lastErr = nil
			logInfo("download.verified", "version", entry.Version, "channel", cand.Channel, "elapsed_ms", elapsedMs, "dest", finalPath)
			break
		}
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			logInfo("download.cancelled", "version", entry.Version)
			return nil, ctx.Err()
		}
		logWarn("download.source_failover", "attempt", idx+1, "channel", cand.Channel, "url", cand.URL, "elapsed_ms", elapsedMs, "err", err)
		lastErr = err
	}
	if lastErr != nil {
		logError("download.all_sources_failed", "version", entry.Version, "asset", asset.Name, "err", lastErr)
		return nil, lastErr
	}

	rec := &VerifiedRecord{
		Version:     entry.Version,
		Tag:         entry.Tag,
		AssetName:   asset.Name,
		DownloadURL: asset.DownloadURL,
		SHA256:      expectedSHA,
		Size:        asset.Size,
		ArchivePath: finalPath,
		PublishedAt: entry.PublishedAt,
		ReleaseURL:  entry.HTMLURL,
		Channel:     usedChannel,
		InPlace:     inPlace,
	}
	doCommit := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := os.Rename(partPath, finalPath); err != nil {
			logError("download.rename_failed", "from", partPath, "to", finalPath, "err", err)
			return err
		}
		if inPlace {
			raw, err := json.MarshalIndent(rec, "", "  ")
			if err != nil {
				return err
			}
			if err := writeFileAtomic(readyRecordPath(dataDir), raw, 0o600); err != nil {
				logError("ready.persist_failed", "version", entry.Version, "err", err)
				return err
			}
			logInfo("ready.persisted", "version", entry.Version)
		}
		return nil
	}
	if commitGuard != nil {
		if err := commitGuard(doCommit); err != nil {
			return nil, err
		}
	} else if err := doCommit(); err != nil {
		return nil, err
	}
	return rec, nil
}

func downloadOneSource(
	ctx context.Context,
	cand LabeledURL,
	destPath string,
	expectedSize int64,
	expectedSHA string,
	onProgress DownloadProgressCallback,
) error {
	body, contentLen, err := openDownloadStream(ctx, cand.URL)
	if err != nil {
		return err
	}
	defer body.Close()

	if expectedSize > maxUpdateBytes || contentLen > maxUpdateBytes {
		return errors.New("update package is too large")
	}
	total := expectedSize
	if total <= 0 && contentLen > 0 {
		total = contentLen
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	hasher := sha256.New()
	buf := make([]byte, 64*1024)
	var written int64
	started := time.Now()
	var lastTick time.Time
	if onProgress != nil {
		onProgress(0, total, 0, cand.Channel)
	}
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if _, writeErr := f.Write(chunk); writeErr != nil {
				return writeErr
			}
			hasher.Write(chunk)
			written += int64(n)
			if written > maxUpdateBytes || (total > 0 && written > total) {
				return errors.New("downloaded size exceeded manifest size")
			}
			if now := time.Now(); onProgress != nil && now.Sub(lastTick) >= 80*time.Millisecond {
				lastTick = now
				speed := 0.0
				if secs := now.Sub(started).Seconds(); secs > 0.05 {
					speed = (float64(written) / (1024.0 * 1024.0)) / secs
				}
				onProgress(written, total, speed, cand.Channel)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	if err := f.Sync(); err != nil {
		return err
	}
	if expectedSize > 0 && written != expectedSize {
		return fmt.Errorf("incomplete download (%d / %d bytes)", written, expectedSize)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != strings.ToLower(expectedSHA) {
		logError("download.checksum_mismatch", "channel", cand.Channel, "expected", expectedSHA[:16], "got", got[:16])
		return fmt.Errorf("checksum mismatch from %s", cand.Channel)
	}
	return nil
}

func prepareMacAppFromDMG(dmgPath, unpackedDir, expectedVersion string) (string, error) {
	if err := os.MkdirAll(unpackedDir, 0o700); err != nil {
		return "", err
	}
	markMacNeverIndex(unpackedDir)
	mountDir, err := os.MkdirTemp("", "cooool-mark-dmg-mnt-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(mountDir)

	attach := exec.Command("/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mountDir, dmgPath)
	if out, err := attach.CombinedOutput(); err != nil {
		return "", fmt.Errorf("mount DMG failed: %s", strings.TrimSpace(string(out)))
	}
	srcApp := filepath.Join(mountDir, macAppName+".app")
	detached := false
	detach := func() {
		if detached {
			return
		}
		detached = true
		_ = exec.Command(lsregisterPath, "-u", srcApp).Run()
		_ = exec.Command("/usr/bin/hdiutil", "detach", mountDir, "-force").Run()
	}
	defer detach()

	info, err := os.Lstat(srcApp)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("DMG does not contain %s.app", macAppName)
	}

	dstApp := filepath.Join(unpackedDir, macAppName+".bundle")
	if out, err := exec.Command("/usr/bin/ditto", srcApp, dstApp).CombinedOutput(); err != nil {
		return "", fmt.Errorf("unpack %s.app failed: %s", macAppName, strings.TrimSpace(string(out)))
	}
	detach()
	_ = exec.Command(lsregisterPath, "-u", dstApp).Run()

	if err := validateMacBundle(dstApp, expectedVersion); err != nil {
		return "", err
	}
	return dstApp, nil
}

func validateMacBundle(bundlePath, expectedVersion string) error {
	st, err := os.Lstat(bundlePath)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("invalid macOS application bundle")
	}
	plistPath := filepath.Join(bundlePath, "Contents", "Info.plist")
	extractKey := func(key string) (string, error) {
		out, err := exec.Command("/usr/bin/plutil", "-extract", key, "raw", "-o", "-", plistPath).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	id, err := extractKey("CFBundleIdentifier")
	if err != nil || id != macBundleID {
		return fmt.Errorf("bundle identifier mismatch (%s)", id)
	}
	if expectedVersion != "" {
		ver, err := extractKey("CFBundleShortVersionString")
		if err != nil || ver == "" || CompareVersion(ver, expectedVersion) != 0 {
			return fmt.Errorf("bundle version mismatch (%s != %s)", ver, expectedVersion)
		}
	}
	exePath := filepath.Join(bundlePath, "Contents", "MacOS", macAppName)
	lipoOut, err := exec.Command("/usr/bin/lipo", "-archs", exePath).Output()
	if err != nil {
		return errors.New("unable to inspect update binary architecture")
	}
	wantArch := "x86_64"
	if runtime.GOARCH == "arm64" {
		wantArch = "arm64"
	}
	if !slices.Contains(strings.Fields(string(lipoOut)), wantArch) {
		return fmt.Errorf("update architecture mismatch (expected %s)", wantArch)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--verbose=2", bundlePath).CombinedOutput(); err != nil {
		return fmt.Errorf("update bundle signature verification failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func installedMacBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(bundle, ".app") {
		return "", errors.New("current process is not running inside a macOS .app bundle")
	}
	return bundle, nil
}

func livesInUpdateStage(bundle string) bool {
	for _, part := range strings.Split(filepath.Clean(bundle), string(filepath.Separator)) {
		if stageDirPattern.MatchString(part) {
			return true
		}
	}
	return false
}

// AcknowledgeStartupAndClean tells a pending macOS install stage that the new version launched
// cleanly, drops a stale ready.json, and prunes cached downloads other than the staged version.
func AcknowledgeStartupAndClean(dataDir, runningVersion string) {
	root := updatesRoot(dataDir)
	_ = os.MkdirAll(root, 0o700)
	markMacNeverIndex(root)

	exe, _ := os.Executable()
	logInfo("startup.init", "running_version", runningVersion, "os", runtime.GOOS, "arch", runtime.GOARCH, "exe", exe)

	if runtime.GOOS == "darwin" {
		if bundle, err := installedMacBundle(); err == nil && !livesInUpdateStage(bundle) {
			keepStage := ""
			if raw, readErr := os.ReadFile(stageRecordPath(dataDir)); readErr == nil {
				var st installStageRecord
				if json.Unmarshal(raw, &st) == nil && filepath.Dir(st.Path) == filepath.Dir(bundle) &&
					stageDirPattern.MatchString(filepath.Base(st.Path)) {
					keepStage = st.Path
					stageInfo, stageErr := os.Lstat(st.Path)
					if stageErr == nil && stageInfo.IsDir() && CompareVersion(runningVersion, st.Version) == 0 {
						if err := os.WriteFile(filepath.Join(st.Path, "started"), []byte(fmt.Sprintf("%d", os.Getpid())), 0o600); err == nil {
							_ = os.Remove(stageRecordPath(dataDir))
							logInfo("startup.install_acknowledged", "from_version", st.FromVersion, "installed_version", runningVersion, "stage", st.Path)
						}
					}
				}
			}
			pruneOrphanStages(bundle, keepStage)
		}
	}

	var keepVer string
	if raw, err := os.ReadFile(readyRecordPath(dataDir)); err == nil {
		var rec VerifiedRecord
		if json.Unmarshal(raw, &rec) == nil && CompareVersion(rec.Version, runningVersion) > 0 {
			keepVer = rec.Version
		} else {
			_ = os.Remove(readyRecordPath(dataDir))
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, ent := range entries {
		if !ent.IsDir() || ent.Name() == keepVer {
			continue
		}
		if _, _, _, ok := ParseReleaseTag(ent.Name()); ok {
			_ = os.RemoveAll(filepath.Join(root, ent.Name()))
			logInfo("startup.pruned_old_cache", "version", ent.Name())
		}
	}
}

// RestoreReady revalidates a persisted ready.json after a restart without downloading again.
func RestoreReady(dataDir, runningVersion string) (*VerifiedRecord, bool) {
	raw, err := os.ReadFile(readyRecordPath(dataDir))
	if err != nil {
		return nil, false
	}
	var rec VerifiedRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		logWarn("restore.invalid_json", "err", err)
		_ = os.Remove(readyRecordPath(dataDir))
		return nil, false
	}
	if err := validateReadyFromOfficialSource(dataDir, runningVersion, &rec); err != nil {
		logWarn("restore.untrusted_update", "err", err)
		return nil, false
	}
	logInfo("restore.ready_ok", "version", rec.Version, "archive", rec.ArchivePath)
	return &rec, true
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-ready-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func matchOfficialReleaseAsset(release ReleaseEntry, rec *VerifiedRecord, fileSize int64) (*ReleaseAsset, bool) {
	if release.Version != rec.Version || release.Tag != rec.Tag {
		return nil, false
	}
	official, inPlace, _ := PickPlatformAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	if inPlace && official != nil && official.Name == rec.AssetName && official.DownloadURL == rec.DownloadURL &&
		(official.Size <= 0 || official.Size == fileSize) &&
		strings.EqualFold(official.EffectiveSHA256(), rec.SHA256) {
		return official, true
	}
	return nil, false
}

var errStagedIntegrity = errors.New("staged update integrity failed")

// validateReadyFromOfficialSource treats a ready record as a hint about a local download; GitHub
// stays authoritative for the asset name, URL, size and digest.
func validateReadyFromOfficialSource(dataDir, runningVersion string, rec *VerifiedRecord) error {
	version, _, _, valid := ParseReleaseTag(rec.Version)
	if !valid || version != rec.Version || CompareVersion(rec.Version, runningVersion) <= 0 {
		return fmt.Errorf("%w: staged update version is invalid", errStagedIntegrity)
	}
	asset := ReleaseAsset{Name: rec.AssetName, DownloadURL: rec.DownloadURL, SHA256: rec.SHA256}
	if !rec.InPlace || !IsTrustedReleaseAsset(asset, rec.Tag) {
		return fmt.Errorf("%w: staged update metadata is invalid", errStagedIntegrity)
	}
	expected := filepath.Join(updatesRoot(dataDir), rec.Version, rec.AssetName)
	if filepath.Clean(rec.ArchivePath) != expected {
		return fmt.Errorf("%w: staged update path does not match the platform asset", errStagedIntegrity)
	}
	root, err := filepath.EvalSymlinks(updatesRoot(dataDir))
	if err != nil {
		return fmt.Errorf("%w: %v", errStagedIntegrity, err)
	}
	resolved, err := filepath.EvalSymlinks(expected)
	if err != nil || resolved != filepath.Join(root, rec.Version, rec.AssetName) {
		return fmt.Errorf("%w: staged update must stay in its cache directory", errStagedIntegrity)
	}
	info, err := os.Stat(expected)
	if err != nil {
		return fmt.Errorf("%w: %v", errStagedIntegrity, err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxUpdateBytes {
		return fmt.Errorf("%w: staged update file type or size is invalid", errStagedIntegrity)
	}

	verify := func(release ReleaseEntry) (bool, error) {
		official, ok := matchOfficialReleaseAsset(release, rec, info.Size())
		if !ok {
			return false, nil
		}
		if shaErr := VerifyFileSHA256(expected, official.EffectiveSHA256()); shaErr != nil {
			return true, fmt.Errorf("%w: %v", errStagedIntegrity, shaErr)
		}
		return true, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	releases, _, _, fetchErr := FetchReleases(ctx)
	checkedPlatformAsset := false
	if fetchErr == nil {
		for _, release := range releases {
			if release.Version == rec.Version && release.Tag == rec.Tag {
				if platformAsset, _, _ := PickPlatformAsset(release.Assets, runtime.GOOS, runtime.GOARCH); platformAsset != nil {
					checkedPlatformAsset = true
				}
			}
			if matched, verifyErr := verify(release); matched {
				return verifyErr
			}
		}
	}
	if !checkedPlatformAsset {
		var transientErr error
		for _, tag := range []string{rec.Tag, "v" + rec.Version} {
			tagCtx, tagCancel := context.WithTimeout(context.Background(), 15*time.Second)
			release, tagErr := FetchReleaseByTag(tagCtx, tag)
			tagCancel()
			if tagErr != nil {
				if !errors.Is(tagErr, errReleaseNotFound) {
					transientErr = tagErr
				}
				continue
			}
			if release == nil {
				continue
			}
			if matched, verifyErr := verify(*release); matched {
				return verifyErr
			}
			return fmt.Errorf("%w: staged update does not match the official platform release", errStagedIntegrity)
		}
		if transientErr != nil {
			return fmt.Errorf("cannot verify the staged update with GitHub: %w", transientErr)
		}
		if fetchErr != nil {
			return fmt.Errorf("cannot verify the staged update with GitHub: %w", fetchErr)
		}
	}
	return fmt.Errorf("%w: staged update does not match the official platform release", errStagedIntegrity)
}

const macSwapInstallerScript = `#!/bin/sh
set -eu
target="$1"
stage="$2"
parent_pid="$3"
target_ver="$4"
relaunch="${5:-1}"
ts() { date "+%Y-%m-%dT%H:%M:%S%z"; }
log() { printf "%s [install.sh] %s\n" "$(ts)" "$*"; }
LSREGISTER=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister

log "begin target=$target stage=$stage parent_pid=$parent_pid target_ver=$target_ver relaunch=$relaunch"
open_target() {
  if [ "$relaunch" = "1" ]; then
    /usr/bin/open "$target" || true
  fi
}
count=0
while kill -0 "$parent_pid" 2>/dev/null; do
  count=$((count + 1))
  if [ "$count" -gt 240 ]; then
    log "ERROR timed out waiting for parent_pid=$parent_pid to exit"
    exit 1
  fi
  sleep 0.25
done
log "parent exited after $((count * 250))ms"

remove_stage() {
  rm -rf "$stage"
}
rollback() {
  log "WARN executing rollback for target=$target"
  if [ -d "$stage/previous.bundle" ]; then
    if [ -e "$target" ] && ! mv "$target" "$stage/failed.bundle"; then
      log "ERROR rollback could not move the new bundle; backup kept at $stage/previous.bundle"
      exit 1
    fi
    if ! mv "$stage/previous.bundle" "$target"; then
      log "ERROR rollback could not restore the old bundle; backup kept at $stage/previous.bundle"
      exit 1
    fi
  fi
  "$LSREGISTER" -f "$target" >/dev/null 2>&1 || true
  open_target
  remove_stage
  log "rollback complete"
}
abort() {
  trap - HUP INT TERM
  rollback
  exit 1
}
trap abort HUP INT TERM

if ! mv "$target" "$stage/previous.bundle"; then
  log "ERROR mv current bundle to previous.bundle failed"
  open_target
  remove_stage
  exit 1
fi
if ! mv "$stage/next.bundle" "$target"; then
  log "ERROR mv next.bundle to target failed"
  rollback
  exit 1
fi
"$LSREGISTER" -f "$target" >/dev/null 2>&1 || true
if [ "$relaunch" != "1" ]; then
  trap - HUP INT TERM
  remove_stage
  log "bundle swapped into place on quit; update to v$target_ver will run on next launch"
  exit 0
fi
log "bundle swapped into place; launching updated app"
if ! /usr/bin/open "$target"; then
  log "ERROR open updated target failed"
  rollback
  exit 1
fi

count=0
while [ ! -f "$stage/started" ]; do
  count=$((count + 1))
  if [ "$count" -gt 60 ]; then
    if ! /usr/bin/pgrep -f "$target/Contents/MacOS/Cooool Mark" >/dev/null 2>&1; then
      log "ERROR updated process not running after 60s; rolling back"
      rollback
      exit 1
    fi
    log "WARN updated process did not acknowledge startup; backup kept at $stage/previous.bundle"
    exit 1
  fi
  sleep 1
done
log "handshake confirmed ($stage/started); update to v$target_ver succeeded"
remove_stage
`

const linuxAppImageInstallerScript = `#!/bin/sh
set -eu
target="$1"
next_bin="$2"
parent_pid="$3"
ts() { date "+%Y-%m-%dT%H:%M:%S%z"; }
log() { printf "%s [install-linux.sh] %s\n" "$(ts)" "$*"; }

log "waiting for parent_pid=$parent_pid"
count=0
while kill -0 "$parent_pid" 2>/dev/null; do
  count=$((count + 1))
  if [ "$count" -gt 240 ]; then
    log "ERROR timed out waiting for parent exit"
    exit 1
  fi
  sleep 0.25
done

target_dir="$(dirname "$target")"
target_base="$(basename "$target")"
staged="$target_dir/.${target_base}.next.$$"
backup="$target_dir/.${target_base}.bak.$$"
swapped=0

cleanup() {
  rm -f "$staged"
  if [ "$swapped" -eq 0 ]; then
    rm -f "$backup"
  fi
}
trap cleanup EXIT

rollback() {
  log "rolling back AppImage to $backup"
  rm -f "$staged"
  if [ -f "$backup" ]; then
    mv -f "$backup" "$target" || true
  fi
}

cp "$next_bin" "$staged"
chmod 755 "$staged"
if [ -f "$target" ]; then
  cp -p "$target" "$backup"
fi
if ! mv -f "$staged" "$target"; then
  log "ERROR atomic swap failed"
  rollback
  exit 1
fi
swapped=1
log "swapped AppImage at $target; relaunching"
"$target" >/dev/null 2>&1 &
new_pid=$!
sleep 2
if ! kill -0 "$new_pid" 2>/dev/null; then
  log "ERROR updated AppImage exited immediately; rolling back"
  rollback
  "$target" >/dev/null 2>&1 &
  exit 1
fi
rm -f "$backup" "$next_bin"
log "updated AppImage running (pid=$new_pid)"
`

// InstallReady verifies the staged package, flushes user data through flushBeforeQuit, starts the
// platform replacement helper once the app has quit, and asks quitApp to quit.
func InstallReady(dataDir string, rec *VerifiedRecord, isChinese bool, flushBeforeQuit func() error, quitApp func(func() error) error) error {
	if rec == nil || quitApp == nil {
		return errors.New("no staged update ready to install")
	}
	tr := func(zh, en string) string {
		if isChinese {
			return zh
		}
		return en
	}
	fromVer := CurrentVersion()
	logInfo("install.begin", "from_version", fromVer, "to_version", rec.Version, "archive", rec.ArchivePath)

	if err := validateReadyFromOfficialSource(dataDir, fromVer, rec); err != nil {
		logError("install.verification_failed", "version", rec.Version, "archive", rec.ArchivePath, "err", err)
		return err
	}

	installLogPath := InstallLogFilePath(dataDir)
	_ = os.MkdirAll(filepath.Dir(installLogPath), 0o755)
	flush := func() error {
		if flushBeforeQuit == nil {
			return nil
		}
		if err := flushBeforeQuit(); err != nil {
			return fmt.Errorf("%s: %w", tr("更新前无法保存数据", "cannot save changes before updating"), err)
		}
		return nil
	}
	spawn := func(cmd *exec.Cmd) (func(), error) {
		configureDetachedProcess(cmd)
		logFile, _ := os.OpenFile(installLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if logFile != nil {
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		}
		return func() {
			if logFile != nil {
				_ = logFile.Close()
			}
		}, nil
	}

	switch runtime.GOOS {
	case "darwin":
		bundle, stage, err := stageMacUpdate(rec, tr)
		if err != nil {
			return err
		}
		scriptPath := filepath.Join(stage, "install.sh")
		if err := flush(); err != nil {
			_ = os.RemoveAll(stage)
			return err
		}
		stageRaw, _ := json.Marshal(installStageRecord{Path: stage, FromVersion: fromVer, Version: rec.Version})
		if err := os.WriteFile(stageRecordPath(dataDir), stageRaw, 0o600); err != nil {
			_ = os.RemoveAll(stage)
			return err
		}

		cmd := exec.Command("/bin/sh", scriptPath, bundle, stage, fmt.Sprintf("%d", os.Getpid()), rec.Version, "1")
		closeLog, _ := spawn(cmd)
		defer closeLog()
		if err := startInstallerOnQuit(cmd, quitApp); err != nil {
			_ = os.RemoveAll(stage)
			_ = os.Remove(stageRecordPath(dataDir))
			return err
		}
		return nil

	case "linux":
		target := strings.TrimSpace(os.Getenv("APPIMAGE"))
		if target == "" || !strings.HasSuffix(strings.ToLower(rec.ArchivePath), ".appimage") {
			return errors.New(tr("请打开已下载的安装包完成更新", "Please open the downloaded package to complete installation"))
		}
		if err := os.Chmod(rec.ArchivePath, 0o755); err != nil {
			logError("install.chmod_failed", "err", err)
			return err
		}
		stage, err := os.MkdirTemp("", "cooool-mark-appimage-update-*")
		if err != nil {
			return err
		}
		scriptPath := filepath.Join(stage, "install.sh")
		if err := os.WriteFile(scriptPath, []byte(linuxAppImageInstallerScript), 0o700); err != nil {
			_ = os.RemoveAll(stage)
			return err
		}
		if err := flush(); err != nil {
			_ = os.RemoveAll(stage)
			return err
		}
		cmd := exec.Command("/bin/sh", scriptPath, target, rec.ArchivePath, fmt.Sprintf("%d", os.Getpid()))
		closeLog, _ := spawn(cmd)
		defer closeLog()
		if err := startInstallerOnQuit(cmd, quitApp); err != nil {
			_ = os.RemoveAll(stage)
			return err
		}
		return nil

	case "windows":
		if !strings.HasSuffix(strings.ToLower(rec.ArchivePath), ".exe") {
			return errors.New("invalid Windows installer")
		}
		if err := flush(); err != nil {
			return err
		}
		if err := launchWindowsElevatedInstaller(rec.ArchivePath); err != nil {
			logError("install.windows_launch_failed", "err", err)
			return err
		}
		logInfo("install.windows_installer_launched", "path", rec.ArchivePath)
		return quitApp(func() error { return nil })
	}
	return errors.New("unsupported platform for automatic install")
}

// stageMacUpdate unpacks the verified DMG into a hidden stage directory next to the installed
// bundle and writes the swap helper; it returns the installed bundle and the stage path.
func stageMacUpdate(rec *VerifiedRecord, tr func(zh, en string) string) (string, string, error) {
	bundle, err := installedMacBundle()
	if err != nil {
		logError("install.not_app_bundle", "err", err)
		return "", "", errors.New(tr("仅打包后的 "+macAppName+".app 支持原位重启更新", "Updates can only be installed from a packaged "+macAppName+".app"))
	}
	if strings.Contains(bundle, "/AppTranslocation/") || strings.HasPrefix(bundle, "/Volumes/") {
		logError("install.readonly_location", "bundle", bundle)
		return "", "", errors.New(tr("请先将 "+macAppName+" 移入「应用程序」文件夹后再执行更新", "Move "+macAppName+".app to /Applications before installing updates"))
	}
	parent := filepath.Dir(bundle)
	probe, err := os.CreateTemp(parent, ".cooool-mark-writecheck-*")
	if err != nil {
		logError("install.permission_denied", "parent", parent, "err", err)
		return "", "", fmt.Errorf(tr("应用所在目录无写入权限：%s", "No write permission in %s"), parent)
	}
	probeName := probe.Name()
	_ = probe.Close()
	_ = os.Remove(probeName)

	unpackedDir, err := os.MkdirTemp(filepath.Dir(rec.ArchivePath), "unpacked-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(unpackedDir)
	appPath, err := prepareMacAppFromDMG(rec.ArchivePath, unpackedDir, rec.Version)
	if err != nil {
		return "", "", err
	}

	stage, err := os.MkdirTemp(parent, stageDirPrefix)
	if err != nil {
		logError("install.stage_mkdir_failed", "err", err)
		return "", "", err
	}
	markMacNeverIndex(stage)
	nextApp := filepath.Join(stage, "next.bundle")
	if out, err := exec.Command("/usr/bin/ditto", appPath, nextApp).CombinedOutput(); err != nil {
		_ = os.RemoveAll(stage)
		logError("install.stage_ditto_failed", "err", strings.TrimSpace(string(out)))
		return "", "", fmt.Errorf("stage update failed: %s", strings.TrimSpace(string(out)))
	}
	if err := validateMacBundle(nextApp, rec.Version); err != nil {
		_ = os.RemoveAll(stage)
		logError("install.stage_validate_failed", "err", err)
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(stage, "install.sh"), []byte(macSwapInstallerScript), 0o700); err != nil {
		_ = os.RemoveAll(stage)
		logError("install.write_script_failed", "err", err)
		return "", "", err
	}
	return bundle, stage, nil
}

// QuitStage is a fully verified macOS update staged in the background, ready to be swapped in
// silently after a normal quit without relaunching the app.
type QuitStage struct {
	Version string
	Bundle  string
	Path    string
}

// PrepareQuitInstall verifies the ready package and stages it so a later normal quit only has
// to spawn the swap helper.
func PrepareQuitInstall(dataDir string, rec *VerifiedRecord) (*QuitStage, error) {
	if runtime.GOOS != "darwin" || rec == nil {
		return nil, errors.New("quit install is only supported on macOS")
	}
	if err := validateReadyFromOfficialSource(dataDir, CurrentVersion(), rec); err != nil {
		return nil, err
	}
	bundle, stage, err := stageMacUpdate(rec, func(_, en string) string { return en })
	if err != nil {
		return nil, err
	}
	logInfo("quit_install.prepared", "version", rec.Version, "stage", stage)
	return &QuitStage{Version: rec.Version, Bundle: bundle, Path: stage}, nil
}

// StartQuitInstall spawns the detached swap helper in no-relaunch mode; it must be called while
// the app is quitting so the helper can replace the bundle once this process exits.
func StartQuitInstall(dataDir string, st *QuitStage) error {
	if st == nil {
		return errors.New("no quit stage")
	}
	if _, err := os.Stat(filepath.Join(st.Path, "next.bundle")); err != nil {
		return err
	}
	installLogPath := InstallLogFilePath(dataDir)
	_ = os.MkdirAll(filepath.Dir(installLogPath), 0o755)
	cmd := exec.Command("/bin/sh", filepath.Join(st.Path, "install.sh"), st.Bundle, st.Path, fmt.Sprintf("%d", os.Getpid()), st.Version, "0")
	configureDetachedProcess(cmd)
	if logFile, _ := os.OpenFile(installLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer logFile.Close()
	}
	if err := cmd.Start(); err != nil {
		logError("quit_install.spawn_failed", "err", err)
		return err
	}
	logInfo("quit_install.helper_spawned", "version", st.Version, "helper_pid", cmd.Process.Pid)
	_ = cmd.Process.Release()
	return nil
}

// DiscardQuitStage removes a background stage that is no longer going to be used.
func DiscardQuitStage(st *QuitStage) {
	if st == nil || !stageDirPattern.MatchString(filepath.Base(st.Path)) {
		return
	}
	_ = os.RemoveAll(st.Path)
}

// pruneOrphanStages removes stale stage directories beside the installed bundle that no pending
// install refers to, e.g. a background quit stage left behind by a crash or a session end.
func pruneOrphanStages(bundle, keep string) {
	parent := filepath.Dir(bundle)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, ent := range entries {
		if !ent.IsDir() || !stageDirPattern.MatchString(ent.Name()) {
			continue
		}
		path := filepath.Join(parent, ent.Name())
		if path == keep {
			continue
		}
		info, err := ent.Info()
		if err != nil || time.Since(info.ModTime()) < 10*time.Minute {
			continue
		}
		_ = os.RemoveAll(path)
		logInfo("startup.pruned_orphan_stage", "stage", path)
	}
}

func startInstallerOnQuit(cmd *exec.Cmd, quitApp func(func() error) error) error {
	return quitApp(func() error {
		if err := cmd.Start(); err != nil {
			logError("install.spawn_helper_failed", "err", err)
			return err
		}
		logInfo("install.helper_spawned", "helper_pid", cmd.Process.Pid)
		_ = cmd.Process.Release()
		return nil
	})
}
