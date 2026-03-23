// Copyright (C) 2019  Dinko Korunic
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

// makeBytes creates a gosmc.SMCBytes with the provided bytes at the start.
func makeBytes(b ...byte) gosmc.SMCBytes {
	var result gosmc.SMCBytes
	copy(result[:], b)

	return result
}

// makeUInt32Char creates a gosmc.UInt32Char from a string (up to 5 bytes).
func makeUInt32Char(s string) gosmc.UInt32Char {
	var result gosmc.UInt32Char
	copy(result[:], s)

	return result
}

func Test_fpToFloat32(t *testing.T) {
	tests := []struct {
		name     string
		smcType  string
		bytes    gosmc.SMCBytes
		size     uint32
		expected float32
		wantErr  bool
	}{
		// fp88: unsigned, divisor 256 — 0x1900 = 6400, 6400/256 = 25.0
		{"fp88 25 degrees", "fp88", makeBytes(0x19, 0x00), 2, 25.0, false},
		// sp78: signed, divisor 256 — 0x1900 = 6400, 6400/256 = 25.0
		{"sp78 25 degrees", "sp78", makeBytes(0x19, 0x00), 2, 25.0, false},
		// sp78: 0xFF00 = 65280 unsigned → int16 = -256, -256/256 = -1.0
		{"sp78 negative", "sp78", makeBytes(0xFF, 0x00), 2, -1.0, false},
		// fp1f: unsigned, divisor 32768 — 0x8000 = 32768, 32768/32768 = 1.0
		{"fp1f 1.0", "fp1f", makeBytes(0x80, 0x00), 2, 1.0, false},
		{"unknown type", "xxxx", makeBytes(0x00, 0x00), 2, 0.0, true},
		{"size too small", "fp88", makeBytes(0x00), 1, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fpToFloat32(tt.smcType, tt.bytes, tt.size)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func Test_fltToFloat32(t *testing.T) {
	tests := []struct {
		name     string
		bytes    gosmc.SMCBytes
		size     uint32
		expected float32
		wantErr  bool
	}{
		// IEEE 754 LE: 0x41C80000 = 25.0
		{"25.0 degrees", makeBytes(0x00, 0x00, 0xC8, 0x41), 4, 25.0, false},
		{"zero", makeBytes(0x00, 0x00, 0x00, 0x00), 4, 0.0, false},
		// IEEE 754 LE: 0x3F800000 = 1.0
		{"1.0", makeBytes(0x00, 0x00, 0x80, 0x3F), 4, 1.0, false},
		{"size too small", makeBytes(0x00), 1, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fltToFloat32("flt", tt.bytes, tt.size)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func Test_smcTypeToString(t *testing.T) {
	tests := []struct {
		name     string
		input    gosmc.UInt32Char
		expected string
	}{
		{"sp78", makeUInt32Char("sp78"), "sp78"},
		{"trailing null", makeUInt32Char("flt\x00"), "flt"},
		{"trailing space", makeUInt32Char("ui8 "), "ui8"},
		{"null only", makeUInt32Char("\x00\x00\x00\x00\x00"), ""},
		{"fp88", makeUInt32Char("fp88"), "fp88"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := smcTypeToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_smcBytesToUint32(t *testing.T) {
	tests := []struct {
		name     string
		bytes    gosmc.SMCBytes
		size     uint32
		expected uint32
	}{
		{"zero 4 bytes", makeBytes(0x00, 0x00, 0x00, 0x00), 4, 0},
		// BigEndian: 0x00000001 = 1
		{"one 4 bytes", makeBytes(0x00, 0x00, 0x00, 0x01), 4, 1},
		// BigEndian: 0x00010000 = 65536
		{"big endian 65536", makeBytes(0x00, 0x01, 0x00, 0x00), 4, 65536},
		{"ui8 one", makeBytes(0x01), 1, 1},
		// BigEndian: 0x0100 = 256
		{"ui16 256", makeBytes(0x01, 0x00), 2, 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := smcBytesToUint32(tt.bytes, tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_smcBytesToFloat32(t *testing.T) {
	tests := []struct {
		name     string
		bytes    gosmc.SMCBytes
		size     uint32
		expected float32
	}{
		{"zero", makeBytes(0x00, 0x00, 0x00, 0x00), 4, 0.0},
		{"one", makeBytes(0x00, 0x00, 0x00, 0x01), 4, 1.0},
		{"ui8 three", makeBytes(0x03), 1, 3.0},
		{"ui8 255", makeBytes(0xFF), 1, 255.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := smcBytesToFloat32(tt.bytes, tt.size)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

// Test_AppleFPConvTable validates the AppleFPConv lookup table entries that are most
// likely to be corrupted by a transcription error. TC-5 targets sp78 specifically
// because it is the most common SMC temperature type and a wrong divisor (e.g. 128
// instead of 256) would silently double every temperature reading.
func Test_AppleFPConvTable(t *testing.T) {
	tests := []struct {
		smcType    string
		wantDiv    float32
		wantSigned bool
	}{
		// TC-5: sp78 — 8 integer + 8 fractional bits → divisor must be 2^8 = 256
		{"sp78", 256.0, true},
		// Verify a few neighbours to catch off-by-one mistakes in the table
		{"sp87", 128.0, true},
		{"sp96", 64.0, true},
		{"fp88", 256.0, false},
		{"fp79", 512.0, false},
		{"fp1f", 32768.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.smcType, func(t *testing.T) {
			v, ok := AppleFPConv[tt.smcType]
			assert.True(t, ok, "type %q must be present in AppleFPConv", tt.smcType)
			assert.Equal(t, tt.wantDiv, v.Div, "type %q divisor must be %g", tt.smcType, tt.wantDiv)
			assert.Equal(t, tt.wantSigned, v.Signed, "type %q signed flag must be %v", tt.smcType, tt.wantSigned)
		})
	}
}

// Test_fpToFloat32_bigEndianAsymmetric explicitly verifies that fpToFloat32 reads
// bytes in big-endian order (TC-1). Using the asymmetric input 0x01 0x00:
//   - big-endian:    0x0100 = 256; 256/256 = 1.0  (correct)
//   - little-endian: 0x0001 = 1;   1/256   = 0.0039 (wrong)
func Test_fpToFloat32_bigEndianAsymmetric(t *testing.T) {
	result, err := fpToFloat32("fp88", makeBytes(0x01, 0x00), 2)
	assert.NoError(t, err)
	assert.InDelta(t, 1.0, result, 0.001, "fp88 0x01 0x00 must be 1.0 under big-endian interpretation")
}

// Test_fltToFloat32_littleEndianAsymmetric verifies that fltToFloat32 reads bytes
// in little-endian order (TC-2). IEEE 754 float 1.0 in little-endian is 0x00 0x00 0x80 0x3F;
// big-endian would produce a very different (garbage) float.
func Test_fltToFloat32_littleEndianAsymmetric(t *testing.T) {
	// 0x3F800000 = 1.0 in big-endian; stored as 0x00 0x00 0x80 0x3F in little-endian
	result, err := fltToFloat32("flt", makeBytes(0x00, 0x00, 0x80, 0x3F), 4)
	assert.NoError(t, err)
	assert.InDelta(t, 1.0, result, 0.001, "flt 0x00 0x00 0x80 0x3F must be 1.0 under little-endian interpretation")
}

// Test_smcBytesToUint32_bigEndianAsymmetric explicitly verifies that smcBytesToUint32
// assembles bytes in big-endian order (TC-3). Using 0x01 0x00:
//   - big-endian:    0x0100 = 256 (correct)
//   - little-endian: 0x0001 = 1   (wrong)
func Test_smcBytesToUint32_bigEndianAsymmetric(t *testing.T) {
	result := smcBytesToUint32(makeBytes(0x01, 0x00), 2)
	assert.Equal(t, uint32(256), result, "smcBytesToUint32 must use big-endian byte order")
}

// Test_ioftToFloat32_divisor verifies that ioftToFloat32 uses the correct 2^16 = 65536
// divisor (TC-4). Using a raw value of 131072 (2^17 in the 48.16 fixed-point word):
//   - correct divisor 65536: 131072/65536 = 2.0
//   - wrong divisor  32768: 131072/32768 = 4.0
func Test_ioftToFloat32_divisor(t *testing.T) {
	// LittleEndian uint64: place 131072 (0x00020000) at bytes [2:4]
	result, err := ioftToFloat32(makeBytes(0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00), 8)
	assert.NoError(t, err)
	assert.InDelta(t, 2.0, result, 0.001, "ioftToFloat32 must divide by 65536 (2^16)")
}

func Test_ioftToFloat32(t *testing.T) {
	tests := []struct {
		name     string
		bytes    gosmc.SMCBytes
		size     uint32
		expected float32
		wantErr  bool
	}{
		// LittleEndian Uint64: 0x0000000000010000 = 65536; 65536/65536 = 1.0
		{"one", makeBytes(0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00), 8, 1.0, false},
		{"zero", makeBytes(0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00), 8, 0.0, false},
		// LittleEndian: 0x0000000000020000 = 131072; 131072/65536 = 2.0
		{"two", makeBytes(0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00), 8, 2.0, false},
		{"size too small", makeBytes(0x00), 1, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ioftToFloat32(tt.bytes, tt.size)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}
