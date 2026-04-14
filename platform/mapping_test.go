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
