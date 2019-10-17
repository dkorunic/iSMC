// @license
// Copyright (C) 2019  Dinko Korunic
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// +build darwin

package smc

import (
	"fmt"
	"github.com/panotza/gosmc"
)

// getKeyFloat32 returns float32 value for a given SMC key
func getKeyFloat32(c uint, key string) (float32, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return 0.0, "", nil
	}

	t := smcTypeToString(v.DataType)
	switch t {
	case gosmc.TypeFLT:
		res, err := fltToFloat32(t, v.Bytes, v.DataSize)
		return res, t, err
	default:
		res, err := fpToFloat32(t, v.Bytes, v.DataSize)
		return res, t, err
	}
}

// getKeyUint32 returns uint32 value for a given SMC key
func getKeyUint32(c uint, key string) (uint32, string, error) {
	v, res := gosmc.SMCReadKey(c, key)
	if res != gosmc.IOReturnSuccess || v.DataSize == 0 {
		return 0, "", nil
	}

	t := smcTypeToString(v.DataType)
	// missing hex_
	switch t {
	case gosmc.TypeUI8, gosmc.TypeUI16, gosmc.TypeUI32, "hex_":
		return smcBytesToUint32(v.Bytes, v.DataSize), t, nil
	default:
		return 0, "", fmt.Errorf("unable to convert to uint32 type %q, bytes %v to float32", t,
			v.Bytes[:v.DataSize])
	}
}

// getKeyBool returns bool value for a given SMC key
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
