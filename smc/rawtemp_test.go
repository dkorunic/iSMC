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

package smc

import (
	"testing"

	"github.com/dkorunic/iSMC/gosmc"
	"github.com/stretchr/testify/assert"
)

// Test_RawKeyToFloat32 verifies the full type-dispatch and NaN/Inf-rejection logic of
// RawKeyToFloat32 without requiring a live SMC connection.
//
// Special cases covered:
//   - Ta0P is mislabelled as "flt" by the SMC firmware but encodes its value in sp78;
//     the workaround in RawKeyToFloat32 must decode it via fpToFloat32("sp78",...).
//   - TypeFLT values that decode to NaN or ±Inf must be rejected (return false), while
//     finite values are accepted.
func Test_RawKeyToFloat32(t *testing.T) {
	tests := []struct {
		name    string
		key     RawKey
		wantVal float32
		wantOK  bool
	}{
		// ── TypeFLT (IEEE 754 little-endian) ─────────────────────────────────
		{
			// 0x41C80000 = 25.0 in big-endian; LE bytes: 0x00,0x00,0xC8,0x41
			name:    "flt 25.0",
			key:     RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0xC8, 0x41)},
			wantVal: 25.0,
			wantOK:  true,
		},
		{
			// size < 4 → fltToFloat32 returns error
			name:   "flt too small",
			key:    RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 1, Bytes: makeBytes(0x00)},
			wantOK: false,
		},
		{
			// 0x7FC00000 = quiet NaN; LE bytes: 0x00,0x00,0xC0,0x7F → must be rejected
			name:   "flt NaN rejected",
			key:    RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0xC0, 0x7F)},
			wantOK: false,
		},
		{
			// 0x7F800000 = +Inf; LE bytes: 0x00,0x00,0x80,0x7F → must be rejected
			name:   "flt +Inf rejected",
			key:    RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0x80, 0x7F)},
			wantOK: false,
		},
		{
			// 0xFF800000 = -Inf; LE bytes: 0x00,0x00,0x80,0xFF → must be rejected
			name:   "flt -Inf rejected",
			key:    RawKey{Key: "TC0H", DataType: gosmc.TypeFLT, DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0x80, 0xFF)},
			wantOK: false,
		},

		// ── ioft (48.16 unsigned fixed-point, little-endian) ─────────────────
		{
			// LittleEndian uint64: 0x0000000000010000=65536 → 65536/65536=1.0
			name:    "ioft 1.0",
			key:     RawKey{Key: "IOFT", DataType: "ioft", DataSize: 8, Bytes: makeBytes(0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00)},
			wantVal: 1.0,
			wantOK:  true,
		},
		{
			// size < 8 → ioftToFloat32 returns error
			name:   "ioft too small",
			key:    RawKey{Key: "IOFT", DataType: "ioft", DataSize: 4, Bytes: makeBytes(0x00, 0x00, 0x01, 0x00)},
			wantOK: false,
		},

		// ── fp*/sp* fixed-point (big-endian) via AppleFPConv ─────────────────
		{
			// sp78: BigEndian 0x1900=6400 → int16(6400)/256=25.0
			name:    "sp78 25.0",
			key:     RawKey{Key: "TC0H", DataType: gosmc.TypeSP78, DataSize: 2, Bytes: makeBytes(0x19, 0x00)},
			wantVal: 25.0,
			wantOK:  true,
		},
		{
			// fp88: BigEndian 0x0100=256 → 256/256=1.0
			name:    "fp88 1.0",
			key:     RawKey{Key: "TC0H", DataType: gosmc.TypeFP88, DataSize: 2, Bytes: makeBytes(0x01, 0x00)},
			wantVal: 1.0,
			wantOK:  true,
		},

		// ── Unknown / unsupported type ────────────────────────────────────────
		{
			// "xxxx" is not in AppleFPConv → fpToFloat32 returns error → false
			name:   "unknown type",
			key:    RawKey{Key: "TC0H", DataType: "xxxx", DataSize: 2, Bytes: makeBytes(0x00, 0x00)},
			wantOK: false,
		},

		// ── Ta0P sp78 workaround ──────────────────────────────────────────────
		{
			// Ta0P is reported as "flt" by firmware but actually encodes sp78.
			// With DataSize=2, DataType=TypeFLT and key="Ta0P", must decode as sp78.
			// BigEndian 0x1900=6400 → int16(6400)/256=25.0
			name:    "Ta0P sp78 workaround",
			key:     RawKey{Key: "Ta0P", DataType: gosmc.TypeFLT, DataSize: 2, Bytes: makeBytes(0x19, 0x00)},
			wantVal: 25.0,
			wantOK:  true,
		},
		{
			// Ta0P workaround path but size < 2 → fpToFloat32 returns error
			name:   "Ta0P too small for sp78",
			key:    RawKey{Key: "Ta0P", DataType: gosmc.TypeFLT, DataSize: 1, Bytes: makeBytes(0x19)},
			wantOK: false,
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
