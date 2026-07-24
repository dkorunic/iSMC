// SPDX-FileCopyrightText: Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package smc

import (
	"errors"
	"fmt"
	"math"

	"github.com/dkorunic/iSMC/gosmc"
)

// Pre-built sentinel avoids fmt.Errorf allocs on the hot miss path.
var errNoValue = errors.New("smc: no value")

// Sentinel errors for SMC type-decode failures, wrapped with dynamic context
// (SMC type, key, raw bytes) at each call site.
var (
	errDecodeFloat32 = errors.New("unable to decode SMC type to float32")
	errNonFiniteFlt  = errors.New("SMC key has non-finite flt value")
	errDecodeUint32  = errors.New("unable to convert SMC type to uint32")
	errDecodeBool    = errors.New("unable to convert SMC type to bool")
)

// getKeyFloat32 returns float32 value for a given SMC key.
func getKeyFloat32(c uint, key string) (float32, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return 0.0, "", errNoValue
	}

	t := smcTypeToString(v.DataType)

	// Ta0P: mislabelled flt, decode as sp78.
	if t == gosmc.TypeFLT && key == "Ta0P" && v.DataSize >= 2 {
		val, err := fpToFloat32("sp78", v.Bytes, v.DataSize)

		return val, t, err
	}

	switch t {
	// hex_ is decoded as an unsigned integer for now.
	case gosmc.TypeUI8, gosmc.TypeUI16, gosmc.TypeUI32, smcTypeHex:
		return smcBytesToFloat32(v.Bytes, v.DataSize), t, nil
	// Reject NaN/Inf from unused flt slots.
	case gosmc.TypeFLT:
		val, ok := decodeToFloat32(t, v.Bytes, v.DataSize)
		if !ok {
			return 0.0, "", fmt.Errorf("%w: type %q", errDecodeFloat32, t)
		}

		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			return 0.0, "", fmt.Errorf("%w: key %q", errNonFiniteFlt, key)
		}

		return val, t, nil
	default:
		val, ok := decodeToFloat32(t, v.Bytes, v.DataSize)
		if !ok {
			return 0.0, "", fmt.Errorf("%w: type %q", errDecodeFloat32, t)
		}

		return val, t, nil
	}
}

// getKeyUint32 returns uint32 value for a given SMC key.
func getKeyUint32(c uint, key string) (uint32, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return 0, "", errNoValue
	}

	t := smcTypeToString(v.DataType)
	switch t {
	// hex_ is decoded as an unsigned integer for now.
	case gosmc.TypeUI8, gosmc.TypeUI16, gosmc.TypeUI32, smcTypeHex:
		return smcBytesToUint32(v.Bytes, v.DataSize), t, nil
	default:
		return 0, "", fmt.Errorf("%w: type %q, bytes %v", errDecodeUint32, t,
			v.Bytes[:min(int(v.DataSize), len(v.Bytes))])
	}
}

// getKeyBool returns bool value for a given SMC key.
func getKeyBool(c uint, key string) (bool, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return false, "", errNoValue
	}

	t := smcTypeToString(v.DataType)
	switch t {
	case gosmc.TypeFLAG:
		return smcBytesToUint32(v.Bytes, v.DataSize) == uint32(1), t, nil
	default:
		return false, "", fmt.Errorf("%w: type %q, bytes %v", errDecodeBool, t,
			v.Bytes[:min(int(v.DataSize), len(v.Bytes))])
	}
}
