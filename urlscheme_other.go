//go:build !windows

package main

func hasInstalledWindowsScheme() bool { return false }
func cleanupStaleWindowsHKCUScheme()  {}
