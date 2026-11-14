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
	"github.com/dkorunic/iSMC/internal/reports"
)

// Test_DumpReplay replays every internal/reports dump through the production
// temperature mapping for the dump's platform and checks, per dump:
//
//	cores    distinct live "CPU {Performance,Efficiency,Super} Core N" rows fall
//	         inside the skuLayouts min–max for the SKU;
//	collide  no description hides a hotter distinct key behind the last-written
//	         one (last-write-wins in addGeneric);
//	unmapped every live T* key resolves to some description;
//	dupkey   no key in scope carries two descriptions (both rows would be
//	         emitted, and LookupTempDesc would report only the last).
//
// A core whose live keys all read exactly gatedTemp is a clock-gated
// placeholder and does not count towards cores.
//
// Known violations live in replayAllow; the test fails on new violations and on
// allowlist entries that no longer fire, so the list can only shrink.
func Test_DumpReplay(t *testing.T) {
	const collideTol = 0.5 // °C; probe A/B vs max spread on a live triplet.

	coreRe := regexp.MustCompile(`^CPU (Performance|Efficiency|Super) Core \d+$`)

	for _, d := range loadDumps(t) {
		name, family, sku, live := d.name, d.family, d.sku, d.live

		t.Run(name, func(t *testing.T) {
			got := map[string]bool{}

			// unmapped
			mapped := MappedTempKeys(family, sku)

			for k := range live {
				if _, ok := mapped[k]; !ok && !notTemperature[k] {
					got["unmapped "+k] = true
				}
			}

			// Replay addGeneric order: last live key written per description.
			rows := rowsFor(AppleTemp, family, sku)

			last := map[string]string{}
			hottest := map[string]float32{}
			ungated := map[string]bool{} // description has a live key that is not gated
			descOf := map[string]string{}

			forEachKey(rows, func(key, desc string) {
				if prev, ok := descOf[key]; ok && prev != desc {
					got["dupkey "+key] = true
				}

				descOf[key] = desc

				if v, ok := live[key]; ok {
					last[desc] = key
					hottest[desc] = max(hottest[desc], v)
					ungated[desc] = ungated[desc] || v != gatedTemp
				}
			})

			cores := map[string]int{}

			for desc, key := range last {
				if hottest[desc]-live[key] > collideTol {
					got["collide "+desc] = true
				}

				if m := coreRe.FindStringSubmatch(desc); m != nil && ungated[desc] {
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

// gatedTemp is the exact value clock-gated slots are pinned at: inside the
// isValidReading window, meaningless as data.
const gatedTemp = 40.0

// dump is one internal/reports file: its platform, taken from the filename
// (the only label a dump has), and its live temperature keys.
type dump struct {
	name, family, sku string
	live              map[string]float32
}

// loadDumps reads every report-<family>[-<pro|max|ultra>][-<seq>].txt dump.
func loadDumps(t *testing.T) []dump {
	t.Helper()

	files, err := filepath.Glob("../internal/reports/report-*.txt")
	if err != nil || len(files) == 0 {
		t.Fatalf("no dumps found: %v", err)
	}

	dumps := make([]dump, 0, len(files))

	for _, f := range files {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "report-"), ".txt")
		family, sku := reports.PlatformFromFilename(filepath.Base(f))

		dumps = append(dumps, dump{name: name, family: family, sku: sku, live: liveTemps(t, f)})
	}

	return dumps
}

// deadRowAllow lists rows ("KEY:Platform") that no dump reads live but that
// other evidence supports. The M4 rows are the alternate P-core and cluster-die
// scheme of the M4 Pro 14-core (m4Pro14CoreReport in mapping_test.go); every M4
// dump here uses the Tp00-Tp0c scheme and reads these as 0 or -4.
var deadRowAllow = slices.Concat(strings.Fields("Tpx8:M4 Tpx9:M4 TpxA:M4 TpxB:M4 TpxC:M4 TpxD:M4 "+
	"Tp1i:M4 Tp1j:M4 Tp1k:M4 Tp1m:M4 Tp1n:M4 Tp1o:M4 Tp1q:M4 Tp1t:M4 Tp1u:M4 Tp1v:M4 "+
	"Tp1w:M4 Tp1x:M4 Tp1y:M4 Tp1z:M4 Tp20:M4 Tp21:M4 Tp22:M4 Tp23:M4 Tp24:M4 Tp25:M4 "+
	"Tp26:M4 Tp27:M4 Tp28:M4 Tp29:M4 Tp2A:M4 Tp2B:M4 Tp2C:M4 Tp2D:M4 Tp2E:M4 Tp2G:M4"),
	// M2 Max P-cores 6-8: the only M2 Max dump reads sentinels on every Tp key;
	// the rows follow report-m2-pro.txt, whose keys are a subset of the Max dump.
	// SKU tags contain a space, so these are separate elements.
	[]string{
		"Tp0a:M2 Max", "Tp0b:M2 Max", "Tp0c:M2 Max", "Tp0e:M2 Max", "Tp0f:M2 Max",
		"Tp0g:M2 Max", "Tp0i:M2 Max", "Tp0j:M2 Max", "Tp0k:M2 Max",
	},
	// M2 Pro/Max second P-level triplets and cluster probe 2: gated in the idle
	// report-m2-pro/m2-max, live at 96-109 °C (Tp2G-I 83-91 °C) in an M2 Max
	// Cinebench dump in exelban/stats#1286.
	strings.Fields("Tp1A:M2 Tp1B:M2 Tp1C:M2 Tp1E:M2 Tp1F:M2 Tp1G:M2 Tp1I:M2 Tp1J:M2 Tp1K:M2 "+
		"Tp1M:M2 Tp1N:M2 Tp1O:M2 Tp1g:M2 Tp1h:M2 Tp1i:M2 Tp1k:M2 Tp1l:M2 Tp1m:M2 "+
		"Tp1o:M2 Tp1p:M2 Tp1q:M2 Tp1s:M2 Tp1t:M2 Tp1u:M2 Tp2G:M2 Tp2H:M2 Tp2I:M2"),
	// M4 Tp1i-scheme fabric triplets: live only in the iSMC#28 JSON run
	// (Mac16,8), which has no raw dump yet.
	strings.Fields("Tp2I:M4 Tp2J:M4 Tp2K:M4 Tp2L:M4 Tp2M:M4 Tp2N:M4 Tp2O:M4 Tp2Q:M4 Tp2R:M4 "+
		"Tp2S:M4 Tp2T:M4 Tp2U:M4 Tp2V:M4 Tp2W:M4 Tp2X:M4"))

// Test_DumpLiveRows checks that every Apple-Silicon-tagged row reads live in at
// least one dump its tag matches, so rows nothing supports cannot accumulate.
// "All" and "Intel" rows are exempt: one T2 dump cannot speak for older Intel
// hardware. Exceptions live in deadRowAllow, which fails on stale entries.
func Test_DumpLiveRows(t *testing.T) {
	dumps := loadDumps(t)
	allow := map[string]bool{}

	for _, e := range deadRowAllow {
		allow[e] = true
	}

	for _, s := range AppleTemp {
		if s.Platform == platformAll || s.Platform == familyIntel {
			continue
		}

		matched, liveAny := false, false

		for _, d := range dumps {
			if !platformMatches(s.Platform, d.family, d.sku) {
				continue
			}

			matched = true

			forEachKey([]SensorStat{s}, func(key, _ string) {
				if _, ok := d.live[key]; ok {
					liveAny = true
				}
			})
		}

		id := s.Key + ":" + s.Platform
		dead := matched && !liveAny

		switch {
		case dead && !allow[id]:
			t.Errorf("row %q (%s) is not live in any %s dump", s.Desc, id, s.Platform)
		case !dead && allow[id]:
			t.Errorf("stale deadRowAllow entry, remove it: %s", id)
		}

		delete(allow, id)
	}

	for id := range allow {
		t.Errorf("deadRowAllow entry matches no row: %s", id)
	}
}

// Test_SensorRowLint catches typos the generator passes through silently: a
// platform tag that matches no known family or SKU yields a row that never
// loads, and a malformed key one that never reads.
func Test_SensorRowLint(t *testing.T) {
	tags := map[string]bool{platformAll: true, familyApple: true, familyIntel: true}

	for _, sku := range platform.SKUs() {
		family, _, _ := strings.Cut(sku, " ")
		tags[sku], tags[family] = true, true

		if _, ok := platform.LookupSKULayout(family); ok {
			tags[family+skuBase] = true
		}
	}

	for _, s := range AppleTemp {
		if !tags[s.Platform] {
			t.Errorf("temp row %q (%s): unknown platform tag %q", s.Desc, s.Key, s.Platform)
		}
	}

	for name, rows := range map[string][]SensorStat{
		"temp": AppleTemp, "power": ApplePower, "voltage": AppleVoltage, "current": AppleCurrent,
	} {
		for _, s := range rows {
			if len(s.Key) != 4 || s.Desc == "" || strings.Count(s.Key, KeyWildcard) > 1 ||
				strings.Contains(s.Key, KeyWildcard) != strings.Contains(s.Desc, KeyWildcard) {
				t.Errorf("%s row %q: malformed key %q or wildcard mismatch", name, s.Desc, s.Key)
			}
		}
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
	// No live, ungated E-core row: M2 (no E mapping) and M6 (generic labels).
	noECores = "cores E=0"
	// Clock-gated placeholders pinned at exactly 40.00 °C in both M3 Max dumps.
	m3MaxGated = "unmapped Tp1g Tp1h Tp1i Tp1o Tp1p Tp1q Tp1x Tp1y Tp1z Tp25 Tp26 Tp27 Tp29 Tp2A " +
		"Tp2B Tp2H Tp2I Tp2J Tp2P Tp2Q Tp2R Tp2X Tp2Y Tp2Z Tp2f Tp2g Tp2h Tp2j Tp2k Tp2l " +
		"Tp2r Tp2s Tp2t Tp2z Tp30 Tp31 Tp33 Tp34 Tp35 Tp3B Tp3C Tp3D Tp3a Tp3b Tp3e Tp3f Tp3i Tp3j"
	// Tp0u/v/w track Tp0y/z/10 within ~0.7 °C (never byte-equal); Te0V aliases
	// Te0T. Both dropped per src/temp.txt.
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
	// Tp0y/z/10 ("Core 9"): third < probe B in all six M3 dumps, loaded or idle.
	m3Core9 = "collide CPU Performance Core 9"
	// Only Te04/Te0R triplets are live on M4 Pro/Max (Tpx8-D read 0 in all four
	// dumps, see deadRowAllow); no other key carries per-E-core data.
	m4ProMaxE = "cores E=2"
	// Register aliases removed from src/temp.txt: TVDP/TVDc ≡ TVD0, Tm0y/Tm2T ≡
	// Tp0a, Ts0G ≡ the Ta00-Ta0R band (see Test_M5Pro18CoreMapping).
	m5Aliases = "unmapped TVDP TVDc Tm0y Tm2T Ts0G"
)

// notTemperature lists live T* keys that are deliberately unmapped because
// they are not temperatures. TR0Z reads one of two constants (51.85/51.82 °C)
// in every dump, Intel T2 included, and matches HID "PMU tcal" on M1 Max,
// M1 Ultra and M6: a PMU calibration value.
var notTemperature = map[string]bool{"TR0Z": true}

// replayAllow lists the violations Test_DumpReplay currently tolerates, keyed
// by dump name (report-<name>.txt). Entries are "cores <T>=<n>",
// "collide <description>", "dupkey <key>" or "unmapped <key> [<key>...]".
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
	"m2-max": {noECores, "cores P=0"},
	// Base M2: four E-cores share the Te04-06 cluster aggregate; Tp0a/b/c third <
	// probe B (idle cluster aggregate); TaMP/Th1H unknown, single dump.
	"m2": {noECores, "collide CPU Performance Cluster Aggregate 1", "unmapped TaMP Th1H"},
	// Mac14,10, loaded: E-cores only as the Te04-06 cluster aggregate.
	"m2-pro": {noECores},
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
	// Cool idle dump (MBP 14" Nov 2023): third-of-triplet < probe B on cores 1-7
	// and cluster 1, as in m3-max-2; not a distinct sensor.
	"m3-pro-4": {
		m3ProP, m3ProE, m3Core9, m3ProAliases, m3ProVirtual,
		"collide CPU Performance Cluster 1",
		"collide CPU Performance Core 1", "collide CPU Performance Core 2",
		"collide CPU Performance Core 3", "collide CPU Performance Core 4",
		"collide CPU Performance Core 5", "collide CPU Performance Core 6",
		"collide CPU Performance Core 7",
	},
	// Virtual-memory selectors and misc keys. TVMh/TVMm copy another TV* key
	// byte-for-byte, but which one varies by machine (Mac16,12 guess run:
	// TVMh ≡ TVh0, TVMm ≡ TVM0 over 51 samples; m4-4/m4-5: TVMh ≡ TVM0).
	// TH0T ≡ TH0x in m4-4/5/7. T5SP stays within 0.5 °C under full CPU and GPU
	// load: a board/ambient probe of unknown placement.
	"m4":   {"unmapped TVMS TVMh TVMm"},
	"m4-2": {"unmapped TVMS TVMh"},
	"m4-4": {"unmapped T5SP TH0T TVMh TVMm"},
	"m4-5": {"unmapped T5SP TH0T TVMh TVMm"},
	"m4-7": {"unmapped T5SP TH0T TVMh TVMm"},
	// Tg2I/Tg3K read exactly 4.00 °C below Tg2H/Tg3J in the only M4 Max dump
	// (Tg0K/L too); cannot tell which of each pair is the max. Every P-core
	// triplet, TCMb and TCMz are pinned at gatedTemp: the P-core mapping is
	// unverified until a loaded M4 Max dump exists.
	"m4-max": {m4ProMaxE, "cores P=0", "collide GPU 28", "collide GPU 34", "unmapped TB0p TBBp TI0p TPMp TPSp TR5p TSVL"},
	"m4-pro": {m4ProMaxE, "unmapped TVMx TVXX TVXh TVXm TVxx"},
	// Idle dumps: all ten P-core triplets are parked at gatedTemp (51 keys);
	// report-m4-pro.txt is the dump that verifies the M4 Pro P-core mapping.
	"m4-pro-2": {m4ProMaxE, "cores P=0", "unmapped TVMx TVXX TVXh TVXm TVxx"},
	"m4-pro-3": {m4ProMaxE, "cores P=0", "unmapped TVMx TVXX TVXh TVxx"},
	// Only four Te keys exist; E-cores 5-6 are among the generic "CPU Probe" Tp
	// slots, which the only (idle) base M5 dump cannot place. TVC0 ≡ TVD0
	// byte-for-byte; TVMS is unknown, as on M4.
	"m5":       {"cores E=4", "unmapped TVC0 TVMS"},
	"m5-max":   {m5Aliases},
	"m5-max-2": {m5Aliases},
	"m5-pro":   {m5Aliases},
	"m5-pro-2": {m5Aliases},
	// M6 E-cores have no per-core sensors (see src/temp.txt). TVDC ≡ TVD0
	// byte-for-byte; Te0g and Ts05 ≡ Te0f in every snapshot of the owner's
	// saved guess run.
	"m6": {noECores, "unmapped TVDC Te0g Ts05"},
}
