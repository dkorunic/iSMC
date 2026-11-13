// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package smc

import (
	"testing"

	"github.com/dkorunic/iSMC/gosmc"
	"github.com/stretchr/testify/assert"
)

// Test_RawKeyToFloat32 verifies the full type-dispatch and NaN/Inf-rejection logic of
// RawKeyToFloat32 without requiring a live SMC connection.
//
// TypeFLT values that decode to NaN or ±Inf must be rejected (return false), while
// finite values are accepted. Ta0P is decoded as its declared flt type (every
// dump in internal/reports holds a valid little-endian float there).
func Test_RawKeyToFloat32(t *testing.T) {
	tests := []struct {
		name    string
		key     RawKey
		wantVal float32
		wantOK  bool
	}{
		// TypeFLT (IEEE 754 little-endian).
		{
			// 0x41C80000 LE = 25.0.
			name:    "flt 25.0",
			key:     RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0xC8, 0x41)},
			wantVal: 25.0,
			wantOK:  true,
		},
		{
			name:   "flt too small",
			key:    RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 1, Bytes: makeBytes(0x00)},
			wantOK: false,
		},
		{
			// 0x7FC00000 LE = quiet NaN.
			name:   "flt NaN rejected",
			key:    RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0xC0, 0x7F)},
			wantOK: false,
		},
		{
			// 0x7F800000 LE = +Inf.
			name:   "flt +Inf rejected",
			key:    RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0x80, 0x7F)},
			wantOK: false,
		},
		{
			// 0xFF800000 LE = -Inf.
			name:   "flt -Inf rejected",
			key:    RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0x80, 0xFF)},
			wantOK: false,
		},

		// ioft (48.16 unsigned fixed-point, little-endian).
		{
			// LE u64 0x10000 = 65536; /65536 = 1.0.
			name:    "ioft 1.0",
			key:     RawKey{Key: "IOFT", DataType: "ioft", DataSize: 8, Bytes: makeBytes(0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00)},
			wantVal: 1.0,
			wantOK:  true,
		},
		{
			name:   "ioft too small",
			key:    RawKey{Key: "IOFT", DataType: "ioft", DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0x01, 0x00)},
			wantOK: false,
		},
		{
			// ≈10¹⁴ °C; observed on report-m4-4.txt TR3d.
			name:   "ioft above rawTempMax rejected",
			key:    RawKey{Key: "TR3d", DataType: "ioft", DataSize: 8, Bytes: makeBytes(0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF)},
			wantOK: false,
		},

		// fp*/sp* fixed-point (big-endian).
		{
			// BE 0x1900 = 6400 / 256 = 25.0.
			name:    "sp78 25.0",
			key:     RawKey{Key: "TC0H", DataType: gosmc.TypeSP78, DataSize: 2, Bytes: makeBytes(0x19, 0x00)},
			wantVal: 25.0,
			wantOK:  true,
		},
		{
			// BE 0x0100 = 256 / 256 = 1.0.
			name:    "fp88 1.0",
			key:     RawKey{Key: "TC0H", DataType: gosmc.TypeFP88, DataSize: 2, Bytes: makeBytes(0x01, 0x00)},
			wantVal: 1.0,
			wantOK:  true,
		},

		{
			name:   "unknown type",
			key:    RawKey{Key: "TC0H", DataType: "xxxx", DataSize: 2, Bytes: makeBytes(0x00, 0x00)},
			wantOK: false,
		},

		{
			// 0x40C66666 LE = 6.2; an sp78 read of the same bytes would give 102.4.
			name:    "Ta0P decoded as plain flt",
			key:     RawKey{Key: "Ta0P", DataType: gosmc.TypeFLT, DataSize: 4, Bytes: makeBytes(0x66, 0x66, 0xC6, 0x40)},
			wantVal: 6.2,
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOK := RawKeyToFloat32(tt.key)
			assert.Equal(t, tt.wantOK, gotOK, "RawKeyToFloat32(%+v).ok", tt.key)

			if tt.wantOK {
				assert.InDelta(t, tt.wantVal, gotVal, 0.001,
					"RawKeyToFloat32(%+v) value", tt.key)
			}
		})
	}
}
