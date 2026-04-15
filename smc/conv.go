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
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/dkorunic/iSMC/gosmc"
)

// FPConv type used for AppleFPConv map.
type FPConv struct {
	Div    float32
	Signed bool
}

// AppleFPConv maps floating point type conversion constants and signedness property.
var AppleFPConv = map[string]FPConv{
	"fp1f": {Div: 32768.0},
	"fp2e": {Div: 16384.0},
	"fp3d": {Div: 8192.0},
	"fp4c": {Div: 4096.0},
	"fp5b": {Div: 2048.0},
	"fp6a": {Div: 1024.0},
	"fp79": {Div: 512.0},
	"fp88": {Div: 256.0},
	"fpa6": {Div: 64.0},
	"fpc4": {Div: 16.0},
	"fpe2": {Div: 4.0},
	"sp1e": {Div: 16384.0, Signed: true},
	"sp2d": {Div: 8192.0, Signed: true},
	"sp3c": {Div: 4096.0, Signed: true},
	"sp4b": {Div: 2048.0, Signed: true},
	"sp5a": {Div: 1024.0, Signed: true},
	"sp69": {Div: 512.0, Signed: true},
	"sp78": {Div: 256.0, Signed: true},
	"sp87": {Div: 128.0, Signed: true},
	"sp96": {Div: 64.0, Signed: true},
	"spa5": {Div: 32.0, Signed: true},
	"spb4": {Div: 16.0, Signed: true},
	"spf0": {Div: 1.0, Signed: true},
}

// fpToFloat32 converts fp* SMC types to float32.
func fpToFloat32(t string, x gosmc.SMCBytes, size uint32) (float32, error) {
	if v, ok := AppleFPConv[t]; ok {
		if size < 2 {
			return 0.0, fmt.Errorf("fpToFloat32: size %d too small for type %q", size, t)
		}

		res := binary.BigEndian.Uint16(x[:2])
		if v.Signed {
			return float32(int16(res)) / v.Div, nil
		}

		return float32(res) / v.Div, nil
	}

	return 0.0, fmt.Errorf("unable to convert to float32 type %q, bytes %v", t, x)
}

// fltToFloat32 converts flt SMC type to float32.
func fltToFloat32(x gosmc.SMCBytes, size uint32) (float32, error) {
	if size < 4 {
		return 0.0, fmt.Errorf("fltToFloat32: size %d too small for flt type", size)
	}

	return math.Float32frombits(binary.LittleEndian.Uint32(x[:4])), nil
}

// smcTypeToString converts UInt32Char array to regular Go string removing trailing null and whitespace.
func smcTypeToString(x gosmc.UInt32Char) string {
	return strings.TrimRight(x.ToString(), "\x00 ")
}

// smcBytesToUint32 converts ui8/ui16/ui32 SMC types to uint32.
// size is clamped to 4 to prevent shift overflow for unexpected key sizes.
func smcBytesToUint32(x gosmc.SMCBytes, size uint32) uint32 {
	if size > 4 {
		size = 4
	}

	var total uint32
	for i := range size {
		total += uint32(x[i]) << ((size - 1 - i) * 8)
	}

	return total
}

// smcBytesToFloat32 converts ui8/ui16/ui32 SMC types to float32.
func smcBytesToFloat32(x gosmc.SMCBytes, size uint32) float32 {
	return float32(smcBytesToUint32(x, size))
}

// ioftToFloat32 converts ioft SMC type (48.16 unsigned fixed-point in LittleEndian) to float32.
func ioftToFloat32(x gosmc.SMCBytes, size uint32) (float32, error) {
	if size < 8 {
		return 0.0, fmt.Errorf("ioftToFloat32: size %d too small for ioft type", size)
	}

	res := binary.LittleEndian.Uint64(x[:8])

	return float32(res) / 65536.0, nil
}

// decodeToFloat32 converts bytes to float32 for continuous-valued SMC types: flt, ioft, and the
// fp*/sp* fixed-point families. Returns (value, true) on success, (0, false) for unknown types.
func decodeToFloat32(dataType string, bytes gosmc.SMCBytes, size uint32) (float32, bool) {
	switch dataType {
	case gosmc.TypeFLT:
		v, err := fltToFloat32(bytes, size)
		return v, err == nil
	case "ioft":
		v, err := ioftToFloat32(bytes, size)
		return v, err == nil
	default:
		// fp* and sp* fixed-point types
		v, err := fpToFloat32(dataType, bytes, size)
		return v, err == nil
	}
}

// DecodeValue converts raw SMC bytes to a human-readable string based on data type,
// mirroring smcFanControl's printVal logic. Returns an empty string for unrecognised types.
func DecodeValue(dataType string, bytes gosmc.SMCBytes, size uint32) string {
	if size == 0 {
		return ""
	}

	switch dataType {
	case gosmc.TypeUI8, gosmc.TypeUI16, gosmc.TypeUI32, "hex_":
		return fmt.Sprintf("%d", smcBytesToUint32(bytes, size))

	case gosmc.TypeSI8:
		return fmt.Sprintf("%d", int8(bytes[0]))

	case gosmc.TypeSI16:
		if size < 2 {
			return ""
		}
		return fmt.Sprintf("%d", int16(binary.BigEndian.Uint16(bytes[:2])))

	case gosmc.TypeSI32:
		if size < 4 {
			return ""
		}
		return fmt.Sprintf("%d", int32(binary.BigEndian.Uint32(bytes[:4])))

	case gosmc.TypeFLAG:
		return fmt.Sprintf("%v", smcBytesToUint32(bytes, size) == 1)

	case "pwm":
		if size < 2 {
			return ""
		}
		pct := float32(binary.BigEndian.Uint16(bytes[:2])) * 100.0 / 65536.0
		return fmt.Sprintf("%.1f%%", pct)

	default:
		// flt, ioft, fp*, sp* types
		v, ok := decodeToFloat32(dataType, bytes, size)
		if !ok {
			return ""
		}
		return fmt.Sprintf("%g", v)
	}
}
