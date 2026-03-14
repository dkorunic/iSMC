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

	"github.com/dkorunic/iSMC/gosmc"
)

// getKeyFloat32 returns float32 value for a given SMC key.
func getKeyFloat32(c uint, key string) (float32, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return 0.0, "", nil
	}

	t := smcTypeToString(v.DataType)

	// Some sensors are mislabeled as 'flt' but actually use 'sp78' format (2-byte fixed-point)
	// The first 2 bytes contain the sp78 value
	// Ta0P is known to be affected on M1/M2 Macs
	if t == gosmc.TypeFLT && v.DataSize >= 2 {
		if key == "Ta0P" {
			// Read first 2 bytes as big-endian int16 and convert to sp78 format
			raw := uint16(v.Bytes[0])<<8 | uint16(v.Bytes[1])
			return float32(int16(raw)) / 256.0, t, nil
		}
	}

	switch t {
	// flt SMC type
	case gosmc.TypeFLT:
		res, err := fltToFloat32(t, v.Bytes, v.DataSize)

		return res, t, err
	// ui8/ui16/ui32 SMC types
	// TODO: Proper "hex_" handling
	case gosmc.TypeUI8, gosmc.TypeUI16, gosmc.TypeUI32, "hex_":
		return smcBytesToFloat32(v.Bytes, v.DataSize), t, nil
		// ioft SMC type
	case "ioft":
		return ioftToFloat32(v.Bytes, v.DataSize), t, nil
	// fp* SMC types
	default:
		res, err := fpToFloat32(t, v.Bytes, v.DataSize)

		return res, t, err
	}
}

// getKeyUint32 returns uint32 value for a given SMC key.
func getKeyUint32(c uint, key string) (uint32, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return 0, "", nil
	}

	t := smcTypeToString(v.DataType)
	switch t {
	// TODO: Proper "hex_" handling
	case gosmc.TypeUI8, gosmc.TypeUI16, gosmc.TypeUI32, "hex_":
		return smcBytesToUint32(v.Bytes, v.DataSize), t, nil
	default:
		return 0, "", fmt.Errorf("unable to convert to uint32 type %q, bytes %v to float32", t,
			v.Bytes[:v.DataSize])
	}
}

// getKeyBool returns bool value for a given SMC key.
func getKeyBool(c uint, key string) (bool, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return false, "", nil
	}

	t := smcTypeToString(v.DataType)
	switch t {
	case gosmc.TypeFLAG:
		return smcBytesToUint32(v.Bytes, v.DataSize) == uint32(1), t, nil
	default:
		return false, "", fmt.Errorf("unable to convert to bool type %q, bytes %v to float32", t,
			v.Bytes[:v.DataSize])
	}
}
