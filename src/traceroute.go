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
	"math"
	"strings"
	"time"

	traceroute_wrapper "github.com/e1z0/speedping/internal/traceroute"
	"github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"
)

func buildTracerouteTab(model *AppModel) *qt.QWidget {
	page := qt.NewQWidget(nil)
	col := qt.NewQVBoxLayout(nil)
	page.SetLayout(col.QLayout)

	// Controls
	row := qt.NewQHBoxLayout(nil)
	target := qt.NewQLineEdit(nil)
	target.SetPlaceholderText("Target (domain or IP)")
	maxHops := qt.NewQLineEdit(nil)
	maxHops.SetText("30")
	timeout := qt.NewQLineEdit(nil)
	timeout.SetText("1.0") // seconds
	probes := qt.NewQLineEdit(nil)
	probes.SetText("1")
	noDNS := qt.NewQCheckBox4("Don't resolve", nil)

	start := qt.NewQPushButton(nil)
	start.SetText("Start")
	stop := qt.NewQPushButton(nil)
	stop.SetText("Stop")
	stop.SetEnabled(false)
	status := qt.NewQLabel6("Idle.", nil, 0)

	row.AddWidget(qt.NewQLabel6("Target:", nil, 0).QWidget)
	row.AddWidget(target.QWidget)
	row.AddWidget(qt.NewQLabel6("Max hops:", nil, 0).QWidget)
	row.AddWidget(maxHops.QWidget)
	row.AddWidget(qt.NewQLabel6("Timeout(s):", nil, 0).QWidget)
	row.AddWidget(timeout.QWidget)
	row.AddWidget(qt.NewQLabel6("Probes:", nil, 0).QWidget)
	row.AddWidget(probes.QWidget)
	row.AddWidget(noDNS.QWidget)
	row.AddStretch()
	row.AddWidget(start.QWidget)
	row.AddWidget(stop.QWidget)

	col.AddLayout(row.QLayout)
	col.AddWidget(status.QWidget)

	// Table
	table := qt.NewQTableWidget(nil)
	table.SetColumnCount(3)
	table.SetHorizontalHeaderLabels([]string{"Hop", "Address", "RTT (ms)"})
	table.HorizontalHeader().SetStretchLastSection(true)
	col.AddWidget2(table.QWidget, 1)

	// Graph
	tmap := NewTracerMap()
	col.AddWidget(&tmap.QWidget)

	// ---- LOAD from config ----
	if c := model.Config(); c != nil {
		target.SetText(c.Trace.Target)
		if c.Trace.MaxHops <= 0 {
			c.Trace.MaxHops = 30
		}
		maxHops.SetText(fmt.Sprint(c.Trace.MaxHops))

		if c.Trace.TimeoutSec <= 0 {
			c.Trace.TimeoutSec = 1.0
		}
		timeout.SetText(fmt.Sprintf("%.1f", c.Trace.TimeoutSec))

		if c.Trace.Probes <= 0 {
			c.Trace.Probes = 1
		}
		probes.SetText(fmt.Sprint(c.Trace.Probes))

		noDNS.SetChecked(c.Trace.DontResolve)

		// pulse speed
		if c.Trace.PulseSeconds > 0 {
			tmap.SetPulseSpeed(c.Trace.PulseSeconds)
		}
	}

	// ---- SAVE helper (debounced by model) ----
	saveNow := func() {
		c := model.Config()
		if c == nil {
			c = defaultConfig()
			model.LoadFromConfig(c)
		}

		c.Trace.Target = strings.TrimSpace(target.Text())
		c.Trace.MaxHops = atoiDefault(maxHops.Text(), 30)
		c.Trace.TimeoutSec = atofDefault(timeout.Text(), 1.0)
		c.Trace.Probes = atoiDefault(probes.Text(), 1)
		c.Trace.DontResolve = noDNS.IsChecked()
		// keep current pulse speed (tmap already has it); if we want a hidden default, persist it:
		if c.Trace.PulseSeconds <= 0 {
			c.Trace.PulseSeconds = 6.0
		}

		model.SaveConfigAsync()
	}

	// Connect signals to save on change
	target.OnEditingFinished(saveNow)
	maxHops.OnEditingFinished(saveNow)
	timeout.OnEditingFinished(saveNow)
	probes.OnEditingFinished(saveNow)
	noDNS.OnToggled(func(bool) { saveNow() })

	// Runtime
	var cancel context.CancelFunc
	setRunning := func(on bool) {
		start.SetEnabled(!on)
		stop.SetEnabled(on)
	}

	start.OnClicked(func() {
		if !start.IsEnabled() {
			return
		}
		saveNow()

		tgt := strings.TrimSpace(target.Text())
		if tgt == "" {
			status.SetText("Please enter target")
			return
		}

		// update config from UI once more before running
		saveNow()

		c := model.Config()
		opt := traceroute_wrapper.Options{
			Target:      c.Trace.Target,
			MaxHops:     c.Trace.MaxHops,
			Timeout:     time.Duration(c.Trace.TimeoutSec*1000) * time.Millisecond,
			Probes:      c.Trace.Probes,
			DontResolve: c.Trace.DontResolve,
		}

		table.SetRowCount(0)
		tmap.Reset()

		ctx, cn := context.WithCancel(context.Background())
		cancel = cn

		ev, err := traceroute_wrapper.Run(ctx, opt)
		if err != nil {
			status.SetText(fmt.Sprintf("Error: %v", err))
			return
		}

		setRunning(true)
		status.SetText("Running…")
		tmap.Reset()

		setRunning(true)

		go func() {
			for e := range ev {
				switch e.Kind {
				case "hop":
					h := *e.Hop
					mainthread.Wait(func() {
						// table row
						r := table.RowCount()
						table.InsertRow(r)
						table.SetItem(r, 0, qt.NewQTableWidgetItem2(fmt.Sprintf("%d", h.Index)))
						table.SetItem(r, 1, qt.NewQTableWidgetItem2(h.Addr))
						if h.RTTms < 0 {
							table.SetItem(r, 2, qt.NewQTableWidgetItem2("timeout"))
						} else {
							table.SetItem(r, 2, qt.NewQTableWidgetItem2(fmt.Sprintf("%.1f", h.RTTms)))
						}
						// map
						tmap.UpsertHop(h.Index, h.Addr, h.RTTms)
					})
				case "error":
					msg := e.Msg
					if e.Err != nil {
						msg += ": " + e.Err.Error()
					}
					mainthread.Wait(func() { status.SetText("Error: " + msg) })

				case "done":
					mainthread.Wait(func() {
						status.SetText("Done.")
						setRunning(false)
						tmap.SetDone()
					})
				}
			}
		}()
	})

	stop.OnClicked(func() {
		if cancel != nil {
			cancel()
			cancel = nil
		}
	})

	return page
}

type TraceHop struct {
	Hop   int
	Addr  string
	RTTms float64 // -1 == timeout
}

type TracerMap struct {
	qt.QWidget

	margin float64
	span   int // max hops shown on X (e.g., 30)

	hops       []TraceHop // ordered by Hop
	yMax       float64    // dynamic scale
	pulsePhase float64    // 0..1 animation position
	pulseSpeed float64    // cycles per second (e.g., 0.15 => ~6.7s per loop)
	lastTick   time.Time  // for dt-based animation
	anim       *qt.QTimer
	done       bool

	mousePos qt.QPoint
	hoverHop int // -1 if none
}

func NewTracerMap() *TracerMap {
	g := &TracerMap{}
	g.QWidget = *qt.NewQWidget(nil)
	g.SetMinimumSize2(900, 240)
	g.margin = 36
	g.span = 30
	g.hoverHop = -1

	g.SetMouseTracking(true)
	g.OnMouseMoveEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		g.mousePos = *qt.NewQPoint2(e.X(), e.Y())
		g.hoverHop = g.hitTestHop(float64(e.X()), float64(e.Y()))
		g.Update()
	})
	g.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { g.hoverHop = -1; g.Update() })

	g.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { g.paint() })

	g.pulsePhase = 0
	g.pulseSpeed = 0.15 // slower: one full pass ~6.7 seconds
	g.lastTick = time.Now()

	g.anim = qt.NewQTimer()
	g.anim.OnTimeout(func() {
		if len(g.hops) == 0 {
			g.lastTick = time.Now()
			return
		}
		// 60 FPS-ish pulse
		dt := time.Since(g.lastTick).Seconds()
		g.lastTick = time.Now()
		g.pulsePhase += g.pulseSpeed * dt
		// keep in [0,1)
		g.pulsePhase -= math.Floor(g.pulsePhase)
		g.Update()
	})
	g.anim.Start(16) // ~60fps

	return g
}

// secondsPerLoop: 0 => keep current; otherwise set new speed
func (g *TracerMap) SetPulseSpeed(secondsPerLoop float64) {
	if secondsPerLoop <= 0 {
		return
	}
	g.pulseSpeed = 1.0 / secondsPerLoop
}

func (g *TracerMap) Reset() {
	g.hops = nil
	g.yMax = 0
	g.pulsePhase = 0
	g.done = false
	g.Update()
}

func (g *TracerMap) UpsertHop(hop int, addr string, rttMs float64) {
	for i := range g.hops {
		if g.hops[i].Hop == hop {
			g.hops[i].Addr = addr
			g.hops[i].RTTms = rttMs
			g.recalcY()
			g.Update()
			return
		}
	}
	g.hops = append(g.hops, TraceHop{Hop: hop, Addr: addr, RTTms: rttMs})
	// keep in hop order (small N, simple bubble insert)
	for i := len(g.hops) - 1; i > 0 && g.hops[i-1].Hop > g.hops[i].Hop; i-- {
		g.hops[i-1], g.hops[i] = g.hops[i], g.hops[i-1]
	}
	g.recalcY()
	g.Update()
}

func (g *TracerMap) SetDone() { g.done = true; g.Update() }

func (g *TracerMap) recalcY() {
	max := 1.0
	for _, h := range g.hops {
		if h.RTTms > max {
			max = h.RTTms
		}
	}
	// headroom + round to nice top (0/5/10 …)
	max *= 1.10
	if max < 30 {
		max = 30
	}
	// snap to 1,2,5 * 10^k
	g.yMax = niceTop(max)
}

func niceTop(x float64) float64 {
	k := math.Pow10(int(math.Floor(math.Log10(x))))
	base := x / k
	var step float64
	switch {
	case base <= 1:
		step = 1
	case base <= 2:
		step = 2
	case base <= 5:
		step = 5
	default:
		step = 10
	}
	return step * k
}

func (g *TracerMap) paint() {
	W := float64(g.Width())
	H := float64(g.Height())
	if W < 10 || H < 10 {
		return
	}
	p := qt.NewQPainter()
	if !p.Begin(g.QPaintDevice) {
		return
	}
	defer p.End()
	p.SetRenderHint2(qt.QPainter__Antialiasing, true)

	bg := g.Palette().ColorWithCr(qt.QPalette__Window)
	fg := g.Palette().ColorWithCr(qt.QPalette__WindowText)
	grid := qt.NewQColor()
	grid.SetRgb2(fg.Red(), fg.Green(), fg.Blue(), 80)
	subtle := qt.NewQColor()
	subtle.SetRgb2(fg.Red(), fg.Green(), fg.Blue(), 140)

	p.FillRect4(qt.NewQRectF4(0, 0, W, H), bg)

	fm := qt.NewQFontMetricsF(p.Font())
	// measure widest Y label among our ticks
	yticks := 6
	maxYLabelW := 0.0
	for i := 0; i <= yticks; i++ {
		v := g.yMax * float64(i) / float64(yticks)
		s := fmt.Sprintf("%.0f ms", v)
		if w := fm.Width(s); w > maxYLabelW {
			maxYLabelW = w
		}
	}

	// space budget
	yLabelGap := 8.0
	axisTitleGap := 10.0
	titleRotWidth := fm.Width("ms") // rotated height ≈ unrotated width

	left := g.margin + maxYLabelW + yLabelGap + titleRotWidth + axisTitleGap
	right := W - g.margin
	bottom := H - g.margin - (fm.Height() + 10)
	top := g.margin
	if right-left < 40 || bottom-top < 40 {
		return
	}
	plot := qt.NewQRectF4(left, top, right-left, bottom-top)
	// --- Y grid + labels ---
	p.Save()
	p.SetClipRect3(plot, qt.ReplaceClip)
	p.SetPen(grid)
	yticks = 6
	for i := 0; i <= yticks; i++ {
		v := g.yMax * float64(i) / float64(yticks)
		y := top + (bottom-top)*(1-v/g.yMax)
		path := qt.NewQPainterPath2(qt.NewQPointF3(left, y))
		path.LineTo(qt.NewQPointF3(right, y))
		p.DrawPath(path)
	}
	p.Restore()

	// labels
	p.SetPen(fg)
	for i := 0; i <= yticks; i++ {
		v := g.yMax * float64(i) / float64(yticks)
		y := top + (bottom-top)*(1-v/g.yMax)
		lbl := qt.NewQStaticText2(fmt.Sprintf("%.0f ms", v))
		p.DrawStaticText2(qt.NewQPoint2(int(left-fm.Width(lbl.Text())-8), int(y-fm.Height()/2)), lbl)
	}
	// vertical axis title "ms" — placed left of the labels, no overlap
	p.Save()
	title := "ms"
	tx := left - maxYLabelW - yLabelGap - axisTitleGap - titleRotWidth/2
	ty := (top + bottom) / 2
	p.Translate2(tx, ty)
	p.Rotate(-90)
	p.SetPen(fg)
	p.DrawStaticText2(qt.NewQPoint2(0, 0), qt.NewQStaticText2(title))
	p.Restore()

	// --- X grid (hop numbers) ---
	p.Save()
	p.SetClipRect3(plot, qt.ReplaceClip)
	p.SetPen(grid)
	for hop := 1; hop <= g.span; hop++ {
		x := left + (right-left)*float64(hop-1)/float64(g.span-1)
		path := qt.NewQPainterPath2(qt.NewQPointF3(x, top))
		path.LineTo(qt.NewQPointF3(x, bottom))
		p.DrawPath(path)
	}
	p.Restore()
	// X labels (clamped, non-overlap)
	prevR := left - 6
	for hop := 1; hop <= g.span; hop++ {
		t := fmt.Sprintf("%d", hop)
		tw := fm.Width(t)
		x := left + (right-left)*float64(hop-1)/float64(g.span-1)
		pos := x - tw/2
		if pos < left {
			pos = left
		}
		if pos+tw > right {
			pos = right - tw
		}
		if pos < prevR+6 {
			continue
		}
		p.DrawStaticText2(qt.NewQPoint2(int(pos), int(bottom+4)), qt.NewQStaticText2(t))
		prevR = pos + tw
	}

	// --- Path & nodes ---
	p.Save()
	p.SetClipRect3(plot, qt.ReplaceClip)

	// neon path pen
	neon := qt.NewQColor()
	neon.SetRgb2(90, 180, 255, 220)
	pen := qt.NewQPen3(neon)
	pen.SetCosmetic(true)
	pen.SetWidthF(2.2)
	p.SetPenWithPen(pen)

	// Build polyline through OK hops (timeouts break the line)
	var path *qt.QPainterPath
	var have bool
	for _, hhop := range g.hops {
		if hhop.RTTms < 0 {
			if have && path != nil {
				p.DrawPath(path)
				have, path = false, nil
			}
			continue
		}
		x := left + (right-left)*float64(hhop.Hop-1)/float64(g.span-1)
		y := top + (bottom-top)*(1-hhop.RTTms/g.yMax)
		if !have {
			path = qt.NewQPainterPath2(qt.NewQPointF3(x, y))
			have = true
		} else {
			path.LineTo(qt.NewQPointF3(x, y))
		}
	}
	if have && path != nil {
		p.DrawPath(path)
	}

	// Nodes
	okFill := qt.NewQColor()
	okFill.SetRgb2(90, 180, 255, 255)
	toFill := qt.NewQColor()
	toFill.SetRgb2(180, 180, 180, 255)
	dstFill := qt.NewQColor()
	dstFill.SetRgb2(120, 255, 170, 255)

	// Track hovered hop to paint tooltip later (top-most, no clip)
	var hovered *TraceHop
	var hoveredX, hoveredY float64
	for i, hhop := range g.hops {
		x := left + (right-left)*float64(hhop.Hop-1)/float64(g.span-1)
		var y float64
		if hhop.RTTms < 0 {
			y = bottom - 2 // timeouts sit near baseline
		} else {
			y = top + (bottom-top)*(1-hhop.RTTms/g.yMax)
		}

		r := 4.0
		rect := qt.NewQRectF4(x-r, y-r, 2*r, 2*r)

		// glow halo
		halo := qt.NewQColor()
		if i == len(g.hops)-1 && g.done && hhop.RTTms >= 0 {
			halo.SetRgb2(dstFill.Red(), dstFill.Green(), dstFill.Blue(), 80)
		} else if hhop.RTTms >= 0 {
			halo.SetRgb2(okFill.Red(), okFill.Green(), okFill.Blue(), 70)
		} else {
			halo.SetRgb2(toFill.Red(), toFill.Green(), toFill.Blue(), 60)
		}
		p.FillRect4(qt.NewQRectF4(rect.X()-2, rect.Y()-2, rect.Width()+4, rect.Height()+4), halo)

		// core
		if i == len(g.hops)-1 && g.done && hhop.RTTms >= 0 {
			p.FillRect4(rect, dstFill)
		} else if hhop.RTTms >= 0 {
			p.FillRect4(rect, okFill)
		} else {
			p.FillRect4(rect, toFill)
		}

		// hover label
		if g.hoverHop == hhop.Hop {
			hovered = &g.hops[i]
			hoveredX, hoveredY = x, y
		}
	}
	p.Restore()

	// --- Comet pulse (head + tapered tail) ---
	if len(g.hops) >= 2 {
		// collect OK hops as points
		type pt struct{ x, y float64 }
		pts := []pt{}
		for _, h := range g.hops {
			if h.RTTms >= 0 {
				x := left + (right-left)*float64(h.Hop-1)/float64(g.span-1)
				y := top + (bottom-top)*(1-h.RTTms/g.yMax)
				pts = append(pts, pt{x, y})
			}
		}
		if len(pts) >= 2 {
			// total path length
			total := 0.0
			seglen := make([]float64, len(pts)-1)
			for i := 1; i < len(pts); i++ {
				d := math.Hypot(pts[i].x-pts[i-1].x, pts[i].y-pts[i-1].y)
				seglen[i-1] = d
				total += d
			}
			if total > 1 {
				// distance of head along path
				dist := g.pulsePhase * total

				// helper: get point at absolute distance s along the polyline (clamped)
				atDist := func(s float64) (float64, float64) {
					if s <= 0 {
						return pts[0].x, pts[0].y
					}
					if s >= total {
						last := pts[len(pts)-1]
						return last.x, last.y
					}
					rem := s
					for i := 1; i < len(pts); i++ {
						if rem <= seglen[i-1] {
							t := rem / seglen[i-1]
							x := pts[i-1].x + (pts[i].x-pts[i-1].x)*t
							y := pts[i-1].y + (pts[i].y-pts[i-1].y)*t
							return x, y
						}
						rem -= seglen[i-1]
					}
					last := pts[len(pts)-1]
					return last.x, last.y
				}

				// head position
				px, py := atDist(dist)

				// compute direction (for subtle head elongation)
				px2, py2 := atDist(math.Max(0, dist-1.0))
				dx, dy := px-px2, py-py2
				dv := math.Hypot(dx, dy)
				ux, uy := 0.0, 0.0
				if dv > 0 {
					ux, uy = dx/dv, dy/dv
				}

				p.Save()
				p.SetClipRect3(plot, qt.ReplaceClip)
				p.SetRenderHint2(qt.QPainter__Antialiasing, true)

				// We use a solid brush + NoPen for filled shapes (DrawPoint ignores pen width)
				noPen := qt.NewQPen()
				noPen.SetStyle(qt.NoPen)
				p.SetPenWithPen(noPen)

				// ---- Tail: draw N samples behind the head with fading alpha and widening radius
				tailLen := 80.0 // total tail length in pixels along the path
				samples := 18   // number of dabs in tail
				baseR := 2.0    // smallest tail radius at the very end
				headR := 6.0    // radius near the head (tail blends into head)
				maxAlpha := 160 // max opacity near head for tail dabs

				for i := 0; i < samples; i++ {
					// s goes from 0 (near head) to 1 (tail end)
					s := float64(i) / float64(samples-1)
					// distance behind head at this sample (ease-out for nicer taper)
					back := tailLen * (s * s)
					tx, ty := atDist(dist - back)

					// radius grows from baseR to headR as we approach the head
					r := baseR + (headR-baseR)*(1.0-s)

					// alpha fades out with distance (slightly steeper than linear)
					alpha := int(float64(maxAlpha) * math.Pow(1.0-s, 1.3))
					if alpha < 0 {
						alpha = 0
					}

					// warm red tail (slightly dimmer than head)
					clr := qt.NewQColor()
					clr.SetRgb2(255, 60, 40, alpha)
					p.SetBrush(qt.NewQBrush3(clr))

					p.DrawEllipse(qt.NewQRectF4(tx-r, ty-r, 2*r, 2*r))
				}

				// ---- Head: bright core + red mantle + subtle forward “flare”
				// Forward flare: a tiny elongated dab in the direction of travel
				if dv > 0 {
					fl := 8.0 // flare length
					fw := 4.0 // flare width (radius)
					fx := px + ux*fl*0.5
					fy := py + uy*fl*0.5

					flClr := qt.NewQColor()
					flClr.SetRgb2(255, 80, 60, 120)
					p.SetBrush(qt.NewQBrush3(flClr))
					// Draw two overlapping ellipses along the direction to fake elongation
					p.DrawEllipse(qt.NewQRectF4(fx-fw, fy-fw, 2*fw, 2*fw))
					p.DrawEllipse(qt.NewQRectF4(px-fw, py-fw, 2*fw, 2*fw))
				}

				// Red mantle (soft)
				mantle := qt.NewQColor()
				mantle.SetRgb2(255, 40, 20, 200)
				p.SetBrush(qt.NewQBrush3(mantle))
				p.DrawEllipse(qt.NewQRectF4(px-7, py-7, 14, 14))

				// Bright white core
				core := qt.NewQColor()
				core.SetRgb2(255, 255, 255, 255)
				p.SetBrush(qt.NewQBrush3(core))
				p.DrawEllipse(qt.NewQRectF4(px-3.5, py-3.5, 7, 7))

				// Soft outer glow
				glow := qt.NewQColor()
				glow.SetRgb2(255, 60, 30, 80)
				p.SetBrush(qt.NewQBrush3(glow))
				p.DrawEllipse(qt.NewQRectF4(px-11, py-11, 22, 22))

				p.Restore()
			}
		}
	}

	// --- Hover tooltip (draw last, NO CLIP, so it's always on top) ---
	if hovered != nil {
		lbl := fmt.Sprintf("hop %d  %s \n%.1f ms", hovered.Hop, hovered.Addr, hovered.RTTms)
		if hovered.RTTms < 0 {
			lbl = fmt.Sprintf("hop %d  %s\n timeout", hovered.Hop, hovered.Addr)
		}
		bw, bh := 200.0, 44.0
		bx, by := hoveredX+10, hoveredY-22
		if bx+bw > right {
			bx = right - bw
		}
		if bx < left {
			bx = left
		}
		if by < top {
			by = hoveredY + 12
		}
		p.FillRect4(qt.NewQRectF4(bx, by, bw, bh), qcolor(0, 0, 0, 170))
		// tooltip text color
		p.SetPen(qcolor(255, 255, 255, 220))
		p.DrawStaticText2(qt.NewQPoint2(int(bx+6), int(by+6)), qt.NewQStaticText2(lbl))
	}
}

func (g *TracerMap) hitTestHop(mx, my float64) int {
	if len(g.hops) == 0 {
		return -1
	}
	fm := qt.NewQFontMetricsF(g.Font())
	left := g.margin + fm.Width("1000 ms") + 10 + fm.Height() + 6
	right := float64(g.Width()) - g.margin
	bottom := float64(g.Height()) - g.margin - (fm.Height() + 10)
	top := g.margin

	for _, h := range g.hops {
		x := left + (right-left)*float64(h.Hop-1)/float64(g.span-1)
		var y float64
		if h.RTTms < 0 {
			y = bottom - 2
		} else {
			y = top + (bottom-top)*(1-h.RTTms/g.yMax)
		}
		if math.Hypot(mx-x, my-y) <= 8 {
			return h.Hop
		}
	}
	return -1
}
