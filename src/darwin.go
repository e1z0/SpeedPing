//go:build darwin
// +build darwin

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
	"syscall"
)

/*
#include <stdint.h>
#include <stdio.h>

#ifdef __cplusplus
#include <csignal>
#else
#include <signal.h>
#endif

void Ignore(int sigNum);

void Ignore(int sigNum) {
    struct sigaction sa;
    sa.sa_handler = SIG_DFL;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags |= SA_ONSTACK;
    sigaction(sigNum, &sa, NULL);
}

*/
import "C"

func Ignore(sigNum syscall.Signal) {
	C.Ignore(C.int(sigNum))
}

func IgnoreSignum() {
	Ignore(syscall.SIGURG)
}
