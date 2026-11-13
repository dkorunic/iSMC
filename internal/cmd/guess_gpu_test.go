// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout returns what fn prints to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdout := os.Stdout
	os.Stdout = w

	fn()

	os.Stdout = stdout

	w.Close()

	b, _ := io.ReadAll(r)

	return string(b)
}

// TestPrintGPUPhase checks GPU-tier classification and the src/temp.txt label
// flags against real M1 Pro rows (Tg05/Tg0C/Tg0D are GPU, Tp01 a P-core, Tm06
// memory). Max GPU rise is 9, so the floor is 4.5 °C.
func TestPrintGPUPhase(t *testing.T) {
	gpu := map[string]float32{
		"Tg05": 9, // GPU-tier, GPU label → no flag
		"Tp01": 6, // GPU-tier, CPU label → flag
		"Tg0D": 3, // tracks CPU as much, GPU label → flag
		"Tm06": 2, // CPU never warms it, but below the floor → counted only
	}
	cpu := []phaseResult{{deltas: map[string]float32{"Tg05": 2, "Tp01": 1, "Tg0D": 4}}}
	visible := map[string]float32{"Tg05": 40, "Tp01": 40, "Tg0D": 40, "Tg0C": 40}

	out := captureStdout(t, func() { printGPUPhase("M1", "M1 Pro", gpu, visible, cpu) })

	line := func(key string) string {
		for l := range strings.SplitSeq(out, "\n") {
			if strings.HasPrefix(l, "//   "+key+" ") {
				return l
			}
		}

		t.Fatalf("no line for %s in:\n%s", key, out)

		return ""
	}

	for key, want := range map[string][]string{
		"Tg05": {"GPU-tier", `"GPU 1"`},
		"Tp01": {"GPU-tier", "⚠ GPU-tier but label is not GPU"},
		"Tg0D": {"shared", "⚠ GPU label but not GPU-tier"},
	} {
		l := line(key)
		for _, w := range want {
			if !strings.Contains(l, w) {
				t.Errorf("%s line %q lacks %q", key, l, w)
			}
		}
	}

	if strings.Contains(line("Tg05"), "⚠") {
		t.Errorf("Tg05 should not be flagged: %q", line("Tg05"))
	}

	if strings.Contains(out, "//   Tm06 ") || !strings.Contains(out, "1 other responder(s)") {
		t.Errorf("Tm06 should be counted, not listed:\n%s", out)
	}

	if !strings.Contains(out, "silent under GPU load (1): Tg0C") {
		t.Errorf("Tg0C (visible, GPU label, no rise) missing from silent list:\n%s", out)
	}

	if strings.Index(out, "Tg05") > strings.Index(out, "Tp01") {
		t.Errorf("responders not sorted by GPU rise:\n%s", out)
	}
}
