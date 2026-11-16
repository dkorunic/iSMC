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
//	cores    distinct live "CPU {Performance,Efficiency,Super} Core N" rows (a
//	         "Core N Probe M" row counts toward core N) fall inside the
//	         skuLayouts min–max for the SKU;
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

	coreRe := regexp.MustCompile(`^CPU (Performance|Efficiency|Super) Core (\d+)(?: Probe \d+)?$`)

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
					// Mirror keepReading: a parked 40.0 °C slot does not
					// overwrite a value the row already holds.
					if _, stored := last[desc]; !stored || v != gatedTemp {
						last[desc] = key
					}

					hottest[desc] = max(hottest[desc], v)
					ungated[desc] = ungated[desc] || v != gatedTemp
				}
			})

			cores := map[string]int{}
			seen := map[string]bool{}

			for desc, key := range last {
				if hottest[desc]-live[key] > collideTol {
					got["collide "+desc] = true
				}

				if m := coreRe.FindStringSubmatch(desc); m != nil && ungated[desc] && !seen[m[1]+m[2]] {
					seen[m[1]+m[2]] = true
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

// gatedTemp is the exact value clock-gated slots are pinned at.
const gatedTemp = GatedTempCelsius

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
// other evidence supports. The M2 and M4 bank-B rows and the M4 Tpx8-D cluster
// dies are not here: report-m2-pro-2 and report-m4-pro-5 (Mac16,7) caught the
// alternate bank live.
var deadRowAllow = slices.Concat(
	// M2 Max P-cores 6-8: the only M2 Max dump reads sentinels on every Tp key;
	// the rows follow report-m2-pro.txt, whose keys are a subset of the Max dump.
	// SKU tags contain a space, so these are separate elements.
	[]string{
		"Tp0a:M2 Max", "Tp0b:M2 Max", "Tp0c:M2 Max", "Tp0e:M2 Max", "Tp0f:M2 Max",
		"Tp0g:M2 Max", "Tp0i:M2 Max", "Tp0j:M2 Max", "Tp0k:M2 Max",
	},
	// M4 Max P-cores 8-12 (cores 11-12 are the M4 Pro Fabric 1-2 triplets, both
	// banks): the only M4 Max dump has every P-core parked at gatedTemp;
	// m4-max-run (Mac16,9) reads them all live under P load. SKU tags contain a
	// space.
	[]string{
		"Tp0S:M4 Max", "Tp0T:M4 Max", "Tp0U:M4 Max", "Tp0W:M4 Max", "Tp0X:M4 Max", "Tp0Y:M4 Max",
		"Tp0a:M4 Max", "Tp0b:M4 Max", "Tp0c:M4 Max", "Tp0d:M4 Max", "Tp0e:M4 Max", "Tp0f:M4 Max",
		"Tp0g:M4 Max", "Tp0i:M4 Max", "Tp0k:M4 Max",
		"Tp2I:M4 Max", "Tp2J:M4 Max", "Tp2K:M4 Max", "Tp2L:M4 Max", "Tp2M:M4 Max", "Tp2N:M4 Max",
	})

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

			// A slot parked at gatedTemp is not evidence that the row is live:
			// M4 Pro/Max park one of two alternating P-core banks there.
			forEachKey([]SensorStat{s}, func(key, _ string) {
				if v, ok := d.live[key]; ok && v != gatedTemp {
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
	// Te0V aliases Te0T on M3 Pro.
	m3ProAliases = "unmapped Te0V"
	// Virtual TV* channels with no known meaning.
	m3MaxVirtual = "unmapped TV0h TV0m TV0s TV1h TV1m TV1s TVFh TVFm TVH3 TVHC TVSC TVsa TVss"
	m3ProVirtual = "unmapped TVHD TVHE TVHF TVHG TVHH TVHO"
	// M3 Pro: twelve indexed P triplets for 5-6 P-cores and three indexed E
	// triplets for 6 E-cores carry generic Group labels (m3-pro-run, ft index).
	m3ProP = "cores P=0"
	m3ProE = noECores
	// M3 Pro/Max cluster probes Tp0u-w/Tp0y-10: third < probe B in every M3
	// Pro and Max dump, loaded or idle.
	m3ProProbes = "collide CPU Cluster Probe 1"
	m3ProProbe2 = "collide CPU Cluster Probe 2"
	// M3 Max carries the M3 Pro Group labels (same die layout, see src/temp.txt).
	m3MaxP = "cores P=0"
	// Only Te04/Te0R triplets are per-E-core keys on M4 Pro/Max; Tpx8-D (live
	// only in report-m4-pro-5) are P cluster dies.
	m4ProMaxE = "cores E=2"
	// TVM*/TVX* virtual selectors on M4 Pro.
	m4ProVirtual = "unmapped TVMx TVXX TVXh TVXm TVxx"
	// Register aliases removed from src/temp.txt: TVDP/TVDc ≡ TVD0, Tm0y/Tm2T ≡
	// Tp0a, Ts0G ≡ the Ta00-Ta0R band (see Test_M5Pro18CoreMapping).
	m5Aliases = "unmapped TVDP TVDc Tm0y Tm2T Ts0G"
	// M5 Pro/Max P-cores read through two 19-probe Tm* clusters with no per-core
	// attribution (m5-max-run), so no "CPU Performance Core N" row exists.
	m5ProMaxP = "cores P=0"
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
	// iSMC#33 bundle, loaded: the P-core triplets (bank A) hold the sentinels of
	// report-m2-max and the eight Probe Group triplets (bank B) read 56-66 °C;
	// ftP indexes both banks alike (P9-Pg = probe B of Groups 1-8), so the
	// generic labels stand. TVM2/TVMO are virtual selectors.
	"m2-pro-2": {noECores, "cores P=0", "unmapped TVM2 TVMO"},
	// Base M3 (MacBook Air 15, Mac15,13): sixteen P triplets and three E groups
	// for 4P+4E, none attributable to a core (m3-air-run), so generic Group
	// labels. TH0T ≡ TH0x in the run; T5SP unknown, as on the M4 Air.
	"m3":     {noECores, "cores P=0", "unmapped T5SP TH0T"},
	"m3-max": {m3MaxP, m3ProProbes, m3ProProbe2, m3MaxVirtual, m3MaxGated},
	// Idle dump: third-of-triplet < probe B on groups 2-12, so last-write
	// shows the cooler value; not a distinct sensor.
	"m3-max-2": {
		m3MaxP, m3ProProbes, m3ProProbe2, m3MaxVirtual, m3MaxGated,
		"collide CPU Performance Group 2", "collide CPU Performance Group 3", "collide CPU Performance Group 4", "collide CPU Performance Group 5", "collide CPU Performance Group 6", "collide CPU Performance Group 7", "collide CPU Performance Group 8", "collide CPU Performance Group 9", "collide CPU Performance Group 10", "collide CPU Performance Group 11", "collide CPU Performance Group 12",
	},
	// Idle dumps: the third key of a triplet reads below probe B on the listed
	// groups (last-write shows the cooler value); not a distinct sensor.
	"m3-pro":   {m3ProP, m3ProE, m3ProProbes, m3ProProbe2, "collide CPU Performance Group 8", m3ProAliases, m3ProVirtual},
	"m3-pro-2": {m3ProP, m3ProE, m3ProProbes, m3ProProbe2, m3ProAliases, m3ProVirtual},
	"m3-pro-3": {m3ProP, m3ProE, m3ProProbes, m3ProProbe2, "collide CPU Performance Group 4", "collide CPU Performance Group 5", "collide CPU Performance Group 8", m3ProAliases, m3ProVirtual},
	// Cool idle dump (MBP 14" Nov 2023): third < probe B on groups 1-10.
	"m3-pro-4": {
		m3ProP, m3ProE, m3ProProbes, m3ProProbe2, m3ProAliases, m3ProVirtual,
		"collide CPU Performance Group 1", "collide CPU Performance Group 2", "collide CPU Performance Group 3", "collide CPU Performance Group 4", "collide CPU Performance Group 5", "collide CPU Performance Group 6", "collide CPU Performance Group 7", "collide CPU Performance Group 8", "collide CPU Performance Group 9", "collide CPU Performance Group 10",
	},
	// Idle dump with the SMC ft index (MBP 14" 6P+6E, m3-pro-run): third <
	// probe B on groups 1-10, as in m3-pro-4.
	"m3-pro-5": {
		m3ProP, m3ProE, m3ProProbes, m3ProProbe2, m3ProAliases, m3ProVirtual,
		"collide CPU Performance Group 1", "collide CPU Performance Group 2", "collide CPU Performance Group 3", "collide CPU Performance Group 4", "collide CPU Performance Group 5", "collide CPU Performance Group 6", "collide CPU Performance Group 7", "collide CPU Performance Group 8", "collide CPU Performance Group 9", "collide CPU Performance Group 10",
	},
	// Virtual-memory selectors and misc keys. TVMh/TVMm copy another TV* key
	// byte-for-byte, but which one varies by machine (Mac16,12 guess run:
	// TVMh ≡ TVh0, TVMm ≡ TVM0 over 51 samples; m4-4/m4-5: TVMh ≡ TVM0).
	// TH0T ≡ TH0x in m4-4/5/7. T5SP stays within 0.5 °C under full CPU and GPU
	// load: a board/ambient probe of unknown placement.
	"m4": {"unmapped TVMS TVMh TVMm"},
	// iSMC#33 bundle, 222-key base M4 like m4/m4-2.
	"m4-8": {"unmapped TVMS TVMh TVMm"},
	"m4-2": {"unmapped TVMS TVMh"},
	"m4-4": {"unmapped T5SP TH0T TVMh TVMm"},
	"m4-5": {"unmapped T5SP TH0T TVMh TVMm"},
	"m4-7": {"unmapped T5SP TH0T TVMh TVMm"},
	// Tg2I/Tg3K read exactly 4.00 °C below Tg2H/Tg3J in the only M4 Max dump
	// (Tg0K/L too); cannot tell which of each pair is the max. Every P-core
	// triplet, TCMb and TCMz are pinned at gatedTemp: the P-core mapping is
	// unverified until a loaded M4 Max dump exists.
	"m4-max": {m4ProMaxE, "cores P=0", "collide GPU 28", "collide GPU 34", "unmapped TB0p TBBp TI0p TPMp TPSp TR5p TSVL"},
	"m4-pro": {m4ProMaxE, m4ProVirtual},
	// Idle dumps: all ten P-core triplets are parked at gatedTemp (51 keys);
	// report-m4-pro.txt is the dump that verifies the M4 Pro P-core mapping.
	"m4-pro-2": {m4ProMaxE, "cores P=0", m4ProVirtual},
	"m4-pro-3": {m4ProMaxE, "cores P=0", "unmapped TVMx TVXX TVXh TVxx"},
	// Mac16,8 (iSMC#28), 14-core: bank A (Tp00..Tp0c) live, bank B parked.
	"m4-pro-4": {m4ProMaxE, m4ProVirtual},
	// Mac16,7 (iSMC#32/#33): bank B (Tp1i..) and Tpx8-D live, bank A mostly
	// sentinels, but the bank-A P-core 9 triplet and Tp0c also read 34-42 °C,
	// so last-write shows the cooler bank-A value (keepReading only skips 40.0).
	"m4-pro-5": {m4ProMaxE, m4ProVirtual, "collide CPU Performance Core 9", "collide CPU Performance Core 10"},
	// Only four Te keys exist; E-cores 5-6 are among the generic "CPU Probe" Tp
	// slots, which the only (idle) base M5 dump cannot place. TVC0 ≡ TVD0
	// byte-for-byte; TVMS is unknown, as on M4.
	"m5": {"cores E=4", "unmapped TVC0 TVMS"},
	// MacBook Air 13 M5 (Mac17,3), idle, with m5-air-run: four Te triplets for
	// six E-cores as on m5; TVDE ≡ TVD0 and TH0T ≡ TH0x byte for byte, TVDP
	// tracks TVD0 (an alias on M5 Pro/Max, m5Aliases); T5SP unknown, as on M4.
	"m5-2":     {"cores E=4", "unmapped T5SP TH0T TVDE TVDP"},
	"m5-max":   {m5Aliases, m5ProMaxP},
	"m5-max-2": {m5Aliases, m5ProMaxP},
	"m5-pro":   {m5Aliases, m5ProMaxP},
	"m5-pro-2": {m5Aliases, m5ProMaxP},
	"m5-pro-3": {m5Aliases, m5ProMaxP},
	// M6 E-cores have no per-core sensors (see src/temp.txt). TVDC ≡ TVD0
	// byte-for-byte; Te0g and Ts05 ≡ Te0f in every snapshot of the owner's
	// saved guess run.
	"m6": {noECores, "unmapped TVDC Te0g Ts05"},
}
