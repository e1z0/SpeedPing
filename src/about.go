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
	"runtime"
	"strings"
	"time"

	"github.com/mappu/miqt/qt"
)

var (
	AppName    = "SpeedPing"
	appName    = "speedping" // used in configs file path'es etc...
	AppVersion = "dev"
	BuildDate  = ""
	build      = ""
	LicenseStr = `This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License v3 (GPL-3.0).`
)

func NewAboutPage(model *AppModel) *qt.QWidget {
	page := qt.NewQWidget(nil)
	col := qt.NewQVBoxLayout(nil)
	page.SetLayout(col.QLayout)

	// Title
	title := qt.NewQLabel6(fmt.Sprintf("%s", AppName), nil, 0)
	titleFont := title.Font()
	titleFont.SetPointSize(titleFont.PointSize() + 10)
	titleFont.SetBold(true)
	title.SetFont(titleFont)
	title.SetAlignment(qt.AlignHCenter)

	// Version/build line
	buildBits := []string{}
	if AppVersion != "" {
		buildBits = append(buildBits, "v"+AppVersion)
	}
	if build != "" {
		buildBits = append(buildBits, "build "+build)
	}
	if BuildDate != "" {
		buildBits = append(buildBits, BuildDate)
	}
	buildLine := qt.NewQLabel6(strings.Join(buildBits, " • "), nil, 0)
	buildLine.SetAlignment(qt.AlignHCenter)

	// Links (QLabel rich text; opens in browser)
	links := qt.NewQLabel6(
		`<div style="text-align:center">
<a href="https://github.com/e1z0/SpeedPing">GitHub</a> &nbsp;•&nbsp;
<a href="https://opensource.org/license/gpl-3-0">GPL-3.0</a>
</div>`, nil, 0)
	links.SetTextFormat(qt.RichText)
	links.SetOpenExternalLinks(true)
	links.SetAlignment(qt.AlignHCenter)

	copyright := qt.NewQLabel3("Copyright (c) 2025 Justinas K <e1z0@icloud.com>")
	copyright.SetAlignment(qt.AlignHCenter)

	// License short blurb
	lic := qt.NewQTextEdit(nil)
	lic.SetReadOnly(true)
	lic.SetMinimumHeight(90)
	lic.SetPlainText(LicenseStr)

	// System info
	sysInfo := qt.NewQTextEdit(nil)
	sysInfo.SetReadOnly(true)
	sysInfo.SetMinimumHeight(110)
	sysInfo.SetPlainText(makeSystemInfo())

	// Buttons row
	btnRow := qt.NewQHBoxLayout(nil)
	btnOpenCfg := qt.NewQPushButton(nil)
	btnOpenCfg.SetText("Open config folder")
	btnOpenLog := qt.NewQPushButton(nil)
	btnOpenLog.SetText("Open logs folder")
	btnCopySys := qt.NewQPushButton(nil)
	btnCopySys.SetText("Copy system info")
	btnRow.AddStretch()
	btnRow.AddWidget(btnOpenCfg.QWidget)
	btnRow.AddWidget(btnOpenLog.QWidget)
	btnRow.AddWidget(btnCopySys.QWidget)
	btnRow.AddStretch()

	// Add widgets
	col.AddSpacing(10)
	col.AddWidget(title.QWidget)
	col.AddWidget(buildLine.QWidget)
	col.AddWidget(links.QWidget)
	col.AddWidget(copyright.QWidget)
	col.AddSpacing(8)
	col.AddWidget(qt.NewQLabel6("License", nil, 0).QWidget)
	col.AddWidget(lic.QWidget)
	col.AddWidget(qt.NewQLabel6("System info", nil, 0).QWidget)
	col.AddWidget(sysInfo.QWidget)
	col.AddLayout(btnRow.QLayout)
	col.AddStretch()

	// Actions
	btnOpenCfg.OnClicked(func() {
		openFileOrDir(configDir())
	})
	btnOpenLog.OnClicked(func() {
		openFileOrDir(logsDir())
	})
	btnCopySys.OnClicked(func() {
		cb := qt.QGuiApplication_Clipboard()
		cb.SetText2(makeSystemInfo(), qt.QClipboard__Clipboard)
	})

	return page
}

func makeSystemInfo() string {
	now := time.Now().Format(time.RFC3339)
	return fmt.Sprintf(
		"App:      %s v%s (build: %s) %s\nGo:        %s\nOS/Arch:   %s/%s\nCPU:       %d\nTime:      %s\nBinary:       %s\nConfig:    %s\nLogs:      %s\n",
		AppName, AppVersion, build, BuildDate,
		runtime.Version(), runtime.GOOS, runtime.GOARCH,
		runtime.NumCPU(), now,
		exePath(), configDir(), logsDir(),
	)
}
