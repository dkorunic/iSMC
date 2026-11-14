// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/dkorunic/iSMC/smc"
)

// Aggregate detection needs no thermal load: the SMC computes aggregates by
// copying a source key's value, so an alias equals its source and a maximum
// equals whichever member is hottest. Exact equality rules out noise; the
// minimums below rule out idle coincidences between quantized readings (M6
// keys sit on a 1/64 °C grid). Only raw load snapshots count: baselines and
// peaks are averages, and the average of a maximum is not the maximum of the
// averages. A sweep reads keys one at a time, so an aggregate is sometimes
// read a tick before or after its source: on the M4 Air TCMz = max(Tp3P Tp3T
// Tp3X) holds in 47 of 51 snapshots, hence aggMinMatch.
const (
	// Share of shared snapshots in which the relation must hold.
	aggMinMatch = 0.9
	// A pair is judged only over snapshots where both keys read a value.
	aggMinShared = 5
	// The aggregate key must take this many distinct values over them.
	aggMinDistinct = 3
	// A maximum's member must have been the hottest in this many snapshots.
	aggMinHits = 2
	// Exactly 40.00 °C is the clock-gated placeholder (idle M3/M4 P-cores).
	aggGatedTemp = float32(40)
	// Lines listed per section.
	aggMaxLines = 30
)

// aggregate is one detected relation: an alias group (Members equal to Key in
// every snapshot) or a maximum (Key equal to the hottest Member in every one).
type aggregate struct {
	Key     string
	Members []string
}

// captureSnapshots returns the raw load snapshots of the CPU phases and the
// GPU phase, gated placeholders removed.
func captureSnapshots(c *guessCapture) []map[string]float32 {
	phases := slices.Clone(c.Phases)
	if c.GPU != nil {
		phases = append(phases, *c.GPU)
	}

	var snaps []map[string]float32

	add := func(m map[string]float32) {
		if len(m) == 0 {
			return
		}

		clean := make(map[string]float32, len(m))

		for k, v := range m {
			if v != aggGatedTemp {
				clean[k] = v
			}
		}

		snaps = append(snaps, clean)
	}

	for _, p := range phases {
		for _, s := range p.Series {
			add(s.Temps)
		}
	}

	return snaps
}

// findAggregates returns alias groups and maxima found in snaps.
func findAggregates(snaps []map[string]float32) ([]aggregate, []aggregate) {
	keySet := map[string]struct{}{}

	for _, s := range snaps {
		for k := range s {
			keySet[k] = struct{}{}
		}
	}

	keys := slices.Sorted(maps.Keys(keySet))
	alias := map[string]string{} // key → alias-group root (first key in sort order)

	for i, a := range keys {
		if _, done := alias[a]; done {
			continue
		}

		for _, b := range keys[i+1:] {
			if _, done := alias[b]; !done && alwaysEqual(snaps, a, b) {
				alias[a], alias[b] = a, a
			}
		}
	}

	groups := map[string][]string{}
	for k, root := range alias {
		groups[root] = append(groups[root], k)
	}

	aliases := make([]aggregate, 0, len(groups))

	for root, g := range groups {
		slices.Sort(g)
		aliases = append(aliases, aggregate{Key: root, Members: g[1:]})
	}

	slices.SortFunc(aliases, func(x, y aggregate) int { return cmp.Compare(x.Key, y.Key) })

	var maxima []aggregate

	for _, k := range keys {
		if root, ok := alias[k]; ok && root != k {
			continue // report a maximum once, under its alias root
		}

		if m := maxOf(snaps, k, alias); len(m) >= 2 {
			maxima = append(maxima, aggregate{Key: k, Members: m})
		}
	}

	return aliases, maxima
}

// alwaysEqual reports whether a and b read the same value in at least
// aggMinMatch of the snapshots holding both, over at least aggMinShared
// snapshots and aggMinDistinct values.
func alwaysEqual(snaps []map[string]float32, a, b string) bool {
	shared, equal := 0, 0
	distinct := map[float32]struct{}{}

	for _, s := range snaps {
		va, okA := s[a]
		vb, okB := s[b]

		if !okA || !okB {
			continue
		}

		shared++

		if va == vb {
			equal++

			distinct[va] = struct{}{}
		}
	}

	return shared >= aggMinShared && len(distinct) >= aggMinDistinct &&
		float64(equal) >= aggMinMatch*float64(shared)
}

// maxOf returns the keys k equals the maximum of: the keys it matched (outside
// its own alias group) at least aggMinHits times, provided k equals their
// maximum in aggMinMatch of the snapshots and takes aggMinDistinct values. It returns nil
// when no such set of two or more keys exists.
func maxOf(snaps []map[string]float32, k string, alias map[string]string) []string {
	hits := map[string]int{}
	distinct := map[float32]struct{}{}

	for _, s := range snaps {
		v, ok := s[k]
		if !ok {
			continue
		}

		distinct[v] = struct{}{}

		for o, ov := range s {
			if o != k && ov == v && (alias[o] == "" || alias[o] != alias[k]) {
				hits[o]++
			}
		}
	}

	if len(distinct) < aggMinDistinct {
		return nil
	}

	var members []string

	for o, n := range hits {
		if n >= aggMinHits && (alias[o] == "" || alias[o] == o) {
			members = append(members, o)
		}
	}

	if len(members) < 2 {
		return nil
	}

	// k must equal the hottest present member in aggMinMatch of the snapshots.
	seen, match := 0, 0

	for _, s := range snaps {
		v, ok := s[k]
		if !ok {
			continue
		}

		hottest, found := float32(0), false

		for _, o := range members {
			if ov, ok := s[o]; ok && (!found || ov > hottest) {
				hottest, found = ov, true
			}
		}

		if found {
			seen++

			if hottest == v {
				match++
			}
		}
	}

	if seen < aggMinShared || float64(match) < aggMinMatch*float64(seen) {
		return nil
	}

	slices.Sort(members)

	return members
}

// printAggregates lists the aliases and maxima a run reveals, with the
// src/temp.txt label of each aggregate key.
func printAggregates(c *guessCapture) {
	aliases, maxima := findAggregates(captureSnapshots(c))
	if len(aliases) == 0 && len(maxima) == 0 {
		return
	}

	label := func(k string) string {
		if d, ok := smc.LookupTempDesc(k, c.Family, c.Product.CPU); ok {
			return fmt.Sprintf("  %q", d)
		}

		return "  (unmapped)"
	}

	section := func(title string, rows []aggregate, line func(aggregate) string) {
		if len(rows) == 0 {
			return
		}

		fmt.Println()
		fmt.Printf("// %s\n", strings.Repeat("─", 72))
		fmt.Printf("// %s (%d):\n", title, len(rows))

		for _, a := range rows[:min(len(rows), aggMaxLines)] {
			fmt.Printf("//   %s%s\n", line(a), label(a.Key))
		}
	}

	section("Aliases: equal in >=90% of this run's load snapshots", aliases, func(a aggregate) string {
		return a.Key + " = " + strings.Join(a.Members, " = ")
	})
	section("Maxima: equal to the hottest listed key in >=90% of snapshots", maxima, func(a aggregate) string {
		return a.Key + " = max(" + strings.Join(a.Members, " ") + ")"
	})
}
