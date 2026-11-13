// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package smc

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/dkorunic/iSMC/gosmc"
	"github.com/dkorunic/iSMC/internal/platform"
)

// Test_DumpReplay replays every internal/reports dump through the production
// temperature mapping for the dump's platform and checks, per dump:
//
//	cores    distinct live "CPU {Performance,Efficiency,Super} Core N" rows fall
//	         inside the skuLayouts min–max for the SKU;
//	collide  no description hides a hotter distinct key behind the last-written
//	         one (last-write-wins in addGeneric);
//	unmapped every live T* key resolves to some description.
//
// Known violations live in replayAllow; the test fails on new violations and on
// allowlist entries that no longer fire, so the list can only shrink.
func Test_DumpReplay(t *testing.T) {
	const collideTol = 0.5 // °C; probe A/B vs max spread on a live triplet.

	coreRe := regexp.MustCompile(`^CPU (Performance|Efficiency|Super) Core \d+$`)

	files, err := filepath.Glob("../internal/reports/report-*.txt")
	if err != nil || len(files) == 0 {
		t.Fatalf("no dumps found: %v", err)
	}

	for _, f := range files {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "report-"), ".txt")

		t.Run(name, func(t *testing.T) {
			// report-<family>[-<pro|max|ultra>][-<seq>].txt
			tok := strings.Split(name, "-")
			family := strings.ToUpper(tok[0][:1]) + tok[0][1:]
			sku := family

			if len(tok) > 1 && slices.Contains([]string{"pro", "max", "ultra"}, tok[1]) {
				sku += " " + strings.ToUpper(tok[1][:1]) + tok[1][1:]
			}

			if family == "A18" {
				sku = "A18 Pro" // only A18 SKU; filename predates the convention.
			}

			live := liveTemps(t, f)
			got := map[string]bool{}

			// unmapped
			mapped := MappedTempKeys(family, sku)

			for k := range live {
				if _, ok := mapped[k]; !ok {
					got["unmapped "+k] = true
				}
			}

			// Replay addGeneric order: last live key written per description.
			var rows []SensorStat

			for _, s := range AppleTemp {
				if platformMatches(s.Platform, family, sku) {
					rows = append(rows, s)
				}
			}

			last := map[string]string{}
			hottest := map[string]float32{}

			forEachKey(rows, func(key, desc string) {
				if v, ok := live[key]; ok {
					last[desc] = key
					hottest[desc] = max(hottest[desc], v)
				}
			})

			cores := map[string]int{}

			for desc, key := range last {
				if hottest[desc]-live[key] > collideTol {
					got["collide "+desc] = true
				}

				if m := coreRe.FindStringSubmatch(desc); m != nil {
					cores[m[1][:1]]++
				}
			}

			if layout, ok := platform.LookupSKULayout(sku); ok {
				for _, c := range []struct {
					typ    string
					lo, hi int
				}{
					{"P", layout.PCoresMin, layout.PCoresMax},
					{"E", layout.ECoresMin, layout.ECoresMax},
					{"S", layout.SCoresMin, layout.SCoresMax},
				} {
					if n := cores[c.typ]; n < c.lo || n > c.hi {
						got[fmt.Sprintf("cores %s=%d", c.typ, n)] = true
					}
				}
			} else if family != "Intel" {
				t.Errorf("no skuLayouts entry for %q", sku)
			}

			want := map[string]bool{}

			for _, e := range replayAllow[name] {
				if keys, ok := strings.CutPrefix(e, "unmapped "); ok {
					for k := range strings.FieldsSeq(keys) {
						want["unmapped "+k] = true
					}

					continue
				}

				want[e] = true
			}

			for v := range got {
				if !want[v] {
					t.Errorf("new violation (sku %s): %s", sku, v)
				}
			}

			for v := range want {
				if !got[v] {
					t.Errorf("stale allowlist entry, remove it: %s", v)
				}
			}
		})
	}
}

var dumpLineRe = regexp.MustCompile(`^\s+(T\S{3})\s+\[(.{4})\].*\(bytes ([0-9a-f ]+)\)`)

// liveTemps decodes every T* key of a dump from its bytes (the inline value is
// unreliable) and returns those inside the isValidReading window.
func liveTemps(t *testing.T, path string) map[string]float32 {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	live := map[string]float32{}

	for line := range strings.Lines(string(data)) {
		m := dumpLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		b, err := hex.DecodeString(strings.ReplaceAll(m[3], " ", ""))
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}

		var raw gosmc.SMCBytes

		copy(raw[:], b)

		//nolint:gosec // G115: hex payload of one dump line, never near 2^32 bytes
		k := RawKey{Key: m[1], DataType: strings.TrimSpace(m[2]), DataSize: uint32(len(b)), Bytes: raw}

		if v, ok := RawKeyToFloat32(k); ok && isValidReading(v, TempUnit) {
			live[k.Key] = v
		}
	}

	return live
}

// Shared allowlist fragments; each names keys a dump exposes but that cannot be
// labelled honestly from the dumps alone.
const (
	// Clock-gated placeholders pinned at exactly 40.00 °C in both M3 Max dumps.
	m3MaxGated = "unmapped Tp1g Tp1h Tp1i Tp1o Tp1p Tp1q Tp1x Tp1y Tp1z Tp25 Tp26 Tp27 Tp29 Tp2A " +
		"Tp2B Tp2H Tp2I Tp2J Tp2P Tp2Q Tp2R Tp2X Tp2Y Tp2Z Tp2f Tp2g Tp2h Tp2j Tp2k Tp2l " +
		"Tp2r Tp2s Tp2t Tp2z Tp30 Tp31 Tp33 Tp34 Tp35 Tp3B Tp3C Tp3D Tp3a Tp3b Tp3e Tp3f Tp3i Tp3j"
	// Tp0u/v/w alias Tp0y/z/10 and Te0V aliases Te0T (dropped per src/temp.txt).
	m3Aliases    = "unmapped Tp0u Tp0v Tp0w"
	m3ProAliases = m3Aliases + " Te0V"
	// Virtual TV* channels with no known meaning.
	m3MaxVirtual = "unmapped TV0h TV0m TV0s TV1h TV1m TV1s TVFh TVFm TVH3 TVHC TVSC TVsa TVss"
	m3ProVirtual = "unmapped TVHD TVHE TVHF TVHG TVHH TVHO"
	// M3 Pro triplets map to 9 "cores" (expected 5-6) and E-cores stop at 5
	// (Te0U has no triplet; Te0V aliases Te0T). Twelve live triplets in two
	// shapes (B = A + 6.1 / + 7.3 °C) exist on both M3 Pro and M3 Max, so the
	// dumps cannot say which are physical cores.
	m3ProP = "cores P=9"
	m3ProE = "cores E=5"
	// Same twelve triplets on M3 Max; only 9 carry core labels (expected 10-12)
	// and nothing in the dumps says which cluster triplets are the missing cores.
	m3MaxP = "cores P=9"
	// Tp0y/z/10 ("Core 9"): third < probe B in all five M3 dumps, loaded or idle.
	m3Core9 = "collide CPU Performance Core 9"
	// Only Te04/Te0R triplets are live on M4 Pro/Max (Tpx8-D read 0 in all four
	// dumps); no other key carries per-E-core data.
	m4ProMaxE = "cores E=2"
	// Register aliases removed from src/temp.txt: TVDP/TVDc ≡ TVD0, Tm0y/Tm2T ≡
	// Tp0a, Ts0G ≡ the Ta00-Ta0R band (see Test_M5Pro18CoreMapping).
	m5Aliases = "unmapped TVDP TVDc Tm0y Tm2T Ts0G"
)

// replayAllow lists the violations Test_DumpReplay currently tolerates, keyed
// by dump name (report-<name>.txt). Entries are "cores <T>=<n>",
// "collide <description>" or "unmapped <key> [<key>...]".
var replayAllow = map[string][]string{
	// Intel T2 Tc/Te/Th/Tp/Ts a/b/x/z quads, Apple-tagged virtual keys and the SSD
	// controllers: no Intel-family reference to label them with confidence.
	"intel-t2": {
		"unmapped TCMb TCMz TH0T TH0x TVA0 TVD0 TVS0 TVS1 TVSx Ta1z TaMP Tc0a Tc0b Tc0x",
		"unmapped Tc0z Tc7a Tc7b Tc7x Tc7z Tc8a Tc8b Tc8x Tc8z Tc9a Tc9b Tc9x Tc9z Tcaa",
		"unmapped Tcab Tcax Tcaz Te0a Te0b Te0x Te0z Te3a Te3b Te3x Te3z Th0a Th0b Th0x",
		"unmapped Th0z Th1a Th1b Th1x Th1z Th2a Th2b Th2x Th2z Tp2a Tp2b Tp2x Tp2z Tp3a",
		"unmapped Tp3b Tp3x Tp3z Tp4a Tp4b Tp4x Tp4z Tp5a Tp5b Tp5x Tp5z Tp7a Tp7b Tp7x",
		"unmapped Tp7z Tp8a Tp8b Tp8x Tp8z Tp9a Tp9b Tp9x Tp9z Ts0P Ts0a Ts0b Ts0x Ts0z",
		"unmapped Ts1P Ts1a Ts1b Ts1x Ts1z Ts2a Ts2b Ts2x Ts2z",
	},
	// Single-dump keys with no known meaning; Tc02 ≡ TCMz and Tc05 ≡ TCMb suggest
	// die-2 CPU max/average, but one dump is not enough to label them.
	"m1-ultra": {"unmapped TB0p TB1p TIDP TMVC TSVL Tc01 Tc02 Tc05 Tc06"},
	// Every Tp0* key reads a sentinel (0-8.4 °C) in the only dump; Te04-06 is a
	// single E-cluster aggregate.
	"m2-max": {"cores E=0", "cores P=0"},
	// Four E-cores share the Te04-06 cluster aggregate; Tp0a/b/c third < probe B
	// (documented cluster aggregate); TaMP/Th1H unknown, single dump.
	"m2-pro": {"cores E=0", "collide CPU Performance Cluster Aggregate 1", "unmapped TaMP Th1H"},
	// Loaded dump: cluster 3 mixes Tp16-18 with Tp3S/T where Tp3T ≡ Tp10.
	"m3-max": {m3MaxP, m3Core9, "collide CPU Performance Cluster 3", m3Aliases, m3MaxVirtual, m3MaxGated},
	// Idle dump: third-of-triplet < probe B on most P triplets, so last-write
	// shows the cooler value; not a distinct sensor.
	"m3-max-2": {
		m3MaxP, m3Core9, m3Aliases, m3MaxVirtual, m3MaxGated,
		"collide CPU Performance Cluster 1", "collide CPU Performance Cluster 2",
		"collide CPU Performance Core 2", "collide CPU Performance Core 3",
		"collide CPU Performance Core 4", "collide CPU Performance Core 5",
		"collide CPU Performance Core 6", "collide CPU Performance Core 7",
		"collide CPU Performance Core 8",
	},
	// Core 6 (Tp0m/n/o) third < probe B at idle; cluster 3 mixes triplets.
	"m3-pro":   {m3ProP, m3ProE, m3Core9, "collide CPU Performance Core 6", "collide CPU Performance Cluster 3", m3ProAliases, m3ProVirtual},
	"m3-pro-2": {m3ProP, m3ProE, m3Core9, m3ProAliases, m3ProVirtual},
	"m3-pro-3": {m3ProP, m3ProE, m3Core9, "collide CPU Performance Core 6", m3ProAliases, m3ProVirtual},
	// Virtual-memory and misc keys with no known meaning.
	"m4":   {"unmapped TVMS TVMh TVMm"},
	"m4-2": {"unmapped TVMS TVMh"},
	"m4-4": {"unmapped T5SP TH0T TVMh TVMm"},
	"m4-5": {"unmapped T5SP TH0T TVMh TVMm"},
	// Tg2I/Tg3K read exactly 4.00 °C below Tg2H/Tg3J in the only M4 Max dump
	// (Tg0K/L too); cannot tell which of each pair is the max.
	"m4-max":   {m4ProMaxE, "collide GPU 28", "collide GPU 34", "unmapped TB0p TBBp TI0p TPMp TPSp TR5p TSVL"},
	"m4-pro":   {m4ProMaxE, "unmapped TVMx TVXX TVXh TVXm TVxx"},
	"m4-pro-2": {m4ProMaxE, "unmapped TVMx TVXX TVXh TVXm TVxx"},
	"m4-pro-3": {m4ProMaxE, "unmapped TVMx TVXX TVXh TVxx"},
	"m5-max":   {m5Aliases},
	"m5-max-2": {m5Aliases},
	"m5-pro":   {m5Aliases},
	"m5-pro-2": {m5Aliases},
}
