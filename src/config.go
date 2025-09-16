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
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

var env Environment
var globalConfig AppConfig

type Environment struct {
	configDir    string // configuration directory ~/.config/speedping
	settingsFile string // configuration path ~/.config/speedping/settings.yml
	homeDir      string // home directory ~/
	appPath      string // application directory where the binary lies
	tmpDir       string // OS Temp directory
	appDebugLog  string // app debug.log
	os           string // current operating system
}

type HostConfig struct {
	Name    string `yaml:"name"`
	Addr    string `yaml:"addr"`
	Enabled bool   `yaml:"enabled"`
}

type PingConfig struct {
	IntervalMs int          `yaml:"interval_ms"`
	Hosts      []HostConfig `yaml:"hosts"`
}

type SpeedConfig struct {
	Server      string `yaml:"server"`
	Port        int    `yaml:"port"`
	DurationSec int    `yaml:"duration_sec"`
	IntervalSec int    `yaml:"interval_sec"`
	Parallel    int    `yaml:"parallel"`
	Reverse     bool   `yaml:"reverse"`
}

type WindowConfig struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
	W int `yaml:"w"`
	H int `yaml:"h"`
}

type TracerouteConfig struct {
	Target       string  `yaml:"target"`
	MaxHops      int     `yaml:"max_hops"`
	TimeoutSec   float64 `yaml:"timeout_sec"`   // per-probe timeout (s)
	Probes       int     `yaml:"probes"`        // probes per hop
	DontResolve  bool    `yaml:"dont_resolve"`  // -n behavior
	PulseSeconds float64 `yaml:"pulse_seconds"` // seconds per pulse loop in the map
}

type AppConfig struct {
	Ping   PingConfig       `yaml:"ping"`
	Speed  SpeedConfig      `yaml:"speed"`
	Trace  TracerouteConfig `yaml:"traceroute"`
	Window WindowConfig     `yaml:"window"`
}

func defaultConfig() *AppConfig {
	return &AppConfig{
		Ping: PingConfig{
			IntervalMs: 1000,
			Hosts:      nil,
		},
		Speed: SpeedConfig{
			Server:      "",
			Port:        5201,
			DurationSec: 10,
			IntervalSec: 1,
			Parallel:    1,
			Reverse:     false,
		},
		Trace: TracerouteConfig{ // <— NEW defaults
			Target:       "",
			MaxHops:      30,
			TimeoutSec:   1.0,
			Probes:       1,
			DontResolve:  false,
			PulseSeconds: 6.0, // slow, pleasant pulse
		},
	}
}

func LoadConfig() (*AppConfig, error) {
	log.Printf("Loading configuration...\n")
	b, err := os.ReadFile(env.settingsFile)
	if errors.Is(err, fs.ErrNotExist) {
		return defaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	cfg := defaultConfig()
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func SaveConfig(cfg *AppConfig) error {
	log.Printf("Saving configuration...\n")
	if err := os.MkdirAll(filepath.Dir(env.settingsFile), 0o755); err != nil {
		return err
	}

	tmp := env.settingsFile + ".tmp"
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, env.settingsFile)
}

// Small debouncer helper to avoid excessive writes.
type DebouncedSaver struct {
	timer *time.Timer
}

func (d *DebouncedSaver) Trigger(delay time.Duration, fn func()) {
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(delay, fn)
}

func InitializeEnvironment() {
	// initialize the logging
	initlog()
	// gather all required directories
	log.Printf("App Path: %s\n", appPath())
	log.Printf("Initializing environment...")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Unable to determine the user home folder: %s\n", err)
	}
	settingsFile := filepath.Join(configDir(), "settings.yml")
	environ := Environment{
		configDir:    configDir(),
		settingsFile: settingsFile,
		homeDir:      homeDir,
		appPath:      appPath(),
		tmpDir:       os.TempDir(),
		appDebugLog:  filepath.Join(logsDir(), "debug.log"),
		os:           runtime.GOOS,
	}
	env = environ
}

func initlog() {
	// create directory if it does not exist
	if _, err := os.Stat(logsDir()); os.IsNotExist(err) {
		os.MkdirAll(logsDir(), 0755)
	}

	// Open the log file
	file, err := os.OpenFile(filepath.Join(logsDir(), "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
		return
	}
	// we always write to log file; if DEBUG=true we write to stdout too)
	if debugging == "true" {
		DEBUG = true
		log.SetOutput(io.MultiWriter(file, os.Stdout))
	} else {
		log.SetOutput(io.MultiWriter(file))
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func appPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	// Resolve any symlinks and clean path
	realPath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		return ""
	}
	return filepath.Dir(realPath)
}

func exePath() string {
	p, _ := os.Executable()
	return p
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", appName)
}
func logsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", appName, "logs")
}
