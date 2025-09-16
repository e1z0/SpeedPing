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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/e1z0/speedping/internal/iperf"
	"github.com/mappu/miqt/qt"
)

// general ui unit

type UI struct {
	main   *qt.QMainWindow
	graph  *GraphWidget
	model  *AppModel
	cancel context.CancelFunc

	backend ProbingBackend
	running bool

	// widgets we need to toggle
	btnStart *qt.QPushButton
	btnStop  *qt.QPushButton

	hostName *qt.QLineEdit
	hostAddr *qt.QLineEdit
	btnAdd   *qt.QPushButton
	btnRem   *qt.QPushButton
	hostList *qt.QListWidget

	intSlider *qt.QSlider
	intLabel  *qt.QLabel
}

func NewUI(model *AppModel) *UI {
	ui := &UI{model: model}
	cfg := ui.model.Config()

	ui.main = qt.NewQMainWindow(nil)
	ui.main.SetWindowTitle("SpeedPing")
	ui.main.SetWindowIcon(globalIcon)

	// ---- TABS ----
	tabs := qt.NewQTabWidget(nil)

	// ---------- PING TAB ----------
	pingPage := qt.NewQWidget(nil)
	// Root is VERTICAL: [ TopRow (hosts+controls) ] then [ Graph (expands) ]
	pingRoot := qt.NewQVBoxLayout(nil)
	pingPage.SetLayout(pingRoot.QLayout)

	// --- TopRow: HBox (left list, right controls) ---
	topRow := qt.NewQHBoxLayout(nil)

	// LEFT: host list (about 5 rows tall), scrollable if more
	leftPane := qt.NewQWidget(nil)
	leftCol := qt.NewQVBoxLayout(nil)
	leftPane.SetLayout(leftCol.QLayout)

	ui.hostList = qt.NewQListWidget(nil)
	ui.hostList.SetUniformItemSizes(true)
	ui.hostList.SetMinimumWidth(200)
	ui.hostList.SetMaximumWidth(240)
	rowH := ui.hostList.FontMetrics().Height() + 6
	ui.hostList.SetFixedHeight(rowH*5 + 12)

	for _, h := range model.Hosts() {
		ui.hostList.AddItem(fmt.Sprintf("%s (%s)", h.Name, h.Addr))
	}

	ui.btnRem = qt.NewQPushButton(nil)
	ui.btnRem.SetText("Remove selected")

	leftCol.AddWidget(ui.hostList.QWidget)
	leftCol.AddWidget(ui.btnRem.QWidget)

	// RIGHT: controls stacked vertically
	rightPane := qt.NewQWidget(nil)
	rightCol := qt.NewQVBoxLayout(nil)
	rightPane.SetLayout(rightCol.QLayout)

	// Row: Add host + Start/Stop
	rowAdd := qt.NewQHBoxLayout(nil)
	ui.hostName = qt.NewQLineEdit(nil)
	ui.hostName.SetPlaceholderText("Display name (optional)")
	ui.hostAddr = qt.NewQLineEdit(nil)
	ui.hostAddr.SetPlaceholderText("Host/IP (e.g., 1.1.1.1)")

	ui.btnAdd = qt.NewQPushButton(nil)
	ui.btnAdd.SetText("Add host")

	ui.btnStart = qt.NewQPushButton(nil)
	ui.btnStart.SetText("Start")
	ui.btnStop = qt.NewQPushButton(nil)
	ui.btnStop.SetText("Stop")

	rowAdd.AddWidget(ui.hostName.QWidget)
	rowAdd.AddWidget(ui.hostAddr.QWidget)
	rowAdd.AddWidget(ui.btnAdd.QWidget)
	rowAdd.AddStretch()
	rowAdd.AddWidget(ui.btnStart.QWidget)
	rowAdd.AddWidget(ui.btnStop.QWidget)
	rightCol.AddLayout(rowAdd.QLayout)

	// Row: Interval slider
	rowInt := qt.NewQHBoxLayout(nil)
	lbl := qt.NewQLabel6("Interval:", nil, 0)
	ui.intSlider = qt.NewQSlider(nil)
	ui.intSlider.SetOrientation(qt.Horizontal)
	ui.intSlider.SetRange(100, 1000)
	ui.intSlider.SetSingleStep(10)
	ui.intSlider.SetPageStep(50)
	ui.intSlider.SetValue(1000)
	ui.intLabel = qt.NewQLabel6("1000 ms", nil, 0)

	if cfg != nil {
		// Ping tab interval slider
		ui.intSlider.SetValue(cfg.Ping.IntervalMs)
		ui.intLabel.SetText(fmt.Sprintf("%d ms", cfg.Ping.IntervalMs))
	}

	rowInt.AddWidget(lbl.QWidget)
	rowInt.AddWidget(ui.intSlider.QWidget)
	rowInt.AddWidget(ui.intLabel.QWidget)
	rightCol.AddLayout(rowInt.QLayout)

	// Add TopRow pieces
	topRow.AddWidget(leftPane)
	topRow.AddWidget2(rightPane, 1)

	// Put TopRow into root (no stretch, it keeps its natural height)
	pingRoot.AddLayout(topRow.QLayout)

	// Bottom: Graph expands
	ui.graph = NewGraphWidget(model)
	ui.graph.StartTicker()
	pingRoot.AddWidget2(&ui.graph.QWidget, 1) // stretch=1 → grows to fill remaining space

	// ---------- SPEED TEST TAB (placeholder) ----------
	speedPage := qt.NewQWidget(nil)
	speedRoot := qt.NewQVBoxLayout(nil)
	speedPage.SetLayout(speedRoot.QLayout)

	// Try to locate iperf3 binary in ./iperf
	iperfBin, selErr := iperf.SelectBinary(appPath() + "/iperf")
	if selErr != nil || iperfBin == "" {
		center := qt.NewQLabel6("iperf3 binary not found.\nPlace it in ./iperf and restart.", nil, 0)
		center.SetAlignment(qt.AlignCenter)
		speedRoot.AddStretch()
		speedRoot.AddWidget(center.QWidget)
		speedRoot.AddStretch()
	} else {
		// Controls row(s)
		row1 := qt.NewQHBoxLayout(nil)
		host := qt.NewQLineEdit(nil)
		host.SetPlaceholderText("Server/IP (required)")
		port := qt.NewQLineEdit(nil)
		port.SetText("5201")
		dur := qt.NewQLineEdit(nil)
		dur.SetText("10") // seconds
		intv := qt.NewQLineEdit(nil)
		intv.SetText("1") // seconds
		parr := qt.NewQLineEdit(nil)
		parr.SetText("1") // -P streams
		rev := qt.NewQCheckBox4("-R Reverse (download)", nil)
		//bidi := qt.NewQCheckBox4("--bidir (simultaneous)", nil) // we disable it for now, because it needs more work to make it working

		row1.AddWidget(qt.NewQLabel6("Server:", nil, 0).QWidget)
		row1.AddWidget(host.QWidget)
		row1.AddWidget(qt.NewQLabel6("Port:", nil, 0).QWidget)
		row1.AddWidget(port.QWidget)
		row1.AddWidget(qt.NewQLabel6("Duration (s):", nil, 0).QWidget)
		row1.AddWidget(dur.QWidget)
		row1.AddWidget(qt.NewQLabel6("Interval (s):", nil, 0).QWidget)
		row1.AddWidget(intv.QWidget)

		row2 := qt.NewQHBoxLayout(nil)
		row2.AddWidget(qt.NewQLabel6("Parallel -P:", nil, 0).QWidget)
		row2.AddWidget(parr.QWidget)
		row2.AddWidget(rev.QWidget)
		//row2.AddWidget(bidi.QWidget)
		row2.AddStretch()

		if cfg != nil {
			host.SetText(cfg.Speed.Server)
			port.SetText(fmt.Sprint(cfg.Speed.Port))
			dur.SetText(fmt.Sprint(cfg.Speed.DurationSec))
			intv.SetText(fmt.Sprint(cfg.Speed.IntervalSec))
			parr.SetText(fmt.Sprint(cfg.Speed.Parallel))
			rev.SetChecked(cfg.Speed.Reverse)
		}

		// Buttons + status
		row3 := qt.NewQHBoxLayout(nil)
		btnStart := qt.NewQPushButton(nil)
		btnStart.SetText("Start")
		btnStop := qt.NewQPushButton(nil)
		btnStop.SetText("Stop")
		btnStop.SetEnabled(false)
		status := qt.NewQLabel6("Idle.", nil, 0)
		lastMbps := qt.NewQLabel6("0.0 Mbps", nil, 0)

		row3.AddWidget(btnStart.QWidget)
		row3.AddWidget(btnStop.QWidget)
		row3.AddWidget(qt.NewQLabel6("Status:", nil, 0).QWidget)
		row3.AddWidget(status.QWidget)
		row3.AddStretch()
		row3.AddWidget(qt.NewQLabel6("Current:", nil, 0).QWidget)
		row3.AddWidget(lastMbps.QWidget)

		// Graph at bottom
		spGraph := NewSpeedGraphWidget()
		spGraph.StartTicker()

		speedRoot.AddLayout(row1.QLayout)
		speedRoot.AddLayout(row2.QLayout)
		speedRoot.AddLayout(row3.QLayout)
		speedRoot.AddWidget2(&spGraph.QWidget, 1)

		// Runtime wiring
		var cancel context.CancelFunc
		running := false
		setRunning := func(on bool) {
			running = on
			btnStart.SetEnabled(!on)
			btnStop.SetEnabled(on)
		}

		btnStart.OnClicked(func() {
			if running {
				return
			}
			if strings.TrimSpace(host.Text()) == "" {
				status.SetText("Please enter server/IP.")
				return
			}
			cfg := iperf.Config{
				BinDir:      appPath() + "/iperf",
				Host:        strings.TrimSpace(host.Text()),
				Port:        atoiDefault(port.Text(), 5201),
				DurationSec: atoiDefault(dur.Text(), 10),
				Parallel:    atoiDefault(parr.Text(), 1),
				IntervalSec: atoiDefault(intv.Text(), 1),
				Reverse:     rev.IsChecked(),
				//Bidirectional: bidi.IsChecked(),
				Format: "m", // Mbps as in our iperf package
			}
			ctx, cn := context.WithCancel(context.Background())
			cancel = cn

			intervals, done, err := iperf.Run(ctx, cfg)
			if err != nil {
				status.SetText(fmt.Sprintf("Start error: %v", err))
				return
			}
			setRunning(true)
			status.SetText("Running…")
			lastMbps.SetText("0.0 Mbps")

			// Consume intervals and update graph
			go func() {
				for iv := range intervals {
					// iv.Bitrate is like "607 Mbits/sec"
					mbps := parseMbps(iv.Bitrate)
					spGraph.AppendMbps(mbps)
					lastMbps.SetText(fmt.Sprintf("%.1f Mbps", mbps))
				}
			}()
			go func() {
				r := <-done
				if r.ExitErr != nil {
					status.SetText(fmt.Sprintf("Finished with error: %v", r.ExitErr))
				} else {
					status.SetText("Finished.")
				}
				setRunning(false)
			}()
		})

		btnStop.OnClicked(func() {
			if cancel != nil {
				cancel()
				cancel = nil
			}
		})
		onChangeSpeed := func() {
			c := ui.model.Config()
			if c == nil {
				c = defaultConfig()
				ui.model.LoadFromConfig(c)
			}

			// Speed
			c.Speed.Server = strings.TrimSpace(host.Text())
			c.Speed.Port = atoiDefault(port.Text(), 5201)
			c.Speed.DurationSec = atoiDefault(dur.Text(), 10)
			c.Speed.IntervalSec = atoiDefault(intv.Text(), 1)
			c.Speed.Parallel = atoiDefault(parr.Text(), 1)
			c.Speed.Reverse = rev.IsChecked()

			ui.model.SaveConfigAsync()
		}

		host.OnEditingFinished(onChangeSpeed)
		port.OnEditingFinished(onChangeSpeed)
		dur.OnEditingFinished(onChangeSpeed)
		intv.OnEditingFinished(onChangeSpeed)
		parr.OnEditingFinished(onChangeSpeed)
		rev.OnToggled(func(checked bool) { onChangeSpeed() })
	}

	// Add tabs
	tabs.AddTab(pingPage, "Ping")
	tabs.AddTab(speedPage, "Speed test")
	tabs.AddTab(buildTracerouteTab(ui.model), "Traceroute")
	aboutPage := NewAboutPage(ui.model)
	tabs.AddTab(aboutPage, "About")

	ui.main.SetCentralWidget(tabs.QWidget)

	// --- logic wiring

	// on change ping settings, save them
	onChange := func() {
		c := ui.model.Config()
		if c == nil {
			c = defaultConfig()
			ui.model.LoadFromConfig(c)
		}
		// Ping interval slider
		model.SetPingIntervalMs(ui.intSlider.Value())
		ui.model.SaveConfigAsync()
	}

	ui.btnAdd.OnClicked(func() {
		name := ui.hostName.Text()
		addr := ui.hostAddr.Text()
		if addr == "" {
			return
		}
		if name == "" {
			name = addr
		}
		model.AddHost(name, addr, DefaultRingCap)
		ui.hostList.AddItem(fmt.Sprintf("%s (%s)", name, addr))
		ui.hostName.SetText("")
		ui.hostAddr.SetText("")
		ui.updateButtons()
		// persist
		c := ui.model.Config()
		// rebuild hosts slice from model to keep it single source of truth
		c.Ping.Hosts = nil
		for _, h := range ui.model.Hosts() {
			c.Ping.Hosts = append(c.Ping.Hosts, HostConfig{Name: h.Name, Addr: h.Addr, Enabled: true})
		}
		ui.model.SaveConfigAsync()

		if ui.running {
			ui.restartPinging()
		}
	})

	ui.btnRem.OnClicked(func() {
		row := ui.hostList.CurrentRow()
		if row < 0 {
			return
		}
		if ok := model.RemoveHostAt(row); ok {
			_ = ui.hostList.TakeItem(row) // remove from list
			ui.updateButtons()
			c := ui.model.Config()
			c.Ping.Hosts = nil
			for _, h := range ui.model.Hosts() {
				c.Ping.Hosts = append(c.Ping.Hosts, HostConfig{Name: h.Name, Addr: h.Addr, Enabled: true})
			}
			ui.model.SaveConfigAsync()

			if ui.running {
				ui.restartPinging()
			}
		}
	})

	ui.btnStart.OnClicked(func() { ui.StartPinging() })
	ui.btnStop.OnClicked(func() { ui.StopPinging() })

	// Hook selection change once (outside updateButtons) so Remove toggles:
	ui.hostList.OnCurrentRowChanged(func(row int) {
		// Remove is allowed only when something is selected
		ui.btnRem.SetEnabled(row >= 0)
	})

	ui.intSlider.OnValueChanged(func(v int) {
		ui.intLabel.SetText(fmt.Sprintf("%d ms", v))
		onChange()
		if ui.running {
			ui.restartPinging()
		}
	})

	ui.updateButtons()
	return ui
}

func (ui *UI) Show() { ui.main.Show() }

func (ui *UI) StartPinging() {
	if ui.running {
		return
	}
	ui.backend = ProbingBackend{
		Privileged: false,
		Interval:   time.Duration(ui.intSlider.Value()) * time.Millisecond,
		// A practical MaxRTT: 2× interval, but not less than 300 ms (helps on Wi-Fi)
		MaxRTT:    maxDur(2*time.Duration(ui.intSlider.Value())*time.Millisecond, 300*time.Millisecond),
		GraceLate: 100 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	ui.cancel = cancel
	ui.running = true
	ui.updateButtons()

	for _, h := range ui.model.Hosts() {
		h.State = HostRunning
		go func(h *Host) {
			// We don’t use the callback any more – ping.go updates h.buf directly.
			// But we still pass a dummy func to satisfy the signature.
			_ = ui.backend.RunForHost(ctx, h)
			h.State = HostStopped
		}(h)
	}
}

func (ui *UI) StopPinging() {
	if !ui.running {
		return
	}
	if ui.cancel != nil {
		ui.cancel()
		ui.cancel = nil
	}
	ui.running = false
	ui.updateButtons()
}

func (ui *UI) restartPinging() {
	// simple strategy: stop then start with new config
	ui.StopPinging()
	ui.StartPinging()
}

func (ui *UI) updateButtons() {
	ui.btnStart.SetEnabled(!ui.running && ui.model.Count() > 0)
	ui.btnStop.SetEnabled(ui.running)
	// while running, avoid structural changes:
	ui.btnAdd.SetEnabled(!ui.running)
}

// small helper
func maxDur(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// iperf.Interval.Bitrate is "<num> <unit>bits/sec" where unit is K/M/G (already handled in our iperf regex).
func parseMbps(bitrate string) float64 {
	// examples: "937 Mbits/sec", "1.25 Gbits/sec", "880 Kbits/sec"
	parts := strings.Fields(strings.TrimSpace(bitrate))
	if len(parts) < 2 {
		return 0
	}
	val, _ := strconv.ParseFloat(parts[0], 64)
	unit := strings.ToUpper(parts[1])
	// Normalize to Mbps
	switch {
	case strings.HasPrefix(unit, "GBITS"):
		return val * 1000.0
	case strings.HasPrefix(unit, "MBITS"):
		return val
	case strings.HasPrefix(unit, "KBITS"):
		return val / 1000.0
	default:
		return val / 1_000_000.0 // raw bits/sec fallback (rare)
	}
}
