//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// hasInstalledWindowsScheme reports whether an HKLM installer registration for
// coooolmark:// points to an existing executable on disk. When true, a portable
// copy must not shadow it with a per-user HKCU override.
func hasInstalledWindowsScheme() bool {
	return installedWindowsSchemeExe() != ""
}

func installedWindowsSchemeExe() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `Software\Classes\coooolmark\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	cmd, _, err := k.GetStringValue("")
	if err != nil {
		return ""
	}
	exe := extractWindowsCommandExe(cmd)
	if exe == "" {
		return ""
	}
	st, err := os.Stat(exe)
	if err != nil || st.IsDir() {
		return ""
	}
	return exe
}

// cleanupStaleWindowsHKCUScheme removes a per-user HKCU coooolmark:// override
// when an official HKLM installation exists and either the HKCU target binary
// is missing or the current process is the installed executable itself.
func cleanupStaleWindowsHKCUScheme() {
	installedExe := installedWindowsSchemeExe()
	if installedExe == "" {
		return
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\coooolmark\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	cmd, _, valErr := k.GetStringValue("")
	_ = k.Close()

	shouldDelete := valErr != nil
	if !shouldDelete {
		hkcuExe := extractWindowsCommandExe(cmd)
		if hkcuExe == "" {
			shouldDelete = true
		} else if st, err := os.Stat(hkcuExe); err != nil || st.IsDir() {
			shouldDelete = true
		} else if curExe, err := os.Executable(); err == nil {
			if realCur, err := filepath.EvalSymlinks(curExe); err == nil {
				curExe = realCur
			}
			if realInst, err := filepath.EvalSymlinks(installedExe); err == nil {
				installedExe = realInst
			}
			if strings.EqualFold(filepath.Clean(curExe), filepath.Clean(installedExe)) {
				shouldDelete = true
			}
		}
	}
	if !shouldDelete {
		return
	}
	deleteRegistryTree(registry.CURRENT_USER, `Software\Classes\coooolmark`)
}

func deleteRegistryTree(root registry.Key, path string) {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err == nil {
		subkeys, _ := k.ReadSubKeyNames(-1)
		_ = k.Close()
		for _, sub := range subkeys {
			deleteRegistryTree(root, path+`\`+sub)
		}
	}
	_ = registry.DeleteKey(root, path)
}

func extractWindowsCommandExe(cmd string) string {
	s := strings.TrimSpace(cmd)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		if end := strings.Index(s[1:], `"`); end > 0 {
			return filepath.Clean(s[1 : 1+end])
		}
		return ""
	}
	if idx := strings.IndexAny(s, " \t"); idx > 0 {
		return filepath.Clean(s[:idx])
	}
	return filepath.Clean(s)
}
