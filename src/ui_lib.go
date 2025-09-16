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

// ------ drawing helpers ------
func qcolor(r, g2, b, a int) *qt.QColor {
	c := qt.NewQColor()
	// QColor::setRgb accepts 0..255
	c.SetRgb2(r, g2, b, a)
	return c
}

var palette = [][]int{
	{90, 180, 255},  // azure
	{255, 120, 120}, // salmon
	{255, 200, 80},  // amber
	{120, 230, 140}, // mint
	{200, 140, 255}, // purple
	{80, 220, 200},  // teal
	{255, 160, 220}, // pink
	{160, 200, 255}, // light blue
}

func seriesColor(i int) *qt.QColor {
	c := palette[i%len(palette)]
	return qcolor(c[0], c[1], c[2], 255)
}

// 1–2–5 tick generator within [min, max]
func niceTicks(min, max float64, target int) []float64 {
	if target < 2 {
		target = 2
	}
	if max <= min {
		max = min + 1
	}
	raw := (max - min) / float64(target)
	step := niceStep(raw)
	start := float64(int(min/step)) * step
	if start < min {
		start += step
	}
	var out []float64
	for v := start; v <= max+1e-9; v += step {
		out = append(out, roundTo(v, step))
	}
	return out
}

func niceStep(x float64) float64 {
	pow := 1.0
	for x > 10 {
		x /= 10
		pow *= 10
	}
	for x < 1 {
		x *= 10
		pow /= 10
	}
	switch {
	case x <= 1.2:
		return 1 * pow
	case x <= 2.5:
		return 2 * pow
	case x <= 5:
		return 5 * pow
	default:
		return 10 * pow
	}
}

func roundTo(v, step float64) float64 {
	if step == 0 {
		return v
	}
	n := int((v / step) + 0.5)
	return float64(n) * step
}

func formatMS(ms float64) string {
	if ms < 1 {
		return "0"
	}
	return fmt.Sprintf("%.0f", ms)
}

func mapX(t time.Time, start, end time.Time, left, right float64) float64 {
	if !t.After(start) {
		return left
	}
	if !t.Before(end) {
		return right
	}
	span := end.Sub(start).Seconds()
	if span <= 0 {
		return right
	}
	return left + (right-left)*t.Sub(start).Seconds()/span
}

func unmapX(x float64, start, end time.Time, left, right float64) time.Time {
	if x <= left {
		return start
	}
	if x >= right {
		return end
	}
	span := end.Sub(start)
	ratio := (x - left) / (right - left)
	return start.Add(time.Duration(ratio * float64(span)))
}

func mapY(v, minV, maxV, top, bottom float64) float64 {
	if maxV <= minV {
		return bottom
	}
	rel := (v - minV) / (maxV - minV)
	return bottom - rel*(bottom-top)
}
