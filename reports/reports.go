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

// Package reports embeds raw SMC sensor dumps captured from real Macs and
// exposes the union of T-prefixed keys observed for each chip family. The
// guess subcommand cross-checks its detected sensors against this observed
// set to flag (a) keys that exist in prior dumps but did not heat up in this
// run, and (b) keys that have never been seen on this family.
//
// The dumps are captured as plain-text reports under reports/*.txt and follow
// the format produced by the project's `dump` command:
//
//	KEY   [type]  [decoded] (bytes b0 b1 b2 b3)
//
// Filenames encode the family: report-<family>(-<variant>)?(-<seq>)?.txt,
// e.g. report-m1-ultra.txt → "M1", report-m4-pro-2.txt → "M4".
package reports

import (
	"embed"
	"strings"
	"sync"
)

//go:embed *.txt
var rawDumps embed.FS

var (
	parseOnce sync.Once
	byFamily  map[string]map[string]struct{}
)

// Keys returns the union of T-prefixed SMC keys observed across every embedded
// report whose filename maps to the given family ("M1", "M2", "M3", "M4", "M5",
// "A18", "Intel"). Returns nil when no report exists for the family. The result
// is cached on first call; callers must not mutate the returned map.
func Keys(family string) map[string]struct{} {
	parseOnce.Do(load)

	return byFamily[family]
}

// Families returns every family tag that has at least one embedded report.
// Order is not guaranteed.
func Families() []string {
	parseOnce.Do(load)

	out := make([]string, 0, len(byFamily))
	for f := range byFamily {
		out = append(out, f)
	}

	return out
}

// load walks every embedded *.txt file, derives its family from the filename,
// and parses out T-prefixed SMC keys. Errors are silently swallowed: a corrupt
// or unparsable report should not break the guess command — at worst the
// cross-check reports zero observed keys for that family.
func load() {
	byFamily = make(map[string]map[string]struct{})

	entries, err := rawDumps.ReadDir(".")
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}

		family := familyFromFilename(e.Name())
		if family == "" {
			continue
		}

		data, err := rawDumps.ReadFile(e.Name())
		if err != nil {
			continue
		}

		ingest(string(data), family)
	}
}

// familyFromFilename derives a family tag from a report filename. The convention
// is report-<family>(-<variant>)?(-<seq>)?.txt; the family is the first dash-
// separated token after stripping the report- prefix.
//
//	report-m4-pro.txt    → "M4"
//	report-m1-ultra.txt  → "M1"
//	report-a18.txt       → "A18"
//	report-intel-t2.txt  → "Intel"
//
// Returns "" when the filename does not follow the convention.
func familyFromFilename(name string) string {
	stem := strings.TrimSuffix(name, ".txt")

	stem, ok := strings.CutPrefix(stem, "report-")
	if !ok || stem == "" {
		return ""
	}

	first, _, _ := strings.Cut(stem, "-")
	if first == "" {
		return ""
	}

	if first == "intel" {
		return "Intel"
	}

	// "m1" → "M1", "a18" → "A18".
	return strings.ToUpper(first[:1]) + first[1:]
}

// ingest scans body for lines that start with a 4-character T-prefixed SMC
// key and records each unique key under the given family.
//
// The expected line shape is:
//
//	Tabc  [type]  [decoded] (bytes b0 b1 b2 b3)
//
// Anything that does not start with whitespace + Tabcd (where each abcd is
// printable) is ignored, including comment lines and counter/header rows that
// share the file but begin with non-T keys (e.g. "AC-B", "BNum").
func ingest(body, family string) {
	set, ok := byFamily[family]
	if !ok {
		set = make(map[string]struct{})
		byFamily[family] = set
	}

	for line := range strings.SplitSeq(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		key := fields[0]
		if len(key) != 4 || key[0] != 'T' {
			continue
		}

		set[key] = struct{}{}
	}
}
