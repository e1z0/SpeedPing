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
	"os"

	"github.com/mappu/miqt/qt"
)

var debugging string
var DEBUG bool
var globalIcon *qt.QIcon

func main() {
	qt.NewQApplication(os.Args)
	pixmap := qt.NewQPixmap()
	pixmap.Load(":/icon.png")
	globalIcon = qt.NewQIcon2(pixmap)
	qt.QApplication_SetWindowIcon(globalIcon)
	qt.QGuiApplication_SetWindowIcon(globalIcon)

	cfg, _ := LoadConfig()
	model := NewAppModel()
	model.LoadFromConfig(cfg)

	ui := NewUI(model)

	// restore window geometry if we have saved it already
	if cfg.Window.W > 0 && cfg.Window.H > 0 {
		ui.main.Resize(cfg.Window.W, cfg.Window.H)
	}
	if cfg.Window.X != 0 || cfg.Window.Y != 0 {
		ui.main.Move(cfg.Window.X, cfg.Window.Y)
	}

	ui.main.OnCloseEvent(func(super func(*qt.QCloseEvent), e *qt.QCloseEvent) {
		// snapshot geometry
		geo := WindowConfig{
			X: ui.main.X(),
			Y: ui.main.Y(),
			W: ui.main.Width(),
			H: ui.main.Height(),
		}
		_ = SaveConfig(model.SnapshotConfig(geo))
		super(e)
	})

	ui.Show()
	IgnoreSignum()
	qt.QApplication_Exec()
}

// entrypoint for runtime variables initialization
func init() {
	InitializeEnvironment()
}
