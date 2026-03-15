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
