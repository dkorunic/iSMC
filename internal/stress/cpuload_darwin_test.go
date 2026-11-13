// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package stress

import (
	"math"
	"testing"
)

func TestBusyShare(t *testing.T) {
	t.Parallel()

	before := []CPUTicks{{Busy: 100, Total: 200}, {Busy: math.MaxUint32 - 9, Total: math.MaxUint32 - 9}}
	// CPU 0: 50 busy of 100; CPU 1: counters wrapped, 20 busy of 20.
	after := []CPUTicks{{Busy: 150, Total: 300}, {Busy: 10, Total: 10}}

	got := BusyShare(before, after)
	if len(got) != 2 || got[0] != 0.5 || got[1] != 1 {
		t.Fatalf("BusyShare = %v, want [0.5 1]", got)
	}

	if BusyShare(before, after[:1]) != nil {
		t.Error("CPU count mismatch must return nil")
	}
}

func TestReadCPUTicks(t *testing.T) {
	t.Parallel()

	if ticks := ReadCPUTicks(); len(ticks) == 0 {
		t.Fatal("ReadCPUTicks returned no CPUs")
	}
}
