// Copyright (C) 2026  Dinko Korunic
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, version 3.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
// General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

//go:build darwin

package reports

import (
	"testing"
)

func TestFamilyFromFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{"report-m1-pro.txt", "M1"},
		{"report-m1-ultra.txt", "M1"},
		{"report-m4-pro-2.txt", "M4"},
		{"report-m4.txt", "M4"},
		{"report-m5-pro.txt", "M5"},
		{"report-a18.txt", "A18"},
		{"report-intel-t2.txt", "Intel"},
		{"report-.txt", ""},           // empty family
		{"some-other-file.txt", ""},   // no report- prefix
		{"report-m4-pro.notxt", "M4"}, // odd suffix still parses; family is leading token
		{"", ""},                      // empty filename
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := familyFromFilename(tt.name)
			if got != tt.want {
				t.Errorf("familyFromFilename(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestKeysIntegration exercises the embedded reports end-to-end: every chip
// family that has a report file in reports/ should produce a non-trivial key
// set with at least one Tp-prefixed key (CPU performance / super cores —
// universal across every Apple Silicon variant).
//
// This test guards against (a) the embed directive accidentally being reduced
// in scope, (b) the filename → family mapper drifting from the actual files,
// and (c) the line parser silently dropping keys (e.g. column-format drift).
//
// We don't assert on specific Tp indices — M1 starts at Tp00, M3 at Tp04, M5
// Pro extends through Tp0X+ — only that the prefix is populated. Te (E-cores)
// is intentionally NOT asserted: M5 Pro / Max have no E-cores, so currently
// our M5 reports (which only cover M5 Pro) include no Te keys.
func TestKeysIntegration(t *testing.T) {
	t.Parallel()

	for _, family := range []string{"M1", "M3", "M4", "M5", "A18"} {
		got := Keys(family)
		if len(got) < 20 {
			t.Errorf("Keys(%q): %d keys parsed, expected ≥20; "+
				"check reports/*.txt for the family", family, len(got))

			continue
		}

		hasTp := false

		for k := range got {
			if len(k) >= 2 && k[1] == 'p' {
				hasTp = true

				break
			}
		}

		if !hasTp {
			t.Errorf("Keys(%q) has no Tp-prefixed CPU core keys", family)
		}
	}

	// Intel report ships with a different fingerprint set (TC0c, TIOP, Tarc...).
	intelKeys := Keys("Intel")
	if len(intelKeys) == 0 {
		t.Error("Keys(\"Intel\") returned no keys; check reports/report-intel-t2.txt")
	}

	// TIOP is a hard fingerprint of Intel/T2 hardware; never appears on Apple Silicon.
	if _, ok := intelKeys["TIOP"]; !ok {
		t.Error("Keys(\"Intel\") missing canonical Intel fingerprint TIOP")
	}
}

// TestFamilies asserts that every family with at least one *.txt under
// reports/ is enumerated. The actual list grows over time as the project
// gains coverage, so we just spot-check that the well-known families are
// present and there are no zero-length tags.
func TestFamilies(t *testing.T) {
	t.Parallel()

	got := Families()
	if len(got) == 0 {
		t.Fatal("Families() returned no entries; embedded reports may be missing")
	}

	wantPresent := map[string]bool{"M1": false, "M4": false, "Intel": false}

	for _, f := range got {
		if f == "" {
			t.Error("Families() returned an empty family tag")
		}

		if _, ok := wantPresent[f]; ok {
			wantPresent[f] = true
		}
	}

	for f, found := range wantPresent {
		if !found {
			t.Errorf("Families() missing expected family %q", f)
		}
	}
}

// TestIngest_skipsNonTKeys uses a synthetic body to confirm that only
// 4-character T-prefixed keys are recorded, and that comment / counter rows
// (e.g. "AC-B", "#KEY") are filtered out.
func TestIngest_skipsNonTKeys(t *testing.T) {
	t.Parallel()

	// Reset the package state for an isolated test.
	byFamily = map[string]map[string]struct{}{}

	body := `
  TB0T  [flt ]  19 (bytes 98 99 99 41)
  AC-B  [si8 ]  -1 (bytes ff)
  #KEY  [ui32]  2332 (bytes 00 00 09 1c)
  Tp00  [flt ]  35.5 (bytes 00 00 00 00)
  XYZ   not a real key
no leading whitespace
`
	ingest(body, "Test")

	got := byFamily["Test"]
	if len(got) != 2 {
		t.Errorf("ingest recorded %d keys, want 2 (TB0T + Tp00): %v", len(got), got)
	}

	for _, want := range []string{"TB0T", "Tp00"} {
		if _, ok := got[want]; !ok {
			t.Errorf("ingest missing expected key %q", want)
		}
	}

	for _, reject := range []string{"AC-B", "#KEY", "XYZ"} {
		if _, ok := got[reject]; ok {
			t.Errorf("ingest erroneously recorded non-temperature key %q", reject)
		}
	}
}
