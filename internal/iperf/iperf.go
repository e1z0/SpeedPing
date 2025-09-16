/* SPDX-License-Identifier: GPL-3.0-or-later
 *
 * SpeedPing
 * Copyright (C) 2025 e1z0 <e1z0@icloud.com>
 *
 * This file is part of SpeedPing.
 *
 * SpeedPing is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * SpeedPing is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with SpeedPing.  If not, see <https://www.gnu.org/licenses/>.
 */
package iperf

// Iperf3 wrapper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Config controls the iperf3 run.
type Config struct {
	BinDir        string   // folder with binaries (e.g., "./iperf")
	Host          string   // server host/IP
	Port          int      // default 5201 if 0
	DurationSec   int      // -t seconds (default 10 if 0)
	Parallel      int      // -P streams  (default 1 if 0)
	IntervalSec   int      // -i seconds  (default 1 if 0)
	Reverse       bool     // -R (download)
	Bidirectional bool     // --bidir (upload+download simultaneously; iperf3 ≥ 3.7)
	ExtraArgs     []string // any additional raw args (optional)
	Format        string   // iperf3 --format (default "m": Mbits/sec)
}

// Interval is a parsed per-interval row.
type Interval struct {
	// Example row:
	// [  7]   0.00-1.01   sec   72.8 MBytes   607 Mbits/sec
	Raw      string // full raw line
	ID       string // stream id or "SUM"
	IsSum    bool
	StartSec float64
	EndSec   float64
	Transfer string // e.g. "72.8 MBytes"
	Bitrate  string // e.g. "607 Mbits/sec"
}

// Result is emitted after iperf exits.
type Result struct {
	ExitErr error // nil on success; iperf non-zero exit -> error
}

// Run starts iperf3 and returns:
//   - intervals: a live stream of parsed interval rows
//   - done: fires when the process exits (with any error)
//
// It prints nothing by itself; we consume the channels.
func Run(ctx context.Context, cfg Config) (<-chan Interval, <-chan Result, error) {
	if cfg.Host == "" {
		return nil, nil, errors.New("iperf: Host is required")
	}
	if cfg.BinDir == "" {
		cfg.BinDir = "iperf"
	}
	if cfg.Port == 0 {
		cfg.Port = 5201
	}
	if cfg.DurationSec == 0 {
		cfg.DurationSec = 10
	}
	if cfg.Parallel == 0 {
		cfg.Parallel = 1
	}
	if cfg.IntervalSec == 0 {
		cfg.IntervalSec = 1
	}
	if cfg.Format == "" {
		cfg.Format = "m" // Mbits/sec
	}

	bin, err := SelectBinary(cfg.BinDir)
	if err != nil {
		return nil, nil, err
	}

	args := []string{
		"-c", cfg.Host,
		"-p", fmt.Sprint(cfg.Port),
		"-t", fmt.Sprint(cfg.DurationSec),
		"-P", fmt.Sprint(cfg.Parallel),
		"-i", fmt.Sprint(cfg.IntervalSec),
		"--forceflush", // flush each interval when piping
		"--format", cfg.Format,
	}
	if cfg.Reverse {
		args = append(args, "-R")
	}
	if cfg.Bidirectional {
		args = append(args, "--bidir")
	}
	if len(cfg.ExtraArgs) > 0 {
		args = append(args, cfg.ExtraArgs...)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	log.Printf("Running iperf3: %s %s\n", bin, args)

	// Important for Windows to find cygwin1.dll when using the shipped iperf3.exe
	cmd.Dir = cfg.BinDir
	if runtime.GOOS == "windows" {
		cmd.Env = append(os.Environ(), "PATH="+cfg.BinDir+";"+os.Getenv("PATH"))
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = cmd.Stdout

	// hide external window
	applyNoWindow(cmd)

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	intervals := make(chan Interval, 128)
	done := make(chan Result, 1)

	// Regex for per-interval rows. We ignore header wording (Bitrate/Bandwidth) by matching the row itself.
	// [ ID]  start-end  sec   <Transfer Bytes>   <Rate> <bits/sec>
	re := regexp.MustCompile(`^\[\s*(\d+|SUM)\]\s+([0-9.]+)-([0-9.]+)\s+sec\s+([0-9.]+\s+[KMG]?Bytes)\s+([0-9.]+)\s+([KMG]?bits/sec)\b`)

	// Stream & parse
	go func(r io.ReadCloser) {
		defer func() {
			_ = r.Close()
			close(intervals)
		}()
		sc := bufio.NewScanner(r)
		// support long lines
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if m := re.FindStringSubmatch(line); m != nil {
				iv := Interval{
					Raw:      line,
					ID:       m[1],
					IsSum:    m[1] == "SUM",
					StartSec: mustParseFloat(m[2]),
					EndSec:   mustParseFloat(m[3]),
					Transfer: m[4],              // e.g., "72.8 MBytes"
					Bitrate:  m[5] + " " + m[6], // e.g., "607 Mbits/sec"
				}
				intervals <- iv
			}
		}
	}(stdout)

	// Waiter
	go func() {
		err := cmd.Wait()
		done <- Result{ExitErr: err}
		close(done)
	}()

	return intervals, done, nil
}

func SelectBinary(binDir string) (string, error) {
	// Explicit override
	if p := os.Getenv("SPEEDPING_IPERF"); p != "" {
		if isExecFile(p) {
			return p, nil
		}
		return "", fmt.Errorf("SPEEDPING_IPERF points to non-executable: %s", p)
	}

	// Bundled copy (same dir as the executable)
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		switch runtime.GOOS {
		case "darwin":
			if runtime.GOARCH == "arm64" {
				candidates = append(candidates,
					filepath.Join(base, "iperf", "iperf3-arm64-osx-14"),
				)
			} else {
				candidates = append(candidates,
					filepath.Join(base, "iperf", "iperf3-amd64-osx-13"),
				)
			}
		case "linux":
			if runtime.GOARCH == "amd64" {
				candidates = append(candidates,
					filepath.Join(base, "iperf", "iperf3-amd64"),
				)
			} else if runtime.GOARCH == "386" {
				candidates = append(candidates,
					filepath.Join(base, "iperf", "iperf3-i386"),
				)
			} else {
				return "", fmt.Errorf("iperf: no linux binary for GOARCH=%s", runtime.GOARCH)
			}
		case "windows":
			candidates = append(candidates,
				filepath.Join(base, "iperf", "iperf3.exe"),
			)
		default:
			return "", fmt.Errorf("iperf: unsupported OS %s", runtime.GOOS)
		}
	}

	for _, c := range candidates {
		if isExecFile(c) {
			return c, nil
		}
	}

	// PATH lookup (Linux/macOS: "iperf3", Windows: "iperf3.exe")
	name := "iperf3"
	if runtime.GOOS == "windows" {
		name = "iperf3.exe"
	}
	if p, err := exec.LookPath(name); err == nil && isExecFile(p) {
		return p, nil
	}

	return "", errors.New("iperf3 binary not found (set SPEEDPING_IPERF or install iperf3 in PATH)")
}

func mustParseFloat(s string) float64 {
	// fast path avoiding strconv import noise here; safe fallback
	var f float64
	fmt.Sscan(s, &f)
	return f
}

func isExecFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	// must be a regular file
	if !fi.Mode().IsRegular() {
		return false
	}
	// fix file permission on Unix as it requires +x to be executable
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && (info.Mode()&0111) == 0 {
		_ = os.Chmod(path, info.Mode()|0111)
	}
	// on Unix, check executable bit; on Windows this is fine for .exe
	return fi.Mode()&0111 != 0 || runtime.GOOS == "windows"
}
