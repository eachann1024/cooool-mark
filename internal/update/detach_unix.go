//go:build !windows

package update

import (
	"errors"
	"os/exec"
	"syscall"
)

func configureDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func launchWindowsElevatedInstaller(_ string) error {
	return errors.New("windows installer launch is not supported on this platform")
}
