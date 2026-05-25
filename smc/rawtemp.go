// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package smc

import (
	"math"

	"github.com/dkorunic/iSMC/gosmc"
)

// Plausible temperature window; rejects firmware-bug outliers like M5 Pro TTPD's −3e8 °C.
const (
	rawTempMin = float32(-100.0)
	rawTempMax = float32(200.0)
)

// RawKeyToFloat32 converts a RawKey's raw bytes to a float32 value using the SMC type
// information stored in the key.
//
// Supported types: flt, ioft, and all fp*/sp* fixed-point variants.
// The Ta0P sensor is handled specially: it is labelled "flt" by the SMC but actually
// encodes its value in sp78 (signed 7.8 fixed-point) format.
//
// Returns (value, true) on success and (0, false) for unsupported types, insufficient
// data, non-finite results, or values outside the plausible temperature window.
func RawKeyToFloat32(k RawKey) (float32, bool) {
	// Ta0P: mislabelled flt, decode as sp78.
	if k.DataType == gosmc.TypeFLT && k.Key == "Ta0P" && k.DataSize >= 2 {
		v, err := fpToFloat32("sp78", k.Bytes, k.DataSize)
		if err != nil {
			return 0, false
		}

		return finiteInRange(v)
	}

	switch k.DataType {
	case gosmc.TypeFLT:
		v, err := fltToFloat32(k.Bytes, k.DataSize)
		if err != nil {
			return 0, false
		}

		return finiteInRange(v)

	case "ioft":
		v, err := ioftToFloat32(k.Bytes, k.DataSize)
		if err != nil {
			return 0, false
		}

		return finiteInRange(v)

	default:
		v, err := fpToFloat32(k.DataType, k.Bytes, k.DataSize)
		if err != nil {
			return 0, false
		}

		return finiteInRange(v)
	}
}

// finiteInRange returns (v, true) if v is finite and within the temperature
// sanity window, otherwise (0, false). Centralising the check keeps every type
// branch in RawKeyToFloat32 in lockstep.
func finiteInRange(v float32) (float32, bool) {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return 0, false
	}

	if v < rawTempMin || v > rawTempMax {
		return 0, false
	}

	return v, true
}
