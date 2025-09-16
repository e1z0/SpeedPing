//go:build !windows
// +build !windows

package traceroute_wrapper

import "os/exec"

func applyNoWindow(cmd *exec.Cmd) {
	// no-op on non-Windows
}
