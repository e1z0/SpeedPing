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
package traceroute_wrapper

import (
	"bufio"
	"context"
	"errors"
	"log"
	"math"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Options struct {
	Target      string
	MaxHops     int           // default 30
	Timeout     time.Duration // per-hop timeout
	Probes      int           // per-hop probes (1 or 3)
	DontResolve bool          // use -n / -d to avoid DNS
}

type Hop struct {
	Index int
	Addr  string
	RTTms float64 // -1 if timeout
	Raw   string  // raw line
}

type Event struct {
	Kind string // "start","hop","done","error","log"
	Msg  string
	Hop  *Hop
	Err  error
}

func Run(ctx context.Context, opt Options) (<-chan Event, error) {
	if opt.Target == "" {
		return nil, errors.New("target required")
	}
	if opt.MaxHops <= 0 {
		opt.MaxHops = 30
	}
	if opt.Timeout <= 0 {
		opt.Timeout = time.Second
	}
	if opt.Probes <= 0 {
		opt.Probes = 1
	}

	var args []string
	var bin string
	switch runtime.GOOS {
	case "windows":
		bin = "tracert"
		args = append(args, "-h", strconv.Itoa(opt.MaxHops))
		args = append(args, "-w", strconv.Itoa(int(opt.Timeout.Milliseconds())))
		if opt.DontResolve {
			args = append(args, "-d")
		}
		args = append(args, opt.Target)
	case "darwin":
		bin = "traceroute"
		if opt.DontResolve {
			args = append(args, "-n")
		}
		// macOS: -w expects integer seconds
		args = append(args, "-w", strconv.Itoa(int(math.Ceil(opt.Timeout.Seconds()))))
		args = append(args, "-q", strconv.Itoa(opt.Probes))
		args = append(args, "-m", strconv.Itoa(opt.MaxHops))
		args = append(args, opt.Target)
	default:
		bin = "traceroute"
		if opt.DontResolve {
			args = append(args, "-n")
		}
		args = append(args, "-q", strconv.Itoa(opt.Probes))
		args = append(args, "-w", strconv.FormatFloat(opt.Timeout.Seconds(), 'f', 1, 64))
		args = append(args, "-m", strconv.Itoa(opt.MaxHops))
		args = append(args, opt.Target)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	log.Printf("Executing traceroute: %s\n", cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	// hide external window
	applyNoWindow(cmd)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	events := make(chan Event, 64)

	// single close owner: close(events) after all workers exit
	var wg sync.WaitGroup
	closed := make(chan struct{}) // signals “we’re closing soon” if you ever want to guard emits

	emit := func(e Event) {
		select {
		case events <- e:
		default:
		}
	}

	emit(Event{Kind: "start", Msg: strings.Join(append([]string{bin}, args...), " ")})

	// Regexes (same as before)
	// ---- tolerant Unix + Windows parsing ----
	reFloatMS := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*ms`)
	reParenIP := regexp.MustCompile(`\(([^)]+)\)`) // (1.2.3.4)
	reTimeoutUnix := regexp.MustCompile(`^\s*(\d+)\s+(?:\*+\s*)+$`)
	reWin := regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s*ms\s+(\d+)\s*ms\s+(\d+)\s*ms\s+(\S+)`)
	reWinTimeout := regexp.MustCompile(`^\s*(\d+)\s+\*`)

	emitted := int32(0)

	// stdout reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := sc.Text()
			log.Printf("traceroute debug: %s\n", line)
			// --- Windows first (exact) ---
			if m := reWin.FindStringSubmatch(line); len(m) == 6 {
				hopIdx, _ := strconv.Atoi(m[1])
				r1, _ := strconv.ParseFloat(m[2], 64)
				r2, _ := strconv.ParseFloat(m[3], 64)
				r3, _ := strconv.ParseFloat(m[4], 64)
				addr := m[5]
				rtt := r1
				if r2 > 0 && r2 < rtt {
					rtt = r2
				}
				if r3 > 0 && r3 < rtt {
					rtt = r3
				}
				emit(Event{Kind: "hop", Hop: &Hop{Index: hopIdx, Addr: addr, RTTms: rtt, Raw: line}})
				atomic.AddInt32(&emitted, 1)
				continue
			}
			if m := reWinTimeout.FindStringSubmatch(line); len(m) == 2 {
				hopIdx, _ := strconv.Atoi(m[1])
				emit(Event{Kind: "hop", Hop: &Hop{Index: hopIdx, Addr: "*", RTTms: -1, Raw: line}})
				atomic.AddInt32(&emitted, 1)
				continue
			}

			// --- Unix/macOS tolerant parsing ---
			// Timeout line like: " 3  *"
			if m := reTimeoutUnix.FindStringSubmatch(line); len(m) == 2 {
				hopIdx, _ := strconv.Atoi(m[1])
				emit(Event{Kind: "hop", Hop: &Hop{Index: hopIdx, Addr: "*", RTTms: -1, Raw: line}})
				atomic.AddInt32(&emitted, 1)
				continue
			}

			// Must start with hop number
			// Example: " 2  host1 (192.168.255.254)  5.818 ms"
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// parse hop index (fields[0] may be hop or first token if leading space)
				hopIdx, err := strconv.Atoi(fields[0])
				if err != nil && len(fields) >= 3 {
					// Some traceroutes indent; try second token
					hopIdx, err = strconv.Atoi(fields[1])
					if err == nil {
						// shift line so downstream logic still works
					}
				}
				if err == nil {
					// Address: prefer (ip) if present
					addr := ""
					if pm := reParenIP.FindStringSubmatch(line); len(pm) == 2 {
						addr = pm[1]
					} else {
						// take the first non-numeric/non-hop token after hop #
						// e.g. "2  hostname  5.818 ms" or "2  10.0.0.1  5.8 ms"
						// split after hop number(s)
						after := line
						// crude but effective: cut off the leading hop number and following spaces
						if i := strings.Index(line, fields[0]); i >= 0 {
							after = strings.TrimSpace(line[i+len(fields[0]):])
						}
						parts := strings.Fields(after)
						if len(parts) > 0 {
							addr = parts[0]
						}
					}

					// RTT: pick minimum of all "\d+(\.\d+)? ms" found
					minRTT := math.MaxFloat64
					ms := reFloatMS.FindAllStringSubmatch(line, -1)
					for _, m := range ms {
						if v, err := strconv.ParseFloat(m[1], 64); err == nil && v >= 0 {
							if v < minRTT {
								minRTT = v
							}
						}
					}
					if minRTT == math.MaxFloat64 {
						// No RTT found → treat as timeout-ish hop, but keep addr if we got it
						emit(Event{Kind: "hop", Hop: &Hop{Index: hopIdx, Addr: firstNonEmpty(addr, "*"), RTTms: -1, Raw: line}})
					} else {
						emit(Event{Kind: "hop", Hop: &Hop{Index: hopIdx, Addr: addr, RTTms: minRTT, Raw: line}})
					}
					atomic.AddInt32(&emitted, 1)
					continue
				}
			}

			// Not a hop → log
			emit(Event{Kind: "log", Msg: line})
		}
		if err := sc.Err(); err != nil {
			emit(Event{Kind: "error", Err: err, Msg: "scan error"})
		}
	}()

	// stderr logger
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			emit(Event{Kind: "log", Msg: sc.Text()})
		}
	}()

	// wait for process
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := cmd.Wait()
		if ctx.Err() == context.Canceled {
			emit(Event{Kind: "done", Msg: "canceled"})
			return
		}
		if err != nil {
			emit(Event{Kind: "error", Err: err, Msg: "traceroute exited"})
		} else {
			emit(Event{Kind: "done", Msg: "completed"})
		}
	}()

	// close events only after all emitters are done
	go func() {
		defer close(events)
		close(closed)
		wg.Wait()
	}()

	return events, nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
