//go:build !windows
// +build !windows

package iperf

import "os/exec"

func applyNoWindow(cmd *exec.Cmd) {
	// no-op on non-Windows
}

