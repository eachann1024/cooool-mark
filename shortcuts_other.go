//go:build !linux || android || (!amd64 && !arm64)

package main

func hasLinuxShortcutPortal() bool { return false }
