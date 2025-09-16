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
	"sync"
	"time"
)

type SampleState int

const (
	SampleOK   SampleState = iota
	SampleLoss             // timeout -> loss
	SampleLate             // arrived in grace window after timeout
)

const (
	// Default number of samples retained per host (roughly ~10 minutes at 1s).
	DefaultRingCap = 600
)

type Sample struct {
	T time.Time
	// ms latency; negative for loss/timeouts
	MS    float64
	Seq   int         // ICMP sequence number
	State SampleState // OK, Loss, Late
}

type Ring struct {
	mu    sync.RWMutex
	data  []Sample
	head  int
	count int
}

func NewRing(capacity int) *Ring {
	return &Ring{data: make([]Sample, capacity)}
}

func (r *Ring) Push(s Sample) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) == 0 {
		return -1
	}
	idx := r.head
	r.data[idx] = s
	r.head = (r.head + 1) % len(r.data)
	if r.count < len(r.data) {
		r.count++
	}
	return idx
}

func (r *Ring) UpdateAt(idx int, update func(*Sample)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count == 0 || idx < 0 || idx >= len(r.data) {
		return
	}
	update(&r.data[idx])
}

func (r *Ring) Snapshot(dst []Sample) []Sample {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.count == 0 {
		return dst[:0]
	}
	n := r.count
	if cap(dst) < n {
		dst = make([]Sample, n)
	} else {
		dst = dst[:n]
	}
	start := (r.head - r.count + len(r.data)) % len(r.data)
	for i := 0; i < n; i++ {
		dst[i] = r.data[(start+i)%len(r.data)]
	}
	return dst
}

type HostState int

const (
	HostStopped HostState = iota
	HostRunning
)

type Host struct {
	Name   string // display name
	Addr   string // ip or hostname
	ColorI int    // color index (we’ll let Qt pick default pen colors per index)
	State  HostState

	buf *Ring
}

type AppModel struct {
	mu             sync.RWMutex
	hosts          []*Host
	pingIntervalMs int // current ping interval (ms)
	cfg            *AppConfig
	saveQ          DebouncedSaver
}

func NewAppModel() *AppModel {
	return &AppModel{hosts: make([]*Host, 0, 8), pingIntervalMs: 1000}
}

func (m *AppModel) Config() *AppConfig { return m.cfg }

// Call on startup
func (m *AppModel) LoadFromConfig(cfg *AppConfig) {
	if cfg == nil {
		cfg = defaultConfig()
	}
	m.cfg = cfg

	// Ping interval
	if cfg.Ping.IntervalMs > 0 {
		m.SetPingIntervalMs(cfg.Ping.IntervalMs)
	}

	// Hosts
	m.ClearHosts()
	for _, h := range cfg.Ping.Hosts {
		if h.Enabled {
			m.AddHost(h.Name, h.Addr, DefaultRingCap)
		}
	}
}

// Collect current state → Config (called before save/exit)
func (m *AppModel) SnapshotConfig(winGeom WindowConfig) *AppConfig {
	if m.cfg == nil {
		m.cfg = defaultConfig()
	}
	// hosts snapshot
	var hosts []HostConfig
	for _, h := range m.Hosts() {
		hosts = append(hosts, HostConfig{Name: h.Name, Addr: h.Addr, Enabled: true})
	}

	m.cfg.Ping.IntervalMs = m.PingIntervalMs()
	m.cfg.Ping.Hosts = hosts
	m.cfg.Window = winGeom
	return m.cfg
}

// Save (debounced)
func (m *AppModel) SaveConfigAsync() {
	m.saveQ.Trigger(400*time.Millisecond, func() {
		_ = SaveConfig(m.SnapshotConfig(WindowConfig{})) // window filled on close
	})
}

func (m *AppModel) Hosts() []*Host {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Host, len(m.hosts))
	copy(out, m.hosts)
	return out
}

func (m *AppModel) AddHost(name, addr string, ringCap int) *Host {
	h := &Host{
		Name:  name,
		Addr:  addr,
		State: HostStopped,
		buf:   NewRing(ringCap),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	h.ColorI = len(m.hosts)
	m.hosts = append(m.hosts, h)
	return h
}

func (m *AppModel) RemoveHostAt(idx int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idx < 0 || idx >= len(m.hosts) {
		return false
	}
	// compact slice
	m.hosts = append(m.hosts[:idx], m.hosts[idx+1:]...)
	// reassign ColorI for stable legend order/colors
	for i := range m.hosts {
		m.hosts[i].ColorI = i
	}
	return true
}

func (m *AppModel) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.hosts)
}

// -------- ping interval --------

func (m *AppModel) SetPingIntervalMs(v int) {
	if v <= 0 {
		v = 1000
	}
	m.mu.Lock()
	m.pingIntervalMs = v
	m.mu.Unlock()
}

func (m *AppModel) PingIntervalMs() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pingIntervalMs
}

// -------- hosts CRUD --------

// Add with explicit ring capacity
func (m *AppModel) AddHostWithCap(name, addr string, ringCap int) *Host {
	if ringCap <= 0 {
		ringCap = DefaultRingCap
	}
	h := &Host{
		Name:  name,
		Addr:  addr,
		State: HostStopped,
		buf:   NewRing(ringCap),
	}
	m.mu.Lock()
	h.ColorI = len(m.hosts)
	m.hosts = append(m.hosts, h)
	m.mu.Unlock()
	return h
}

func (m *AppModel) ClearHosts() {
	m.mu.Lock()
	m.hosts = m.hosts[:0]
	m.mu.Unlock()
}
