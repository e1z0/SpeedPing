//go:build windows
// +build windows

package iperf

import (
	"os/exec"
	"syscall"
)

func applyNoWindow(cmd *exec.Cmd) {
	// Hide the console window for console subsystem children (iperf3.exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
}

