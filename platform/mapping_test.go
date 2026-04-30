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

package platform

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test_productsIntegrity validates every entry in the products lookup table for common
// transcription errors: missing fields, implausible years, and mis-tagged families.
//
// Family consistency rules enforced:
//   - Models whose CPU string contains "Core" or "Xeon" must have Family == "Intel".
//   - Models whose CPU starts with "M" (e.g. "M1", "M4 Pro") must have Family starting with "M".
//   - Models whose CPU starts with "A" (e.g. "A18 Pro") must have Family starting with "A".
func Test_productsIntegrity(t *testing.T) {
	t.Parallel()

	for modelID, p := range products {
		t.Run(modelID, func(t *testing.T) {
			t.Parallel()

			assert.NotEmpty(t, p.Name, "model %q: Name must not be empty", modelID)
			assert.NotEmpty(t, p.Family, "model %q: Family must not be empty", modelID)
			assert.NotEmpty(t, p.CPU, "model %q: CPU must not be empty", modelID)
			assert.GreaterOrEqual(t, p.Year, 2006,
				"model %q: Year %d is before the first Intel Mac (2006)", modelID, p.Year)
			assert.LessOrEqual(t, p.Year, 2030,
				"model %q: Year %d looks implausibly far in the future", modelID, p.Year)

			// Family/CPU consistency
			cpuIsIntel := strings.Contains(p.CPU, "Core") || strings.Contains(p.CPU, "Xeon")
			if cpuIsIntel {
				assert.Equal(t, "Intel", p.Family,
					"model %q: CPU %q implies Intel family but Family=%q", modelID, p.CPU, p.Family)
			}

			if strings.HasPrefix(p.CPU, "M") {
				assert.True(t, strings.HasPrefix(p.Family, "M"),
					"model %q: CPU %q implies M-family but Family=%q", modelID, p.CPU, p.Family)
			}

			if strings.HasPrefix(p.CPU, "A") {
				assert.True(t, strings.HasPrefix(p.Family, "A"),
					"model %q: CPU %q implies A-family but Family=%q", modelID, p.CPU, p.Family)
			}
		})
	}
}

// Test_productsKnownEntries verifies a representative set of well-known model identifiers
// to catch column-swap errors (e.g. Name vs CPU) or wrong year assignments.
func Test_productsKnownEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		modelID string
		want    Product
	}{
		// First-generation Apple Silicon Mac mini
		{"Macmini9,1", Product{Name: "Mac mini (M1)", Year: 2020, Family: "M1", CPU: "M1"}},
		// M4 Mac mini (2024)
		{"Mac16,10", Product{Name: "Mac mini (M4)", Year: 2024, Family: "M4", CPU: "M4"}},
		// Last Intel Mac mini
		{"Macmini8,1", Product{Name: "Mac mini", Year: 2018, Family: "Intel", CPU: "Core i3-8100B"}},
		// First M1 MacBook Pro
		{"MacBookPro17,1", Product{Name: "MacBook Pro 13 (M1)", Year: 2020, Family: "M1", CPU: "M1"}},
		// First M1 MacBook Air
		{"MacBookAir10,1", Product{Name: "MacBook Air 13 (M1)", Year: 2020, Family: "M1", CPU: "M1"}},
		// M1 iMac
		{"iMac21,1", Product{Name: "iMac 24-Inch (M1)", Year: 2021, Family: "M1", CPU: "M1"}},
		// Intel iMac Pro
		{"iMacPro1,1", Product{Name: "iMac Pro", Year: 2017, Family: "Intel", CPU: "Xeon W-2140B"}},
		// M2 Mac Pro
		{"Mac14,8", Product{Name: "Mac Pro (M2 Ultra)", Year: 2023, Family: "M2", CPU: "M2 Ultra"}},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			t.Parallel()

			got, ok := products[tt.modelID]
			assert.True(t, ok, "model %q must be present in products map", tt.modelID)
			assert.Equal(t, tt.want, got, "model %q data mismatch", tt.modelID)
		})
	}
}

// Test_GetProduct_consistency verifies that GetProduct() and GetModelID() return
// mutually consistent results: the product returned by GetProduct() must be identical
// to the entry at products[GetModelID()].
//
// The test is skipped when hw.model returns an empty string (e.g. in a sandbox that
// blocks sysctl) and gracefully passes when the model is not in the products map.
func Test_GetProduct_consistency(t *testing.T) {
	modelID := GetModelID()
	if modelID == "" {
		t.Skip("hw.model sysctl returned empty string; skipping consistency check")
	}

	gotProduct, gotOK := GetProduct()
	wantProduct, wantOK := products[modelID]

	assert.Equal(t, wantOK, gotOK,
		"GetProduct() found=%v but products[%q] found=%v", gotOK, modelID, wantOK)

	if gotOK {
		assert.Equal(t, wantProduct, gotProduct,
			"GetProduct() must return the same data as products[%q]", modelID)
	}
}

// Test_GetFamily_consistency verifies that GetFamily() returns a value consistent with
// the products map entry for the current machine's model identifier.
func Test_GetFamily_consistency(t *testing.T) {
	modelID := GetModelID()
	if modelID == "" {
		t.Skip("hw.model sysctl returned empty string; skipping GetFamily consistency check")
	}

	family := GetFamily()

	p, inMap := products[modelID]
	if !inMap {
		// Model not yet in the table; GetFamily returns "Unknown" — just verify that.
		assert.Equal(t, "Unknown", family,
			"GetFamily must return 'Unknown' for unrecognised model %q", modelID)

		return
	}

	assert.Equal(t, p.Family, family,
		"GetFamily() must match products[%q].Family", modelID)
}

// Test_skuLayoutsIntegrity walks the SKU roster and verifies every entry honours
// the invariants the validate-temp-mappings skill encodes:
//   - Min ≤ Max for every {S,P,E,GPU}Cores pair.
//   - PairSignature is one of P+E / S+E / S+P.
//   - The "absent" core type for the pair signature has zero Min/Max
//     (e.g. M5 Pro is S+P → ECoresMax must be zero; M5 base is S+E → PCoresMax
//     must be zero).
//   - Dies is 1 (monolithic) or 2 (UltraFusion).
//   - GPUCoresMax is positive (every shipped Apple Silicon SKU has GPU cores).
func Test_skuLayoutsIntegrity(t *testing.T) {
	t.Parallel()

	for sku, layout := range skuLayouts {
		t.Run(sku, func(t *testing.T) {
			t.Parallel()

			assert.LessOrEqual(t, layout.SCoresMin, layout.SCoresMax,
				"%s: SCoresMin > SCoresMax", sku)
			assert.LessOrEqual(t, layout.PCoresMin, layout.PCoresMax,
				"%s: PCoresMin > PCoresMax", sku)
			assert.LessOrEqual(t, layout.ECoresMin, layout.ECoresMax,
				"%s: ECoresMin > ECoresMax", sku)
			assert.LessOrEqual(t, layout.GPUCoresMin, layout.GPUCoresMax,
				"%s: GPUCoresMin > GPUCoresMax", sku)

			assert.Contains(t,
				[]string{PairSignaturePE, PairSignatureSE, PairSignatureSP},
				layout.PairSignature,
				"%s: PairSignature %q is not one of the known constants", sku, layout.PairSignature)

			switch layout.PairSignature {
			case PairSignaturePE:
				assert.Zero(t, layout.SCoresMax, "%s: P+E SKU must have no S-cores", sku)
				assert.Positive(t, layout.PCoresMax, "%s: P+E SKU must have P-cores", sku)
				assert.Positive(t, layout.ECoresMax, "%s: P+E SKU must have E-cores", sku)
			case PairSignatureSE:
				assert.Zero(t, layout.PCoresMax, "%s: S+E SKU must have no P-cores", sku)
				assert.Positive(t, layout.SCoresMax, "%s: S+E SKU must have S-cores", sku)
				assert.Positive(t, layout.ECoresMax, "%s: S+E SKU must have E-cores", sku)
			case PairSignatureSP:
				assert.Zero(t, layout.ECoresMax, "%s: S+P SKU must have no E-cores", sku)
				assert.Positive(t, layout.SCoresMax, "%s: S+P SKU must have S-cores", sku)
				assert.Positive(t, layout.PCoresMax, "%s: S+P SKU must have P-cores", sku)
			}

			assert.Contains(t, []int{1, 2}, layout.Dies,
				"%s: Dies %d not in {1, 2}", sku, layout.Dies)
			assert.Positive(t, layout.GPUCoresMax,
				"%s: GPUCoresMax must be > 0", sku)
		})
	}
}

// Test_skuLayouts_coverage verifies that every Apple Silicon SKU referenced from
// the products map has a corresponding entry in skuLayouts. Intel SKUs are
// excluded — they have no per-core SMC convention worth modelling here.
func Test_skuLayouts_coverage(t *testing.T) {
	t.Parallel()

	for modelID, p := range products {
		if p.Family == "Intel" {
			continue
		}

		_, ok := skuLayouts[p.CPU]
		assert.True(t, ok,
			"model %q (CPU %q, family %q) has no entry in skuLayouts; add it to the family roster",
			modelID, p.CPU, p.Family)
	}
}

// Test_LookupSKULayout_known verifies lookup of a few well-known SKUs returns
// the expected pair signature and core counts. Catches column-swap regressions.
func Test_LookupSKULayout_known(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cpu                    string
		wantPairSig            string
		wantPMin, wantPMax     int
		wantEMin, wantEMax     int
		wantSMin, wantSMax     int
		wantDies               int
		wantGPUMin, wantGPUMax int
	}{
		// Base M1: 4P+4E, monolithic, 7 or 8 GPU cores.
		{"M1", PairSignaturePE, 4, 4, 4, 4, 0, 0, 1, 7, 8},
		// M1 Ultra: dual-die fused 16P+4E.
		{"M1 Ultra", PairSignaturePE, 16, 16, 4, 4, 0, 0, 2, 48, 64},
		// M3 Pro: variable P-count (5 or 6), 6E baseline.
		{"M3 Pro", PairSignaturePE, 5, 6, 6, 6, 0, 0, 1, 14, 18},
		// M5 base: S+E, no P-cores.
		{"M5", PairSignatureSE, 0, 0, 6, 6, 3, 4, 1, 8, 10},
		// M5 Pro: S+P, no E-cores.
		{"M5 Pro", PairSignatureSP, 10, 12, 0, 0, 5, 6, 1, 16, 20},
		// A18 Pro: P+E.
		{"A18 Pro", PairSignaturePE, 2, 2, 4, 4, 0, 0, 1, 6, 6},
	}

	for _, tt := range tests {
		t.Run(tt.cpu, func(t *testing.T) {
			t.Parallel()

			got, ok := LookupSKULayout(tt.cpu)
			assert.True(t, ok, "%s: must be present", tt.cpu)
			assert.Equal(t, tt.wantPairSig, got.PairSignature, "%s: PairSignature", tt.cpu)
			assert.Equal(t, tt.wantPMin, got.PCoresMin, "%s: PCoresMin", tt.cpu)
			assert.Equal(t, tt.wantPMax, got.PCoresMax, "%s: PCoresMax", tt.cpu)
			assert.Equal(t, tt.wantEMin, got.ECoresMin, "%s: ECoresMin", tt.cpu)
			assert.Equal(t, tt.wantEMax, got.ECoresMax, "%s: ECoresMax", tt.cpu)
			assert.Equal(t, tt.wantSMin, got.SCoresMin, "%s: SCoresMin", tt.cpu)
			assert.Equal(t, tt.wantSMax, got.SCoresMax, "%s: SCoresMax", tt.cpu)
			assert.Equal(t, tt.wantDies, got.Dies, "%s: Dies", tt.cpu)
			assert.Equal(t, tt.wantGPUMin, got.GPUCoresMin, "%s: GPUCoresMin", tt.cpu)
			assert.Equal(t, tt.wantGPUMax, got.GPUCoresMax, "%s: GPUCoresMax", tt.cpu)
		})
	}
}

// Test_LookupSKULayout_unknown verifies that an unknown SKU string returns the
// zero SKULayout and false, so callers can use the boolean to skip validation.
func Test_LookupSKULayout_unknown(t *testing.T) {
	t.Parallel()

	got, ok := LookupSKULayout("M99 Imaginary")
	assert.False(t, ok)
	assert.Equal(t, SKULayout{}, got)
}
