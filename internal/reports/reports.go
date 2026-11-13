// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

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
// Filenames encode the platform: report-<family>(-<variant>)?(-<seq>)?.txt,
// e.g. report-m1-ultra.txt → family "M1", SKU "M1 Ultra"; report-m4-pro-2.txt
// → "M4", "M4 Pro" (see PlatformFromFilename).
package reports

import (
	"embed"
	"slices"
	"strings"
	"sync"
)

//go:embed *.txt
var rawDumps embed.FS

var (
	parseOnce sync.Once
	byFamily  map[string]map[string]struct{}
	bySKU     map[string]map[string]struct{}
)

// Keys returns the union of T-prefixed SMC keys observed across every embedded
// report whose filename maps to the given family ("M1", "M2", "M3", "M4", "M5",
// "A18", "Intel"). Returns nil when no report exists for the family. The result
// is cached on first call; callers must not mutate the returned map.
func Keys(family string) map[string]struct{} {
	parseOnce.Do(load)

	return byFamily[family]
}

// SKUKeys returns the union of T-prefixed SMC keys observed across every
// embedded report of the given SKU (Product.CPU, e.g. "M4 Pro"), or nil when
// no report exists for it. Callers must not mutate the returned map.
func SKUKeys(sku string) map[string]struct{} {
	parseOnce.Do(load)

	return bySKU[sku]
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
	bySKU = make(map[string]map[string]struct{})

	entries, err := rawDumps.ReadDir(".")
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}

		family, sku := PlatformFromFilename(e.Name())
		if family == "" {
			continue
		}

		data, err := rawDumps.ReadFile(e.Name())
		if err != nil {
			continue
		}

		ingest(byFamily, string(data), family)
		ingest(bySKU, string(data), sku)
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

	return strings.ToUpper(first[:1]) + first[1:]
}

// PlatformFromFilename derives the family and SKU (Product.CPU) of a report
// from its filename. The variant token, when it is pro/max/ultra, completes the
// SKU; anything else (a sequence number, "t2") leaves the base SKU, which is
// spelled like the family. A18 has a single SKU, A18 Pro.
//
//	report-m4-pro-2.txt → "M4", "M4 Pro"
//	report-m4-3.txt     → "M4", "M4"
//	report-a18.txt      → "A18", "A18 Pro"
//	report-intel-t2.txt → "Intel", "Intel"
//
// Returns "", "" when the filename does not follow the convention.
func PlatformFromFilename(name string) (string, string) {
	family := familyFromFilename(name)
	if family == "" {
		return "", ""
	}

	if family == "A18" {
		return family, "A18 Pro"
	}

	tok := strings.Split(strings.TrimSuffix(strings.TrimPrefix(name, "report-"), ".txt"), "-")
	if len(tok) > 1 && slices.Contains([]string{"pro", "max", "ultra"}, tok[1]) {
		return family, family + " " + strings.ToUpper(tok[1][:1]) + tok[1][1:]
	}

	return family, family
}

// ingest scans body for lines that start with a 4-character T-prefixed SMC
// key and records each unique key under the given family in target.
//
// The expected line shape is:
//
//	Tabc  [type]  [decoded] (bytes b0 b1 b2 b3)
//
// Anything that does not start with whitespace + Tabcd (where each abcd is
// printable) is ignored, including comment lines and counter/header rows that
// share the file but begin with non-T keys (e.g. "AC-B", "BNum").
func ingest(target map[string]map[string]struct{}, body, family string) {
	set, ok := target[family]
	if !ok {
		set = make(map[string]struct{})
		target[family] = set
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
