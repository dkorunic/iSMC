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
	"fmt"
	"math"

	"github.com/dkorunic/iSMC/gosmc"
)

// getKeyFloat32 returns float32 value for a given SMC key.
func getKeyFloat32(c uint, key string) (float32, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return 0.0, "", fmt.Errorf("SMCReadKey(%q): result %v, dataSize %d", key, res, v.DataSize)
	}

	t := smcTypeToString(v.DataType)

	// Ta0P is mislabeled as 'flt' but actually uses 'sp78' format (2-byte signed fixed-point).
	if t == gosmc.TypeFLT && key == "Ta0P" && v.DataSize >= 2 {
		val, err := fpToFloat32("sp78", v.Bytes, v.DataSize)

		return val, t, err
	}

	switch t {
	// ui8/ui16/ui32 SMC types
	// TODO: Proper "hex_" handling
	case gosmc.TypeUI8, gosmc.TypeUI16, gosmc.TypeUI32, "hex_":
		return smcBytesToFloat32(v.Bytes, v.DataSize), t, nil
	// flt values can encode IEEE 754 NaN or Inf if a sensor slot is unused; reject them.
	case gosmc.TypeFLT:
		val, ok := decodeToFloat32(t, v.Bytes, v.DataSize)
		if !ok {
			return 0.0, "", fmt.Errorf("unable to decode SMC type %q to float32", t)
		}

		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			return 0.0, "", fmt.Errorf("SMC key %q has non-finite flt value", key)
		}

		return val, t, nil
	// ioft, fp*, sp* types
	default:
		val, ok := decodeToFloat32(t, v.Bytes, v.DataSize)
		if !ok {
			return 0.0, "", fmt.Errorf("unable to decode SMC type %q to float32", t)
		}

		return val, t, nil
	}
}

// getKeyUint32 returns uint32 value for a given SMC key.
func getKeyUint32(c uint, key string) (uint32, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return 0, "", fmt.Errorf("SMCReadKey(%q): result %v, dataSize %d", key, res, v.DataSize)
	}

	t := smcTypeToString(v.DataType)
	switch t {
	// TODO: Proper "hex_" handling
	case gosmc.TypeUI8, gosmc.TypeUI16, gosmc.TypeUI32, "hex_":
		return smcBytesToUint32(v.Bytes, v.DataSize), t, nil
	default:
		return 0, "", fmt.Errorf("unable to convert to uint32 type %q, bytes %v", t,
			v.Bytes[:v.DataSize])
	}
}

// getKeyBool returns bool value for a given SMC key.
func getKeyBool(c uint, key string) (bool, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return false, "", fmt.Errorf("SMCReadKey(%q): result %v, dataSize %d", key, res, v.DataSize)
	}

	t := smcTypeToString(v.DataType)
	switch t {
	case gosmc.TypeFLAG:
		return smcBytesToUint32(v.Bytes, v.DataSize) == uint32(1), t, nil
	default:
		return false, "", fmt.Errorf("unable to convert to bool type %q, bytes %v", t,
			v.Bytes[:v.DataSize])
	}
}
