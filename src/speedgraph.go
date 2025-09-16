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
	"fmt"
	"time"

	"github.com/mappu/miqt/qt"
)

type mbpsSample struct {
	T    time.Time
	Mbps float64
}

type mbpsRing struct {
	data  []mbpsSample
	head  int
	count int
}

func newMbpsRing(capacity int) *mbpsRing { return &mbpsRing{data: make([]mbpsSample, capacity)} }
func (r *mbpsRing) push(s mbpsSample) {
	if len(r.data) == 0 {
		return
	}
	r.data[r.head] = s
	r.head = (r.head + 1) % len(r.data)
	if r.count < len(r.data) {
		r.count++
	}
}
func (r *mbpsRing) snapshot(dst []mbpsSample) []mbpsSample {
	if r.count == 0 {
		return dst[:0]
	}
	n := r.count
	if cap(dst) < n {
		dst = make([]mbpsSample, n)
	} else {
		dst = dst[:n]
	}
	start := (r.head - r.count + len(r.data)) % len(r.data)
	for i := 0; i < n; i++ {
		dst[i] = r.data[(start+i)%len(r.data)]
	}
	return dst
}

type SpeedGraphWidget struct {
	qt.QWidget

	span      time.Duration
	marginPx  float64
	frameRate int
	ticker    *qt.QTimer

	ring *mbpsRing

	mouseX      int
	mouseInside bool
}

func NewSpeedGraphWidget() *SpeedGraphWidget {
	w := &SpeedGraphWidget{}
	w.QWidget = *qt.NewQWidget(nil)
	w.SetMinimumSize2(800, 240)
	w.span = 60 * time.Second
	w.marginPx = 40
	w.frameRate = 30
	w.ring = newMbpsRing(600) // ~10 minutes @ 1s; plenty for scrolling window

	w.SetMouseTracking(true)
	w.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { w.mouseInside = true })
	w.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { w.mouseInside = false; w.Update() })
	w.OnMouseMoveEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		w.mouseX = e.X()
		w.Update()
	})
	w.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { w.paint() })
	return w
}

func (w *SpeedGraphWidget) StartTicker() {
	w.ticker = qt.NewQTimer()
	w.ticker.OnTimeout(func() { w.Update() })
	ms := 1000 / w.frameRate
	if ms <= 0 {
		ms = 33
	}
	w.ticker.Start(ms)
}

func (w *SpeedGraphWidget) AppendMbps(v float64) {
	w.ring.push(mbpsSample{T: time.Now(), Mbps: v})
}

func (w *SpeedGraphWidget) paint() {
	W := float64(w.Width())
	H := float64(w.Height())
	if W < 4 || H < 4 {
		return
	}

	p := qt.NewQPainter()
	if !p.Begin(w.QPaintDevice) {
		return
	}
	defer p.End()
	p.SetRenderHint2(qt.QPainter__Antialiasing, true)

	// ----- palette-aware colors -----
	bg := w.Palette().ColorWithCr(qt.QPalette__Window)
	txt := w.Palette().ColorWithCr(qt.QPalette__WindowText)
	// Grid color: faint version of text
	gridCol := qt.NewQColor()
	gridCol.SetRgb2(txt.Red(), txt.Green(), txt.Blue(), 80)
	lineCol := qcolor(90, 180, 255, 255)
	tooltipBg := qcolor(0, 0, 0, 160)

	p.FillRect4(qt.NewQRectF4(0, 0, W, H), bg)

	now := time.Now()
	startT := now.Add(-w.span)

	// ---- collect points in window ----
	buf := w.ring.snapshot(nil)
	var pts []mbpsSample
	for _, s := range buf {
		if s.T.After(startT) {
			pts = append(pts, s)
		}
	}

	// ---- dynamic Y scale with headroom ----
	yMin := 0.0
	yMax := 0.0
	for _, s := range pts {
		if s.Mbps > yMax {
			yMax = s.Mbps
		}
	}
	// ensure at least a floor
	if yMax <= 0 {
		yMax = 1
	}
	// add ~10% headroom so top point is not clipped
	yMax *= 1.10 // +10% headroom
	ticks := niceTicks(yMin, yMax, 5)
	if len(ticks) > 0 {
		yMax = ticks[len(ticks)-1] // snap to nice top
	}

	// ----- dynamic margins from font metrics -----
	fm := qt.NewQFontMetricsF(p.Font())
	// widest possible Y label among ticks, format like "1000Mbps"
	maxYLabelW := 0.0
	yLabels := make([]string, len(ticks))
	for i, v := range ticks {
		s := fmt.Sprintf("%.0f Mbps", v)
		yLabels[i] = s
		if w := fm.Width(s); w > maxYLabelW {
			maxYLabelW = w
		}
	}
	const yLabelGap = 8.0
	const axisTitleGap = 6.0

	left := w.marginPx + maxYLabelW + yLabelGap + fm.Height() + axisTitleGap // space for Y labels + vertical "Mbps"
	bottomPad := fm.Height() + 10.0                                          // room for time labels
	top := w.marginPx
	bottom := H - w.marginPx - bottomPad
	right := W - w.marginPx

	if right-left < 40 || bottom-top < 40 {
		return // too small to render nicely
	}

	// ---- plot rect + clipping for grid/series ----
	plotRect := qt.NewQRectF4(left, top, right-left, bottom-top)

	// ---- Y grid lines + labels (grid clipped, labels outside) ----
	p.Save()
	p.SetClipRect3(plotRect, qt.ReplaceClip)
	p.SetPen(gridCol)
	for _, v := range ticks {
		y := mapY(v, yMin, yMax, top, bottom)
		path := qt.NewQPainterPath2(qt.NewQPointF3(left, y))
		path.LineTo(qt.NewQPointF3(right, y))
		p.DrawPath(path)
	}
	p.Restore()

	// Y labels (no clip)
	p.SetPen(txt)
	for i, v := range ticks {
		y := mapY(v, yMin, yMax, top, bottom)
		lbl := qt.NewQStaticText2(yLabels[i])
		p.DrawStaticText2(qt.NewQPoint2(int(left-maxYLabelW-yLabelGap), int(y-fm.Height()/2)), lbl)
	}

	// ---- vertical axis title "Mbps" ----
	p.Save()
	title := "Mbps"
	tx := left - maxYLabelW - yLabelGap - axisTitleGap
	ty := (top + bottom) / 2
	p.Translate2(tx, ty)
	p.Rotate(-90)
	p.SetPen(txt)
	p.DrawStaticText2(qt.NewQPoint2(int(-fm.Height()/2), int(-fm.Width(title)/2)), qt.NewQStaticText2(title))
	p.Restore()

	// --- Grid X each 10s ---
	var xTicks []time.Time
	p.Save()
	p.SetClipRect3(plotRect, qt.ReplaceClip)
	p.SetPen(gridCol)
	for t := startT.Truncate(10 * time.Second); !t.After(now); t = t.Add(10 * time.Second) {
		xTicks = append(xTicks, t)
		x := mapX(t, startT, now, left, right)
		path := qt.NewQPainterPath2(qt.NewQPointF3(x, top))
		path.LineTo(qt.NewQPointF3(x, bottom))
		p.DrawPath(path)
	}
	p.Restore()

	// ---- series (clipped; cosmetic pen so it shows on Windows HiDPI) ----
	if len(pts) >= 2 {
		p.Save()
		p.SetClipRect3(plotRect, qt.ReplaceClip)
		pen := qt.NewQPen3(lineCol)
		pen.SetCosmetic(true)
		pen.SetWidthF(2.0)
		p.SetPenWithPen(pen)

		var path *qt.QPainterPath
		for i, s := range pts {
			x := mapX(s.T, startT, now, left, right)
			y := mapY(s.Mbps, yMin, yMax, top, bottom)
			if i == 0 {
				path = qt.NewQPainterPath2(qt.NewQPointF3(x, y))
			} else {
				path.LineTo(qt.NewQPointF3(x, y))
			}
		}
		if path != nil {
			p.DrawPath(path)
		}
		p.Restore()
	}

	// ---- X time labels (clamped to plot; prevent overlaps) ----
	p.SetPen(txt)
	prevRight := left - 6 // last drawn label's right edge
	for _, t := range xTicks {
		x := mapX(t, startT, now, left, right)
		text := t.Format("15:04:05")
		tw := fm.Width(text)

		// center on x, then clamp to [left, right - tw]
		posX := x - tw/2
		if posX < left {
			posX = left
		}
		if posX+tw > right {
			posX = right - tw
		}

		// skip if overlapping previous label (keep 6px gap)
		if posX < prevRight+6 {
			continue
		}

		p.DrawStaticText2(qt.NewQPoint2(int(posX), int(bottom+4)), qt.NewQStaticText2(text))
		prevRight = posX + tw
	}

	// --- Hover crosshair + tooltip ---
	if w.mouseInside && w.mouseX >= int(left) && w.mouseX <= int(right) {
		x := float64(w.mouseX)
		p.Save()
		p.SetClipRect3(plotRect, qt.ReplaceClip)
		// crosshair uses faint text color (theme-aware)
		cs := qt.NewQColor()
		cs.SetRgb2(txt.Red(), txt.Green(), txt.Blue(), 80)
		p.SetPen(cs)
		path := qt.NewQPainterPath2(qt.NewQPointF3(x, top))
		path.LineTo(qt.NewQPointF3(x, bottom))
		p.DrawPath(path)
		p.Restore()

		tAtX := unmapX(x, startT, now, left, right)
		// nearest point
		best := mbpsSample{}
		bestDT := time.Duration(1<<62 - 1)
		for _, s := range pts {
			dt := s.T.Sub(tAtX)
			if dt < 0 {
				dt = -dt
			}
			if dt < bestDT {
				bestDT = dt
				best = s
			}
		}
		// tooltip (outside clip)
		box := qt.NewQRectF4(x+8, top+8, 170, 40)
		if box.X()+box.Width() > right {
			box.SetX(right - box.Width())
		}
		p.FillRect4(box, tooltipBg)
		// tooltip text uses normal text color for contrast
		p.SetPen(qcolor(255, 255, 255, 220))
		p.DrawStaticText2(qt.NewQPoint2(int(box.X()+6), int(box.Y()+6)), qt.NewQStaticText2(tAtX.Format("15:04:05")))
		p.DrawStaticText2(qt.NewQPoint2(int(box.X()+6), int(box.Y()+22)), qt.NewQStaticText2(fmt.Sprintf("%.1f Mbps", best.Mbps)))
	}
}
