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
	"context"
	"runtime"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

type ProbingBackend struct {
	Privileged bool
	Interval   time.Duration
	MaxRTT     time.Duration
	GraceLate  time.Duration // how long after MaxRTT we still call it "late" (not loss)
}

// RunForHost binds a specific host so packet handlers can safely update its ring.
func (pb ProbingBackend) RunForHost(ctx context.Context, h *Host) error {
	if h == nil {
		return context.Canceled
	}

	pinger, err := probing.NewPinger(h.Addr)
	if err != nil {
		return err
	}

	// ---- sane defaults ----
	if pb.Interval <= 0 {
		pb.Interval = time.Second
	}
	if pb.MaxRTT <= 0 {
		pb.MaxRTT = maxDur(3*pb.Interval, 150*time.Millisecond)
	}
	if pb.GraceLate <= 0 {
		pb.GraceLate = 100 * time.Millisecond
	}

	// ---- configure pinger ----
	if runtime.GOOS == "windows" {
		// on windows it works as privileged, without need to be privileged at all :)
		pinger.SetPrivileged(true)
	} else {
		pinger.SetPrivileged(pb.Privileged)
	}

	pinger.Interval = pb.Interval
	if pinger.Interval <= 0 {
		pinger.Interval = time.Second
	}
	// Avoid 0-duration paths inside pro-bing
	pinger.Timeout = 24 * time.Hour
	pinger.RecordRtts = false
	pinger.Count = 0
	pinger.Size = 56

	type pending struct {
		timer  *time.Timer // fires at MaxRTT → insert LOSS
		idx    int         // index in ring where LOSS went
		pushed bool        // true once LOSS inserted
	}
	var (
		mu    sync.Mutex
		pends = make(map[int]*pending) // seq -> pending
	)

	// Start a per-seq timer; on fire, insert LOSS sample and remember its index
	pinger.OnSend = func(pkt *probing.Packet) {
		seq := pkt.Seq
		t := time.AfterFunc(pb.MaxRTT, func() {
			mu.Lock()
			p, ok := pends[seq]
			if !ok {
				mu.Unlock()
				return // reply already handled
			}
			p.idx = h.buf.Push(Sample{
				T:     time.Now(),
				MS:    -1,
				Seq:   seq,
				State: SampleLoss,
			})
			p.pushed = true
			mu.Unlock()
		})
		mu.Lock()
		pends[seq] = &pending{timer: t, idx: -1, pushed: false}
		mu.Unlock()
	}

	// On receive: OK if within MaxRTT, else "late" (and reconcile loss if already pushed)
	pinger.OnRecv = func(pkt *probing.Packet) {
		seq := pkt.Seq
		rtt := pkt.Rtt
		now := time.Now()

		mu.Lock()
		p, had := pends[seq]
		if had && p.timer != nil {
			p.timer.Stop()
		}
		delete(pends, seq)
		mu.Unlock()

		if rtt <= pb.MaxRTT {
			// on-time → normal point
			h.buf.Push(Sample{
				T:     now,
				MS:    float64(rtt.Microseconds()) / 1000.0,
				Seq:   seq,
				State: SampleOK,
			})
			return
		}

		// Late: within grace → if LOSS already inserted, convert it to LATE
		if had && p.pushed && rtt <= pb.MaxRTT+pb.GraceLate {
			h.buf.UpdateAt(p.idx, func(s *Sample) {
				s.State = SampleLate
				s.MS = float64(rtt.Microseconds()) / 1000.0
				s.T = now
			})
			return
		}

		// Otherwise, record as a standalone LATE marker (gap in line)
		h.buf.Push(Sample{
			T:     now,
			MS:    float64(rtt.Microseconds()) / 1000.0,
			Seq:   seq,
			State: SampleLate,
		})
	}

	// Run until cancel/error
	errCh := make(chan error, 1)
	go func() { errCh <- pinger.Run() }()

	select {
	case <-ctx.Done():
		pinger.Stop()
		mu.Lock()
		for _, p := range pends {
			if p.timer != nil {
				p.timer.Stop()
			}
		}
		pends = map[int]*pending{}
		mu.Unlock()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
