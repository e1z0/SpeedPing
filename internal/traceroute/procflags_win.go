//go:build windows
// +build windows

package traceroute_wrapper

import (
	"os/exec"
	"syscall"
)

func applyNoWindow(cmd *exec.Cmd) {
	// Hide the console window for console subsystem children (tracert.exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
}
