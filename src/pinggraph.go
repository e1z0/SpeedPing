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

type GraphWidget struct {
	qt.QWidget

	model     *AppModel
	timeSpan  time.Duration
	marginPx  float64
	frameRate int

	ticker      *qt.QTimer
	mouseX      int
	mouseInside bool
}

func NewGraphWidget(model *AppModel) *GraphWidget {
	g := &GraphWidget{}
	g.QWidget = *qt.NewQWidget(nil)
	g.SetMinimumSize2(800, 320)

	g.model = model
	g.timeSpan = 60 * time.Second
	g.marginPx = 40
	g.frameRate = 30

	// enable hover
	g.SetMouseTracking(true)
	g.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { g.mouseInside = true })
	g.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { g.mouseInside = false; g.Update() })
	g.OnMouseMoveEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		g.mouseX = e.X()
		g.Update()
	})

	g.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		g.paint()
	})
	return g
}

func (g *GraphWidget) StartTicker() {
	g.ticker = qt.NewQTimer()
	g.ticker.OnTimeout(func() { g.Update() })
	ms := g.frameRateToMs()
	if ms <= 0 {
		ms = 33
	}
	g.ticker.Start(ms)
}

func (g *GraphWidget) frameRateToMs() int {
	if g.frameRate <= 0 {
		return 33
	}
	return int(1000 / g.frameRate)
}

func (g *GraphWidget) paint() {
	w := float64(g.Width())
	h := float64(g.Height())
	if w <= 2 || h <= 2 {
		return
	}

	p := qt.NewQPainter()
	if !p.Begin(g.QPaintDevice) {
		return
	}
	defer p.End()

	// High quality lines
	p.SetRenderHint2(qt.QPainter__Antialiasing, true)

	bg := g.Palette().ColorWithCr(qt.QPalette__Window)
	txt := g.Palette().ColorWithCr(qt.QPalette__WindowText)
	gridCol := qt.NewQColor()
	gridCol.SetRgb2(txt.Red(), txt.Green(), txt.Blue(), 80)

	p.FillRect4(qt.NewQRectF4(0, 0, w, h), bg)

	now := time.Now()
	startT := now.Add(-g.timeSpan)

	// ---- dynamic Y range (with headroom) ----
	yMin := 0.0
	yMax := 0.0
	for _, host := range g.model.Hosts() {
		tmp := host.buf.Snapshot(nil)
		for _, s := range tmp {
			if s.MS >= 0 && s.MS > yMax {
				yMax = s.MS
			}
		}
	}
	if yMax <= 0 {
		yMax = 1
	}
	yMax *= 1.10 // +10% headroom
	ticks := niceTicks(yMin, yMax, 5)
	if len(ticks) > 0 {
		yMax = ticks[len(ticks)-1] // snap top to a nice tick
	}

	// ---- dynamic margins from font metrics ----
	fm := qt.NewQFontMetricsF(p.Font())

	// widest Y label like "123 ms"
	maxYLabelW := 0.0
	yLabels := make([]string, len(ticks))
	for i, v := range ticks {
		s := formatMS(v) // e.g., "50 ms"
		yLabels[i] = s
		if w := fm.Width(s); w > maxYLabelW {
			maxYLabelW = w
		}
	}

	const yLabelGap = 8.0
	const axisTitleGap = 6.0

	left := g.marginPx + maxYLabelW + yLabelGap + fm.Height() + axisTitleGap // room for labels + vertical "ms"
	bottomPad := fm.Height() + 10.0                                          // room for time labels
	top := g.marginPx
	bottom := h - g.marginPx - bottomPad
	right := w - g.marginPx

	if right-left < 40 || bottom-top < 40 {
		return // too small to render nicely
	}

	plotRect := qt.NewQRectF4(left, top, right-left, bottom-top)

	// ---- Y grid (clipped) ----
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

	// ---- vertical axis title "ms" ----
	p.Save()
	title := "ms"
	tx := left - maxYLabelW - yLabelGap - axisTitleGap
	ty := (top + bottom) / 2
	p.Translate2(tx, ty)
	p.Rotate(-90)
	p.SetPen(txt)
	p.DrawStaticText2(qt.NewQPoint2(int(-fm.Height()/2), int(-fm.Width(title)/2)), qt.NewQStaticText2(title))
	p.Restore()

	// ---- X grid every 10s (clipped) ----
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

	// ---- series (clipped; cosmetic pen for HiDPI) ----
	p.Save()
	p.SetClipRect3(plotRect, qt.ReplaceClip)
	for i, host := range g.model.Hosts() {
		tmp := host.buf.Snapshot(nil)
		if len(tmp) == 0 {
			continue
		}

		col := seriesColor(i)
		pen := qt.NewQPen3(col)
		pen.SetCosmetic(true)
		pen.SetWidthF(2.0)
		p.SetPenWithPen(pen)

		var path *qt.QPainterPath
		var havePath bool

		for _, s := range tmp {
			if s.T.Before(startT) {
				continue
			}
			x := mapX(s.T, startT, now, left, right)

			switch s.State {
			case SampleOK:
				y := mapY(s.MS, yMin, yMax, top, bottom)
				if !havePath {
					path = qt.NewQPainterPath2(qt.NewQPointF3(x, y))
					havePath = true
				} else {
					path.LineTo(qt.NewQPointF3(x, y))
				}

			case SampleLoss:
				// flush any existing path before disjoint marker
				if havePath && path != nil {
					p.DrawPath(path)
					havePath = false
					path = nil
				}
				// short tick at top
				tk := qt.NewQPainterPath2(qt.NewQPointF3(x, top))
				tk.LineTo(qt.NewQPointF3(x, top+12))
				p.DrawPath(tk)

			case SampleLate:
				// flush path before disjoint marker
				if havePath && path != nil {
					p.DrawPath(path)
					havePath = false
					path = nil
				}
				r := 3.0
				y := mapY(s.MS, yMin, yMax, top, bottom)
				red := qcolor(255, 0, 0, 255)
				pr := qt.NewQPen3(red)
				pr.SetCosmetic(true)
				pr.SetWidthF(1.5)
				p.SetPenWithPen(pr)
				// hollow square
				rect := qt.NewQRectF4(x-r, y-r, 2*r, 2*r)
				// draw square via path
				box := qt.NewQPainterPath2(qt.NewQPointF3(rect.X(), rect.Y()))
				box.LineTo(qt.NewQPointF3(rect.X()+rect.Width(), rect.Y()))
				box.LineTo(qt.NewQPointF3(rect.X()+rect.Width(), rect.Y()+rect.Height()))
				box.LineTo(qt.NewQPointF3(rect.X(), rect.Y()+rect.Height()))
				box.LineTo(qt.NewQPointF3(rect.X(), rect.Y()))
				p.DrawPath(box)
				// restore main pen
				p.SetPenWithPen(pen)
			}
		}
		if havePath && path != nil {
			p.DrawPath(path)
		}
	}
	p.Restore()

	// ---- legend (outside clip, left top) ----
	legendY := top + 2
	for i, host := range g.model.Hosts() {
		col := seriesColor(i)
		chip := qt.NewQRectF4(left+4, legendY+float64(i*18), 12, 10)
		p.FillRect4(chip, col)
		lbl := qt.NewQStaticText2(host.Name + " (" + host.Addr + ")")
		p.SetPen(txt)
		p.DrawStaticText2(qt.NewQPoint2(int(left+22), int(legendY+float64(i*18))), lbl)
	}

	// ---- X time labels (clamped + no overlap) ----
	p.SetPen(txt)
	prevRight := left - 6
	for _, t := range xTicks {
		x := mapX(t, startT, now, left, right)
		text := t.Format("15:04:05")
		tw := fm.Width(text)

		posX := x - tw/2
		if posX < left {
			posX = left
		}
		if posX+tw > right {
			posX = right - tw
		}
		if posX < prevRight+6 {
			continue
		}
		p.DrawStaticText2(qt.NewQPoint2(int(posX), int(bottom+4)), qt.NewQStaticText2(text))
		prevRight = posX + tw
	}

	// ---- hover crosshair + readout (crosshair clipped; tooltip outside) ----
	if g.mouseInside && g.mouseX >= int(left) && g.mouseX <= int(right) {
		x := float64(g.mouseX)
		// crosshair inside plot
		p.Save()
		p.SetClipRect3(plotRect, qt.ReplaceClip)
		cs := qt.NewQColor()
		cs.SetRgb2(txt.Red(), txt.Green(), txt.Blue(), 80)
		p.SetPen(cs)
		ch := qt.NewQPainterPath2(qt.NewQPointF3(x, top))
		ch.LineTo(qt.NewQPointF3(x, bottom))
		p.DrawPath(ch)
		p.Restore()

		// nearest per series + tooltip text lines
		tAtX := unmapX(x, startT, now, left, right)
		boxLeft := x + 8
		if boxLeft > right-200 {
			boxLeft = right - 200
		}
		boxTop := top + 8

		lines := []string{tAtX.Format("15:04:05")}
		for i, host := range g.model.Hosts() {
			tmp := host.buf.Snapshot(nil)
			if len(tmp) == 0 {
				continue
			}
			best := Sample{}
			bestDT := time.Duration(1<<62 - 1)
			for _, s := range tmp {
				dt := s.T.Sub(tAtX)
				if dt < 0 {
					dt = -dt
				}
				if dt < bestDT {
					bestDT = dt
					best = s
				}
			}
			val := "loss"
			if best.MS >= 0 {
				val = fmt.Sprintf("%.0f ms", best.MS)
			}
			lines = append(lines, fmt.Sprintf("%s: %s", host.Name, val))

			// small dot marker inside plot
			if best.MS >= 0 {
				y := mapY(best.MS, yMin, yMax, top, bottom)
				p.Save()
				p.SetClipRect3(plotRect, qt.ReplaceClip)
				p.FillRect4(qt.NewQRectF4(mapX(best.T, startT, now, left, right)-2, y-2, 4, 4), seriesColor(i))
				p.Restore()
			}
		}

		// draw tooltip box
		boxW := 150.0
		boxH := float64(16*len(lines) + 8)
		p.FillRect4(qt.NewQRectF4(boxLeft, boxTop, boxW, boxH), qcolor(0, 0, 0, 160))
		p.SetPen(qcolor(255, 255, 255, 220))
		for i, s := range lines {
			lbl := qt.NewQStaticText2(s)
			p.DrawStaticText2(qt.NewQPoint2(int(boxLeft+6), int(boxTop+6+float64(16*i))), lbl)
		}
	}
}
