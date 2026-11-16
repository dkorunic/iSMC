// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// m3FtIndex is the base M3 SMC index read from report-m3.txt (Mac15,13), where
// every entry is byte-identical to its source key.
var m3FtIndex = map[string]string{
	"ftP0": "Tp01", "ftP1": "Tp05", "ftP2": "Tp09", "ftP3": "Tp0D", "ftP4": "Tp0H", "ftP5": "Tp0L",
	"ftP6": "Tp0P", "ftP7": "Tp0T", "ftP8": "Tp0X", "ftP9": "Tp0b", "ftPa": "Tp0f", "ftPb": "Tp0j",
	"ftPc": "Tp0n", "ftPd": "Tp0r", "ftPe": "Tp0v", "ftPf": "Tp0z", "ftPg": "Tp13", "ftPh": "Tp17",
	"ftE0": "Te05", "ftE1": "Te0L",
	"ftG0": "Tg0D", "ftG1": "Tg0P", "ftG2": "Tg0X", "ftG3": "Tg0b", "ftG4": "Tg0j", "ftG5": "Tg0v",
}

// m3ProFtIndex is the M3 Pro index read from report-m3-pro-5.txt (MacBook Pro
// 14 6P+6E, idle), byte-identical to its sources. Its order is not key order.
var m3ProFtIndex = map[string]string{
	"ftP0": "Tp0V", "ftP1": "Tp05", "ftP2": "Tp0b", "ftP3": "Tp0D", "ftP4": "Tp1J", "ftP5": "Tp17",
	"ftP6": "Tp1R", "ftP7": "Tp1F", "ftP8": "Tp0h", "ftP9": "Tp0L", "ftPa": "Tp0n", "ftPb": "Tp0S",
	"ftPc": "Tp0v", "ftPd": "Tp0z",
	"ftE0": "Te05", "ftE1": "Te0H", "ftE2": "Te0S",
	"ftG0": "Tg01", "ftG1": "Tg05", "ftG2": "Tg0D", "ftG3": "Tg0L", "ftG4": "Tg0V", "ftG5": "Tg0v",
	"ftG6": "Tg13", "ftG7": "Tg1B", "ftG8": "Tg1l",
}

// withFtIndex loads a saved run and adds the index keys to every reading, as
// a run saved by a guess that reads the index would hold them.
func withFtIndex(t *testing.T, run string, index map[string]string) *guessCapture {
	t.Helper()

	c, err := loadCapture("testdata/" + run + ".json")
	if err != nil {
		t.Fatal(err)
	}

	add := func(m map[string]float32) {
		for f, src := range index {
			if v, ok := m[src]; ok {
				m[f] = v
			}
		}
	}

	for _, p := range append(slices.Clone(c.Phases), *c.GPU) {
		add(p.Baseline)
		add(p.Hot)

		for _, s := range p.Series {
			add(s.Temps)
		}
	}

	return c
}

func TestResolveFtIndex(t *testing.T) {
	t.Parallel()

	c := withFtIndex(t, "m3-air-run", m3FtIndex)

	got := resolveFtIndex(c)
	for f, want := range m3FtIndex {
		if got[f] != want {
			t.Errorf("resolveFtIndex[%s] = %q, want %q", f, got[f], want)
		}
	}

	stripFtKeys(c)

	for _, s := range captureSnapshots(c) {
		for k := range s {
			if strings.HasPrefix(k, ftPrefix) {
				t.Fatalf("stripFtKeys left %s", k)
			}
		}
	}

	if resolveFtIndex(c) != nil {
		t.Error("resolveFtIndex without index keys should return nil")
	}
}

// TestReplayM3FtIndex: with the index, the base M3 bank and cluster maxima
// and the Te0O/Te0P pair leave the core tiers, and the E tier keeps only the
// two indexed Te groups instead of being filled with Tp16-18.
func TestReplayM3FtIndex(t *testing.T) {
	c := withFtIndex(t, "m3-air-run", m3FtIndex)
	out := captureStdout(t, func() { analyzeCapture(c) })
	cores := strings.Join(regexp.MustCompile(`(?m)^CPU \w+ Core \d+:\w+:\w+`).FindAllString(out, -1), "\n")

	for _, k := range []string{"Tp1H", "Tp1I", "Tp1K", "Tp1L", "Te0O", "Te0P"} {
		if strings.Contains(cores, ":"+k+":") {
			t.Errorf("%s still listed as a core:\n%s", k, cores)
		}
	}

	if e := regexp.MustCompile(`(?m)^CPU Efficiency Core \d+:(\w+)`).FindAllStringSubmatch(out, -1); len(e) != 6 ||
		slices.ContainsFunc(e, func(m []string) bool { return !strings.HasPrefix(m[1], "Te") }) {
		t.Errorf("E tier = %v, want the six keys of Te04-06 and Te0K-M", e)
	}

	if !strings.Contains(out, "ftE: ftE0=Te05 ftE1=Te0L") {
		t.Errorf("index section missing:\n%s", out)
	}
}

// TestReplayM3ProFtIndex: with the index, the bank maxima Tp3O/P and Tp3W/X
// leave the P tier, the GPU-side probes Tp0u-w/Tp0y-10 are cut from it, and
// the E tier is not filled with P triplets (Tp0g, Tp0y, Tp1Q without it).
func TestReplayM3ProFtIndex(t *testing.T) {
	c := withFtIndex(t, "m3-pro-run", m3ProFtIndex)

	// The run holds no Tg* keys, so only the P and E entries can resolve.
	got := resolveFtIndex(c)
	for f, want := range m3ProFtIndex {
		if f[2] != 'G' && got[f] != want {
			t.Errorf("resolveFtIndex[%s] = %q, want %q", f, got[f], want)
		}
	}

	out := captureStdout(t, func() { analyzeCapture(c) })

	p := regexp.MustCompile(`(?m)^CPU Performance Core \d+:(\w+)`).FindAllStringSubmatch(out, -1)
	if len(p) != 36 || slices.ContainsFunc(p, func(m []string) bool {
		return strings.HasPrefix(m[1], "Tp3") || strings.HasPrefix(m[1], "Tp0u") || strings.HasPrefix(m[1], "Tp0y")
	}) {
		t.Errorf("P tier = %v, want the 36 keys of the twelve indexed core triplets", p)
	}

	if e := regexp.MustCompile(`(?m)^CPU Efficiency Core \d+:(\w+)`).FindAllStringSubmatch(out, -1); slices.ContainsFunc(e,
		func(m []string) bool { return !strings.HasPrefix(m[1], "Te") }) {
		t.Errorf("E tier holds non-Te keys: %v", e)
	}
}

// TestReplayM1UltraFtIndex: a live Mac Studio M1 Ultra run read with the
// index. ftP lags its sources by up to ~5 °C while load changes, yet every
// entry resolves to probe B of one of the 20 core triplets, so no core group
// is dropped (16 P + 4 E as without the index).
func TestReplayM1UltraFtIndex(t *testing.T) {
	t.Parallel()

	c, err := loadCapture("testdata/m1-ultra-ft-run.json")
	if err != nil {
		t.Fatal(err)
	}

	idx := resolveFtIndex(c)
	want := strings.Fields("Tp01 Tp05 Tp09 Tp0D Tp0H Tp0L Tp0P Tp0T Tp0X Tp0b " +
		"Tp21 Tp25 Tp29 Tp2D Tp2H Tp2L Tp2P Tp2T Tp2X Tp2b")

	for i, src := range want {
		if k := "ftP" + string("0123456789abcdefghij"[i]); idx[k] != src {
			t.Errorf("idx[%s] = %q, want %q", k, idx[k], src)
		}
	}

	if idx["ftE0"] != "Te01" || idx["ftE1"] != "Te21" {
		t.Errorf("ftE = %q, %q, want Te01, Te21", idx["ftE0"], idx["ftE1"])
	}

	if g := unindexedGroups([][]string{{"Tp00", "Tp01", "Tp02"}, {"Tp2a", "Tp2b", "Tp2c"}}, idx); g != nil {
		t.Errorf("unindexedGroups dropped %v", g)
	}
}

// TestResolveFtIndexMax: on the M3 Max run bank B wakes beside bank A under
// load, so only exact matches tie the index to its sources. ftP0-ftPd are the
// M3 Pro bank-A list (byte-identical in report-m3-max-2.txt) and ftPe-ftPr
// bank B (40.0 in the dump, exact in the run's live snapshots). M4 Max ftPN
// copies the SSD key TH0x and is not a tier entry.
func TestResolveFtIndexMax(t *testing.T) {
	t.Parallel()

	c, err := loadCapture("testdata/m3-max-run.json")
	if err != nil {
		t.Fatal(err)
	}

	idx := resolveFtIndex(c)
	want := strings.Fields("Tp0V Tp05 Tp0b Tp0D Tp1J Tp17 Tp1R Tp1F Tp0h Tp0L Tp0n Tp0S Tp0v Tp0z " +
		"Tp2A Tp1h Tp2I Tp1p Tp34 Tp2s Tp3C Tp30 Tp2Q Tp1y Tp2Y Tp26 Tp2g Tp2k")

	for i, src := range want {
		if k := "ftP" + string("0123456789abcdefghijklmnopqr"[i]); idx[k] != src {
			t.Errorf("M3 Max idx[%s] = %q, want %q", k, idx[k], src)
		}
	}

	c, err = loadCapture("testdata/m4-max-run.json")
	if err != nil {
		t.Fatal(err)
	}

	if src, ok := resolveFtIndex(c)["ftPN"]; ok {
		t.Errorf("M4 Max ftPN resolved to %s, want it skipped", src)
	}
}
