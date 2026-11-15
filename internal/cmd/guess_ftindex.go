// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// The SMC keeps its own index of the temperature sensors it controls by:
// ftP0, ftP1, … (CPU), ftE* (efficiency or die), ftG* (GPU), ftS* (SoC), each
// mirroring one T* key, in key order. Base M3 (report-m3.txt, idle):
// ftP0-ftPf are probe B of the sixteen P triplets Tp00-Tp10 and ftPg/ftPh
// those of the Tp12/Tp16 probes, byte-identical; ftE0/ftE1 are Te05/Te0L and
// Te0O/Te0P are absent. M1 Ultra: ftP0-ftPj are probe B of the 20 core
// triplets (16P + 4E), lagging the source by up to ~5 °C while load changes.
// TG Pro reads the same index. It does not say which core a sensor belongs
// to, only which sensors the firmware treats as part of each block.
const (
	ftPrefix = "ft"
	// ftTiers are the index letters guess reads; the others (ft0*, ftA/B/D/N/R,
	// fts*) are ambient, battery, constants or copies of virtual sensors.
	ftTiers = "PEGS"
	// An index key resolves to the T* key with the smallest mean absolute
	// difference over the load snapshots, if that mean is at most ftMatchTol.
	// M1 Ultra: correct sources at 0.1-0.8 °C, the next candidate ≥ 0.2 °C
	// further. ponytail: nearest mean only; correlate the changes if the
	// filtered index ever lags more than this.
	ftMatchTol = 1.0
)

// isFtIndexKey reports whether key is an ft index entry guess resolves.
func isFtIndexKey(key string) bool {
	return len(key) == 4 && strings.HasPrefix(key, ftPrefix) && strings.ContainsRune(ftTiers, rune(key[2]))
}

// ftIndex maps an index key (ftP0) to the T* key it mirrors (Tp01).
type ftIndex map[string]string

// sources returns the set of indexed T* keys.
func (x ftIndex) sources() map[string]bool {
	out := make(map[string]bool, len(x))

	for _, src := range x {
		out[src] = true
	}

	return out
}

// hasTier reports whether the index lists any key of tier (P, E, G or S).
func (x ftIndex) hasTier(tier byte) bool {
	for k := range x {
		if k[2] == tier {
			return true
		}
	}

	return false
}

// resolveFtIndex ties each ft index key in c's load snapshots to the T* key
// it tracks. Runs saved before guess read the index have none: it returns nil.
func resolveFtIndex(c *guessCapture) ftIndex {
	snaps := captureSnapshots(c)
	if len(snaps) < aggMinShared {
		return nil
	}

	var idx, cand []string

	for k := range snaps[0] {
		switch {
		case isFtIndexKey(k):
			idx = append(idx, k)
		// Per-sensor families only (Tp, Te, Tg, Th, …): virtual and aggregate
		// keys (TVD0, TCMb) can track a source closer than the source's
		// neighbours (M1 Ultra: ftE1 resolved to TVD0 instead of Te21).
		case len(k) == 4 && k[0] == 'T' && k[1] >= 'a' && k[1] <= 'z':
			cand = append(cand, k)
		}
	}

	slices.Sort(cand) // deterministic tie-break: lowest key wins

	out := ftIndex{}

	for _, f := range idx {
		best, bestMean := "", math.Inf(1)

		for _, k := range cand {
			var (
				sum float64
				n   int
			)

			for _, s := range snaps {
				fv, ok1 := s[f]
				kv, ok2 := s[k]

				if ok1 && ok2 {
					sum += math.Abs(float64(fv - kv))
					n++
				}
			}

			if n >= aggMinShared && sum/float64(n) < bestMean {
				best, bestMean = k, sum/float64(n)
			}
		}

		if bestMean <= ftMatchTol {
			out[f] = best
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// stripFtKeys removes the index keys from every map in c: they copy T* keys
// and would otherwise show up as aliases, responders and novel sensors.
func stripFtKeys(c *guessCapture) {
	strip := func(m map[string]float32) {
		for k := range m {
			if strings.HasPrefix(k, ftPrefix) {
				delete(m, k)
			}
		}
	}

	phases := make([]*capturedPhase, 0, len(c.Phases)+1)
	for i := range c.Phases {
		phases = append(phases, &c.Phases[i])
	}

	if c.GPU != nil {
		phases = append(phases, c.GPU)
	}

	for _, p := range phases {
		strip(p.Baseline)
		strip(p.Hot)

		for _, s := range p.Series {
			strip(s.Temps)
		}
	}
}

// unindexedGroups returns the core groups none of whose keys the SMC index
// lists, of a series (Tp, Te) the index covers: maxima and probes the firmware
// does not control by, not cores. Base M3: the bank maxima Tp1H/Tp1I and
// Tp1K/Tp1L and the Te0O/Te0P pair. Without an index nothing is returned.
func unindexedGroups(groups [][]string, idx ftIndex) [][]string {
	if len(idx) == 0 {
		return nil
	}

	src := idx.sources()

	covered := map[string]bool{}
	for k := range src {
		covered[k[:2]] = true
	}

	var out [][]string

	for _, g := range groups {
		if covered[g[0][:min(2, len(g[0]))]] && !slices.ContainsFunc(g, func(k string) bool { return src[k] }) {
			out = append(out, g)
		}
	}

	return out
}

// printFtIndex lists the resolved index by tier.
func printFtIndex(idx ftIndex) {
	if len(idx) == 0 {
		return
	}

	fmt.Println()
	fmt.Printf("// %s\n", strings.Repeat("─", 72))
	fmt.Println("// SMC sensor index (ft*: the keys the firmware controls by; a core group with " +
		"no indexed key is listed as a cluster sensor):")

	for _, tier := range ftTiers {
		var keys []string

		for k := range idx {
			if rune(k[2]) == tier {
				keys = append(keys, k)
			}
		}

		if len(keys) == 0 {
			continue
		}

		slices.SortFunc(keys, func(a, b string) int { return charValue(a[3]) - charValue(b[3]) })

		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+idx[k])
		}

		fmt.Printf("//   ft%c: %s\n", tier, strings.Join(parts, " "))
	}
}
