package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

const GitHubRepoURL = "https://github.com/eachann1024/cooool-mark"

type Status string

const (
	StatusIdle        Status = "idle"
	StatusChecking    Status = "checking"
	StatusUpToDate    Status = "up-to-date"
	StatusAvailable   Status = "available"
	StatusDownloading Status = "downloading"
	StatusReady       Status = "ready"
	StatusInstalling  Status = "installing"
	StatusError       Status = "error"
)

const mockUpdateEnv = "COOOOL_MOCK_UPDATE"

type Snapshot struct {
	Status          Status
	CurrentVersion  string
	LatestVersion   string
	ReleaseURL      string
	SourceMode      SourceMode
	ActiveChannel   string
	SpeedMBps       float64
	DownloadedBytes int64
	TotalBytes      int64
	Percent         int
	Preparing       bool
	InPlace         bool
	PackageManaged  bool
	ErrorMessage    string
	Notes           AggregatedNotes
}

func (s Snapshot) HasActionableUpdate() bool {
	switch s.Status {
	case StatusAvailable, StatusDownloading, StatusReady, StatusInstalling:
		return CompareVersion(s.LatestVersion, s.CurrentVersion) > 0
	default:
		return false
	}
}

type Config struct {
	DataDir             string
	Version             string
	InitialSourceMode   SourceMode
	IsChinese           func() bool
	OnSourceModeChanged func(SourceMode)
	FlushBeforeUpdate   func() error
	QuitApp             func(startInstaller func() error) error
	OnChange            func()
	OnCheckResult       func(manual bool, snap Snapshot, err error)
}

type Service struct {
	cfg Config

	mu              sync.Mutex
	status          Status
	sourceMode      SourceMode
	activeChannel   string
	speedMBps       float64
	notes           AggregatedNotes
	latestEntry     *ReleaseEntry
	targetAsset     *ReleaseAsset
	readyRecord     *VerifiedRecord
	inPlace         bool
	pkgManaged      bool
	downloadedBytes int64
	totalBytes      int64
	percent         int
	preparing       bool
	errorMsg        string
	checkGen        uint64
	checkRunning    bool
	downloadGen     uint64
	downloadRunning bool
	downloadCancel  context.CancelFunc
	installRunning  bool
	autoStarted     bool
	quitStage       *QuitStage
	quitPreparing   bool
}

func NewService(cfg Config) *Service {
	setCurrentVersion(cfg.Version)
	if cfg.DataDir != "" {
		setLogDataDir(cfg.DataDir)
	}
	cur := CurrentVersion()
	s := &Service{
		cfg:        cfg,
		status:     StatusIdle,
		sourceMode: NormalizeSourceMode(string(cfg.InitialSourceMode)),
		notes:      AggregatedNotes{FromVersion: cur, ToVersion: cur},
	}
	if os.Getenv(mockUpdateEnv) == "1" {
		s.installMockUpdate()
	}
	return s
}

func (s *Service) isChinese() bool {
	return s.cfg.IsChinese != nil && s.cfg.IsChinese()
}

func (s *Service) notify() {
	if s.cfg.OnChange != nil {
		s.cfg.OnChange()
	}
	s.maybePrepareQuitStage()
}

// maybePrepareQuitStage stages a ready macOS update in the background so a normal quit can
// swap it in silently; the next manual launch then runs the new version.
func (s *Service) maybePrepareQuitStage() {
	if runtime.GOOS != "darwin" || os.Getenv(mockUpdateEnv) == "1" {
		return
	}
	s.mu.Lock()
	if s.quitPreparing || s.installRunning || s.status != StatusReady || s.readyRecord == nil || !s.readyRecord.InPlace ||
		s.cfg.DataDir == "" || (s.quitStage != nil && s.quitStage.Version == s.readyRecord.Version) {
		s.mu.Unlock()
		return
	}
	rec := *s.readyRecord
	stale := s.quitStage
	s.quitStage = nil
	s.quitPreparing = true
	dataDir := s.cfg.DataDir
	s.mu.Unlock()

	go func() {
		DiscardQuitStage(stale)
		st, err := PrepareQuitInstall(dataDir, &rec)
		if err != nil {
			logWarn("quit_install.prepare_failed", "version", rec.Version, "err", err)
		}
		s.mu.Lock()
		s.quitPreparing = false
		keep := st != nil && !s.installRunning && s.status == StatusReady && s.readyRecord != nil && s.readyRecord.Version == st.Version
		if keep {
			s.quitStage = st
		}
		s.mu.Unlock()
		if !keep {
			DiscardQuitStage(st)
		}
	}()
}

// ApplyOnQuit hands a background-staged update to the swap helper during a normal quit. The
// helper replaces the bundle after this process exits and does not relaunch the app.
func (s *Service) ApplyOnQuit() {
	s.mu.Lock()
	st := s.quitStage
	if s.installRunning || st == nil || s.status != StatusReady || s.readyRecord == nil || s.readyRecord.Version != st.Version {
		s.mu.Unlock()
		return
	}
	s.quitStage = nil
	s.installRunning = true
	dataDir := s.cfg.DataDir
	s.mu.Unlock()
	if err := StartQuitInstall(dataDir, st); err != nil {
		DiscardQuitStage(st)
	}
}

func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Service) snapshotLocked() Snapshot {
	cur := CurrentVersion()
	latest := s.notes.ToVersion
	if latest == "" {
		latest = cur
	}
	relURL := PublicReleasesWebURL
	if s.latestEntry != nil && s.latestEntry.HTMLURL != "" {
		relURL = s.latestEntry.HTMLURL
	}
	if s.status == StatusReady && s.readyRecord != nil {
		latest = s.readyRecord.Version
		if s.readyRecord.ReleaseURL != "" {
			relURL = s.readyRecord.ReleaseURL
		}
	}
	return Snapshot{
		Status:          s.status,
		CurrentVersion:  cur,
		LatestVersion:   latest,
		ReleaseURL:      relURL,
		SourceMode:      s.sourceMode,
		ActiveChannel:   s.activeChannel,
		SpeedMBps:       s.speedMBps,
		DownloadedBytes: s.downloadedBytes,
		TotalBytes:      s.totalBytes,
		Percent:         s.percent,
		Preparing:       s.preparing,
		InPlace:         s.inPlace,
		PackageManaged:  s.pkgManaged,
		ErrorMessage:    s.errorMsg,
		Notes:           s.notes,
	}
}

func (s *Service) SetSourceMode(mode SourceMode) {
	norm := NormalizeSourceMode(string(mode))
	s.mu.Lock()
	changed := s.sourceMode != norm
	s.sourceMode = norm
	s.mu.Unlock()
	if !changed {
		return
	}
	logInfo("config.source_mode_changed", "mode", norm)
	if s.cfg.OnSourceModeChanged != nil {
		s.cfg.OnSourceModeChanged(norm)
	}
	s.notify()
}

func (s *Service) CancelDownload() {
	s.mu.Lock()
	cancel := s.downloadCancel
	s.downloadCancel = nil
	if !s.downloadRunning {
		s.mu.Unlock()
		return
	}
	s.downloadGen++
	s.downloadRunning = false
	s.preparing = false
	s.speedMBps = 0
	s.percent = 0
	s.downloadedBytes = 0
	if s.latestEntry != nil {
		s.status = StatusAvailable
	} else {
		s.status = StatusIdle
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.notify()
}

func (s *Service) StartAutomatic() {
	s.mu.Lock()
	if s.autoStarted {
		s.mu.Unlock()
		return
	}
	s.autoStarted = true
	dataDir := s.cfg.DataDir
	s.mu.Unlock()

	go func() {
		cur := CurrentVersion()
		if dataDir != "" {
			AcknowledgeStartupAndClean(dataDir, cur)
			if rec, ok := RestoreReady(dataDir, cur); ok {
				s.mu.Lock()
				s.readyRecord = rec
				s.status = StatusReady
				s.inPlace = rec.InPlace
				s.totalBytes = rec.Size
				s.downloadedBytes = rec.Size
				s.percent = 100
				s.activeChannel = rec.Channel
				if CompareVersion(s.notes.ToVersion, rec.Version) < 0 {
					s.notes.ToVersion = rec.Version
					s.notes.HasNewer = true
				}
				s.mu.Unlock()
				s.notify()
			}
		}

		time.Sleep(12 * time.Second)
		s.runScheduledCycle()
		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.runScheduledCycle()
		}
	}()
}

func (s *Service) runScheduledCycle() {
	s.checkAsync(false, func() {
		s.mu.Lock()
		autoDownload := s.status == StatusAvailable && s.inPlace && s.targetAsset != nil && !s.downloadRunning
		s.mu.Unlock()
		if autoDownload {
			s.StartDownload()
		}
	})
}

func (s *Service) CheckAsync(manual bool) {
	s.checkAsync(manual, nil)
}

func (s *Service) report(manual bool, err error) {
	if s.cfg.OnCheckResult != nil {
		s.cfg.OnCheckResult(manual, s.Snapshot(), err)
	}
}

func (s *Service) checkAsync(manual bool, onDone func()) {
	if os.Getenv(mockUpdateEnv) == "1" {
		s.installMockUpdate()
		s.notify()
		s.report(manual, nil)
		if onDone != nil {
			onDone()
		}
		return
	}

	s.mu.Lock()
	if s.downloadRunning || s.installRunning || (s.checkRunning && !manual) {
		s.mu.Unlock()
		return
	}
	s.checkGen++
	gen := s.checkGen
	s.checkRunning = true
	wasReady := s.status == StatusReady && s.readyRecord != nil
	if !wasReady {
		s.status = StatusChecking
		s.errorMsg = ""
	}
	s.mu.Unlock()
	s.notify()

	go func() {
		defer func() {
			if onDone != nil {
				onDone()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		entries, channel, latencyMs, err := FetchReleases(ctx)

		s.mu.Lock()
		if gen != s.checkGen {
			s.mu.Unlock()
			return
		}
		s.checkRunning = false
		if s.downloadRunning || s.installRunning {
			s.mu.Unlock()
			return
		}

		if err != nil {
			if !wasReady {
				s.status = StatusError
				s.errorMsg = err.Error()
			}
			s.mu.Unlock()
			s.notify()
			s.report(manual, err)
			return
		}

		cur := CurrentVersion()
		s.activeChannel = channel
		s.notes = AggregateReleases(entries, cur)
		s.errorMsg = ""
		logInfo("check.summary", "manual", manual, "current", cur, "latest", s.notes.ToVersion, "has_newer", s.notes.HasNewer, "channel", channel, "latency_ms", latencyMs)

		if !s.notes.HasNewer || len(s.notes.Versions) == 0 {
			if s.readyRecord != nil && CompareVersion(s.readyRecord.Version, cur) > 0 {
				s.status = StatusReady
			} else {
				s.latestEntry = nil
				s.targetAsset = nil
				s.readyRecord = nil
				s.status = StatusUpToDate
			}
			s.mu.Unlock()
			s.notify()
			s.report(manual, nil)
			return
		}

		var picked *ReleaseEntry
		var asset *ReleaseAsset
		var inPlace bool
		_, _, pkgManaged := PickPlatformAsset(s.notes.Versions[0].Assets, runtime.GOOS, runtime.GOARCH)
		for i := range s.notes.Versions {
			cand := s.notes.Versions[i]
			found, ip, pm := PickPlatformAsset(cand.Assets, runtime.GOOS, runtime.GOARCH)
			if pm {
				pkgManaged = true
			}
			if found != nil {
				picked = &cand
				asset = found
				inPlace = ip
				break
			}
		}
		if picked == nil {
			latest := s.notes.Versions[0]
			picked = &latest
		}
		s.latestEntry = picked
		s.targetAsset = asset
		s.inPlace = inPlace
		s.pkgManaged = pkgManaged

		switch {
		case s.readyRecord != nil && asset != nil && inPlace && s.readyRecord.Version == picked.Version &&
			s.readyRecord.Tag == picked.Tag && s.readyRecord.AssetName == asset.Name &&
			s.readyRecord.DownloadURL == asset.DownloadURL && s.readyRecord.SHA256 == asset.EffectiveSHA256():
			s.status = StatusReady
		case s.readyRecord != nil && CompareVersion(s.readyRecord.Version, cur) > 0 &&
			(asset == nil || CompareVersion(s.readyRecord.Version, picked.Version) > 0):
			s.status = StatusReady
		default:
			s.readyRecord = nil
			s.status = StatusAvailable
			if asset != nil {
				s.totalBytes = asset.Size
			}
		}
		s.mu.Unlock()
		s.notify()
		s.report(manual, nil)
	}()
}

func (s *Service) StartDownload() {
	if os.Getenv(mockUpdateEnv) == "1" {
		s.runMockDownload()
		return
	}

	s.mu.Lock()
	if s.downloadRunning || s.installRunning || s.status == StatusReady || s.latestEntry == nil || s.cfg.DataDir == "" {
		s.mu.Unlock()
		return
	}
	if s.targetAsset == nil {
		version := s.latestEntry.Version
		s.status = StatusError
		if s.isChinese() {
			s.errorMsg = "未找到适配当前系统架构的安装包，请前往下载页手动升级"
		} else {
			s.errorMsg = "No compatible installer found for this platform; open Releases to update manually"
		}
		s.mu.Unlock()
		logError("download.missing_platform_asset", "version", version, "os", runtime.GOOS, "arch", runtime.GOARCH)
		s.notify()
		return
	}
	entry := *s.latestEntry
	asset := *s.targetAsset
	inPlace := s.inPlace
	mode := s.sourceMode
	dataDir := s.cfg.DataDir

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	s.downloadGen++
	dlGen := s.downloadGen
	s.downloadRunning = true
	s.downloadCancel = cancel
	s.status = StatusDownloading
	s.downloadedBytes = 0
	s.totalBytes = asset.Size
	s.speedMBps = 0
	s.percent = 0
	s.preparing = false
	s.errorMsg = ""
	s.mu.Unlock()
	s.notify()

	go func() {
		defer cancel()

		rec, err := DownloadAndStage(ctx, dataDir, entry, asset, inPlace, mode, func(downloaded, total int64, speed float64, channel string) {
			s.mu.Lock()
			if !s.downloadRunning || dlGen != s.downloadGen {
				s.mu.Unlock()
				return
			}
			s.downloadedBytes = downloaded
			if total > 0 {
				s.totalBytes = total
			}
			s.speedMBps = speed
			if s.totalBytes > 0 {
				s.percent = int(min(99, max(0, downloaded*100/s.totalBytes)))
			}
			s.preparing = s.totalBytes > 0 && downloaded >= s.totalBytes
			if channel != "" {
				s.activeChannel = channel
			}
			s.mu.Unlock()
			s.notify()
		}, func(commit func() error) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.downloadRunning || dlGen != s.downloadGen {
				return context.Canceled
			}
			return commit()
		})

		s.mu.Lock()
		if dlGen != s.downloadGen {
			s.mu.Unlock()
			return
		}
		if !s.downloadRunning && errors.Is(err, context.Canceled) {
			s.mu.Unlock()
			return
		}
		s.downloadRunning = false
		s.downloadCancel = nil
		s.preparing = false
		s.speedMBps = 0
		if err != nil {
			if errors.Is(err, context.Canceled) {
				s.status = StatusAvailable
			} else {
				s.status = StatusError
				s.errorMsg = err.Error()
			}
			s.mu.Unlock()
			s.notify()
			return
		}
		s.activeChannel = rec.Channel
		s.percent = 100
		if rec.Size > 0 {
			s.downloadedBytes = rec.Size
			s.totalBytes = rec.Size
		}
		if inPlace {
			s.readyRecord = rec
			s.status = StatusReady
		} else {
			s.status = StatusAvailable
		}
		s.mu.Unlock()
		s.notify()
	}()
}

func (s *Service) InstallNow() {
	s.mu.Lock()
	if s.installRunning || s.status != StatusReady || s.readyRecord == nil {
		s.mu.Unlock()
		return
	}
	rec := *s.readyRecord
	dataDir := s.cfg.DataDir
	s.installRunning = true
	s.status = StatusInstalling
	s.errorMsg = ""
	staged := s.quitStage
	s.quitStage = nil
	s.mu.Unlock()
	s.notify()

	go func() {
		DiscardQuitStage(staged)
		if os.Getenv(mockUpdateEnv) == "1" {
			time.Sleep(600 * time.Millisecond)
			s.mu.Lock()
			s.installRunning = false
			s.status = StatusReady
			s.mu.Unlock()
			s.notify()
			return
		}
		err := InstallReady(dataDir, &rec, s.isChinese(), func() error {
			if s.cfg.FlushBeforeUpdate != nil {
				return s.cfg.FlushBeforeUpdate()
			}
			return nil
		}, s.cfg.QuitApp)
		if err == nil {
			return
		}
		s.mu.Lock()
		s.installRunning = false
		if _, statErr := os.Stat(rec.ArchivePath); statErr == nil && !errors.Is(err, errStagedIntegrity) {
			s.status = StatusReady
		} else {
			_ = os.Remove(readyRecordPath(dataDir))
			s.readyRecord = nil
			s.status = StatusError
		}
		s.errorMsg = err.Error()
		s.mu.Unlock()
		s.notify()
	}()
}

func (s *Service) installMockUpdate() {
	cur := CurrentVersion()
	_, _, sv, ok := ParseReleaseTag(cur)
	if !ok {
		sv = Semver{Major: 1}
	}
	nextVer := fmt.Sprintf("%d.%d.%d", sv.Major, sv.Minor, sv.Patch+2)
	prevVer := fmt.Sprintf("%d.%d.%d", sv.Major, sv.Minor, sv.Patch+1)
	latest := ReleaseEntry{
		Version:     nextVer,
		Tag:         "v" + nextVer,
		PublishedAt: "2026-11-21T12:00:00Z",
		HTMLURL:     PublicReleasesWebURL,
		Zh: LocalizedNotes{
			New:          []string{"应用内一键更新。"},
			Improvements: []string{"后台预下载并校验 SHA-256。"},
		},
		En: LocalizedNotes{
			New:          []string{"One-click in-app updates."},
			Improvements: []string{"Background pre-download with SHA-256 verification."},
		},
	}
	previous := ReleaseEntry{
		Version:     prevVer,
		Tag:         "v" + prevVer,
		PublishedAt: "2026-11-20T09:30:00Z",
		HTMLURL:     PublicReleasesWebURL,
		Zh:          LocalizedNotes{Fixes: []string{"修复若干界面问题。"}},
		En:          LocalizedNotes{Fixes: []string{"Fixed several UI issues."}},
	}
	asset := ReleaseAsset{
		Name:        "cooool-mark-macos-arm64.dmg",
		DownloadURL: fmt.Sprintf("https://github.com/%s/releases/download/%s/cooool-mark-macos-arm64.dmg", PublicRepo, latest.Tag),
		Size:        13_421_772,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.downloadRunning || s.installRunning || s.status == StatusReady {
		return
	}
	s.notes = AggregateReleases([]ReleaseEntry{latest, previous}, cur)
	s.latestEntry = &latest
	s.targetAsset = &asset
	s.inPlace = true
	s.activeChannel = "mirror"
	s.totalBytes = asset.Size
	s.status = StatusAvailable
}

func (s *Service) runMockDownload() {
	s.mu.Lock()
	if s.downloadRunning || s.installRunning || s.status == StatusReady || s.latestEntry == nil {
		s.mu.Unlock()
		return
	}
	s.downloadGen++
	dlGen := s.downloadGen
	s.downloadRunning = true
	s.status = StatusDownloading
	s.preparing = false
	total := int64(13_421_772)
	s.totalBytes = total
	s.downloadedBytes = 0
	s.speedMBps = 4.2
	s.percent = 0
	s.mu.Unlock()
	s.notify()

	go func() {
		for step := 1; step <= 9; step++ {
			time.Sleep(160 * time.Millisecond)
			s.mu.Lock()
			if !s.downloadRunning || dlGen != s.downloadGen {
				s.mu.Unlock()
				return
			}
			s.downloadedBytes = total * int64(step) / 10
			s.percent = step * 10
			s.mu.Unlock()
			s.notify()
		}
		time.Sleep(160 * time.Millisecond)
		s.mu.Lock()
		if !s.downloadRunning || dlGen != s.downloadGen {
			s.mu.Unlock()
			return
		}
		s.downloadedBytes = total
		s.percent = 100
		s.preparing = true
		s.mu.Unlock()
		s.notify()

		time.Sleep(360 * time.Millisecond)
		s.mu.Lock()
		if !s.downloadRunning || dlGen != s.downloadGen {
			s.mu.Unlock()
			return
		}
		s.downloadRunning = false
		s.preparing = false
		s.speedMBps = 0
		s.readyRecord = &VerifiedRecord{Version: s.latestEntry.Version, InPlace: true}
		s.status = StatusReady
		s.mu.Unlock()
		s.notify()
	}()
}
