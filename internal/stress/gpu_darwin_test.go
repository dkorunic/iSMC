// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package stress

import (
	"testing"
	"time"
)

// Test_StartGPU checks the Metal kernel compiles and stop returns promptly.
func Test_StartGPU(t *testing.T) {
	stop, err := StartGPU()
	if err != nil {
		t.Skip(err)
	}

	time.Sleep(500 * time.Millisecond)

	start := time.Now()

	stop()

	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("stop took %v; a single dispatch is too long", d)
	}
}
