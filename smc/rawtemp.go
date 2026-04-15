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
	"math"

	"github.com/dkorunic/iSMC/gosmc"
)

// RawKeyToFloat32 converts a RawKey's raw bytes to a float32 value using the SMC type
// information stored in the key.
//
// Supported types: flt, ioft, and all fp*/sp* fixed-point variants.
// The Ta0P sensor is handled specially: it is labelled "flt" by the SMC but actually
// encodes its value in sp78 (signed 7.8 fixed-point) format.
//
// Returns (value, true) on success and (0, false) for unsupported types, insufficient
// data, or non-finite results.
func RawKeyToFloat32(k RawKey) (float32, bool) {
	// Ta0P is mislabelled as flt but is actually sp78 – apply the same workaround as
	// getKeyFloat32 in get.go.
	if k.DataType == gosmc.TypeFLT && k.Key == "Ta0P" && k.DataSize >= 2 {
		v, err := fpToFloat32("sp78", k.Bytes, k.DataSize)
		if err != nil {
			return 0, false
		}

		return v, true
	}

	switch k.DataType {
	case gosmc.TypeFLT:
		v, err := fltToFloat32(k.Bytes, k.DataSize)
		if err != nil {
			return 0, false
		}

		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return 0, false
		}

		return v, true

	case "ioft":
		v, err := ioftToFloat32(k.Bytes, k.DataSize)
		if err != nil {
			return 0, false
		}

		return v, true

	default:
		// Covers all fp*/sp* fixed-point types via AppleFPConv.
		v, err := fpToFloat32(k.DataType, k.Bytes, k.DataSize)
		if err != nil {
			return 0, false
		}

		return v, true
	}
}
