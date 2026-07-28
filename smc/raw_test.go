// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package smc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeKeyAt returns a keyAt probe over a fixed key list that counts probes and
// optionally fails on a given index.
func fakeKeyAt(keys []string, probes *int, failAt int) func(uint32) (string, bool) {
	return func(i uint32) (string, bool) {
		*probes++

		if failAt >= 0 && int(i) == failAt {
			return "", false
		}

		return keys[i], true
	}
}

// sortedNamespace mimics a firmware key index: ascending ASCII order with a
// contiguous Tg block in the middle.
var sortedNamespace = []string{
	"#KEY", "AC-B", "ACLC", "BATP", "CHBI", "F0Ac", "PC0C", "PSTR",
	"TB0T", "TCMb", "TH0p", "Tg04", "Tg05", "Tg0C", "Tg0D", "Tg0K",
	"Th0a", "Tp00", "Tp01", "Tp04", "VD0R", "Vp0C", "WvBu", "zDBG",
}

func Test_scanKeysByPrefix(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		prefix string
		want   []string
		wantOK bool
	}{
		{
			"block in middle", sortedNamespace, "Tg",
			[]string{"Tg04", "Tg05", "Tg0C", "Tg0D", "Tg0K"},
			true,
		},
		{
			"single-letter prefix spans families", sortedNamespace, "T",
			[]string{
				"TB0T", "TCMb", "TH0p", "Tg04", "Tg05", "Tg0C", "Tg0D", "Tg0K",
				"Th0a", "Tp00", "Tp01", "Tp04",
			},
			true,
		},
		{
			"block at start", sortedNamespace, "#",
			[]string{"#KEY"},
			true,
		},
		{
			"block at end", sortedNamespace, "z",
			[]string{"zDBG"},
			true,
		},
		{
			"absent prefix", sortedNamespace, "Q",
			nil, true,
		},
		{
			"prefix past all keys", sortedNamespace, "zz",
			nil, true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probes := 0
			got, ok := scanKeysByPrefix(totalOf(tt.keys),
				fakeKeyAt(tt.keys, &probes, -1), tt.prefix)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Test_scanKeysByPrefix_ProbeCount proves the search is logarithmic plus the
// block length, not a full namespace walk.
func Test_scanKeysByPrefix_ProbeCount(t *testing.T) {
	// 2016 sorted keys with the 16-key Tg block in the middle.
	keys := make([]string, 0, 2048)

	for i := range 1000 {
		keys = append(keys, "A"+padIndex(i))
	}

	tgBlock := []string{
		"Tg04", "Tg05", "Tg0C", "Tg0D", "Tg0K", "Tg0L", "Tg0S", "Tg0T",
		"Tg0a", "Tg0b", "Tg0i", "Tg0j", "Tg0q", "Tg0r", "Tg0y", "Tg0z",
	}
	keys = append(keys, tgBlock...)

	for i := range 1000 {
		keys = append(keys, "V"+padIndex(i))
	}

	probes := 0
	got, ok := scanKeysByPrefix(totalOf(keys), fakeKeyAt(keys, &probes, -1), "Tg")

	assert.True(t, ok)
	assert.Equal(t, tgBlock, got)
	// log2(2016) ≈ 11 binary-search probes + 16 block keys + 1 terminator.
	assert.LessOrEqual(t, probes, 32,
		"prefix scan probed %d indices; must not degrade to a full walk", probes)
}

func Test_scanKeysByPrefix_Fallback(t *testing.T) {
	t.Run("probe failure mid-search", func(t *testing.T) {
		probes := 0
		_, ok := scanKeysByPrefix(totalOf(sortedNamespace),
			fakeKeyAt(sortedNamespace, &probes, len(sortedNamespace)/2), "Tg")

		assert.False(t, ok, "a failed CMDReadIndex probe must request fallback")
	})

	t.Run("order violation inside block", func(t *testing.T) {
		unsorted := []string{"Tg05", "Tg04", "Tg0C"} // firmware not sorted

		probes := 0
		_, ok := scanKeysByPrefix(totalOf(unsorted),
			fakeKeyAt(unsorted, &probes, -1), "Tg")

		assert.False(t, ok, "an order violation must request fallback")
	})
}

// totalOf returns len(keys) as the uint32 index total scanKeysByPrefix expects.
func totalOf(keys []string) uint32 {
	//nolint:gosec // G115: test fixtures are far below the uint32 range
	return uint32(len(keys))
}

// padIndex renders i as a 3-digit zero-padded string ("007").
func padIndex(i int) string {
	//nolint:gosec // G115: digits are bounded to '0'..'9'
	return string([]byte{byte('0' + i/100), byte('0' + i/10%10), byte('0' + i%10)})
}

// Test_EnumerateKeysPrefix_Live cross-checks the prefix fast path against a
// filtered full enumeration on real hardware. Skips when no SMC is reachable
// (CI, sandboxed runners).
func Test_EnumerateKeysPrefix_Live(t *testing.T) {
	conn, err := Open()
	if err != nil {
		t.Skipf("no SMC connection available: %v", err)
	}

	defer Close(conn)

	want := make([]string, 0, 256)

	for _, k := range EnumerateKeys(conn) {
		if len(k) > 0 && k[0] == 'T' {
			want = append(want, k)
		}
	}

	got := EnumerateKeysPrefix(conn, "T")

	assert.Equal(t, want, got,
		"prefix fast path must return exactly the T-keys of a full enumeration, in index order")
	assert.NotEmpty(t, got, "a real Mac always exposes T-prefixed sensor keys")
}
