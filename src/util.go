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
package main

import (
	"log"
	"math"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// open file (default association) or folder (supports windows, linux, mac)
func openFileOrDir(file string) {
	log.Printf("Opening external: %s\n", file)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", file)
	case "linux":
		cmd = exec.Command("xdg-open", file)
	case "windows":
		cmd = exec.Command("explorer", file)
	default:
		return
	}
	_ = cmd.Start()
}

func atoiDefault(s string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v < 0 {
		return def
	}
	return v
}

// atofDefault parses s as float64, returning def on error.
// It also accepts commas as decimal separators (e.g., "1,5").
func atofDefault(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	// allow European decimal comma
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return def
	}
	return v
}
