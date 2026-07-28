// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package smc

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/dkorunic/iSMC/gosmc"
)

const (
	keyCount = "#KEY"

	// Guards against a corrupt/spoofed #KEY; the largest observed real values
	// are ~2300–2400 (A18: 2332, M2 Max: 2409 per OSHI/issue #37).
	maxKeys = 4096
)

// RawKey holds raw SMC key data for reporting.
type RawKey struct {
	Key      string
	DataType string
	DataSize uint32
	Bytes    gosmc.SMCBytes
}

// Open opens an SMC connection for callers issuing many reads over one connection
// (e.g. repeated sampling); the caller must release it with Close.
func Open() (uint, error) {
	return openSMC()
}

// Close releases an SMC connection returned by Open.
func Close(conn uint) {
	gosmc.SMCClose(conn)
}

// keyAtIndex returns the 4-char key name at enumeration index i over conn.
func keyAtIndex(conn uint, i uint32) (string, bool) {
	input := &gosmc.SMCKeyData{
		Data8:  gosmc.CMDReadIndex,
		Data32: i,
	}

	output, res := gosmc.SMCCall(conn, gosmc.KernelIndexSMC, input)
	if res != gosmc.IOReturnSuccess {
		return "", false
	}

	// Decode the uint32 key code as 4-char big-endian ASCII.
	var b [4]byte

	binary.BigEndian.PutUint32(b[:], output.Key)

	return string(b[:]), true
}

// keyTotal returns the firmware-reported key count over conn, capped at maxKeys.
func keyTotal(conn uint) (uint32, bool) {
	countVal, res := gosmc.SMCReadKey(conn, keyCount)
	if res != gosmc.IOReturnSuccess || countVal.DataSize == 0 {
		return 0, false
	}

	return min(smcBytesToUint32(countVal.Bytes, countVal.DataSize), maxKeys), true
}

// EnumerateKeys returns every SMC key name over conn without reading values, so a
// caller sampling a fixed subset can enumerate once and then read only its keys.
func EnumerateKeys(conn uint) []string {
	total, ok := keyTotal(conn)
	if !ok {
		return nil
	}

	names := make([]string, 0, total)

	for i := range total {
		if k, ok := keyAtIndex(conn, i); ok {
			names = append(names, k)
		}
	}

	return names
}

// scanKeysByPrefix collects the contiguous key block matching prefix via binary
// search over keyAt. It assumes ascending ASCII index order — observed on every
// machine in internal/reports, but undocumented firmware behaviour — so a failed
// probe or order violation returns ok=false, requiring the caller to fall back
// to a full enumeration walk. O(log total + block length) probes.
func scanKeysByPrefix(total uint32, keyAt func(uint32) (string, bool), prefix string) ([]string, bool) {
	lo, hi := uint32(0), total

	for lo < hi {
		mid := lo + (hi-lo)/2

		k, ok := keyAt(mid)
		if !ok {
			return nil, false
		}

		if k < prefix {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	var keys []string

	prev := ""

	for i := lo; i < total; i++ {
		k, ok := keyAt(i)
		if !ok {
			return nil, false
		}

		if !strings.HasPrefix(k, prefix) {
			break
		}

		if prev != "" && k < prev {
			return nil, false
		}

		prev = k
		keys = append(keys, k)
	}

	return keys, true
}

// EnumerateKeysPrefix returns the SMC key names starting with prefix in ~30
// IOKit round trips instead of a full ~2000-key walk. Any anomaly — failed
// probe, order violation, empty block — degrades to filtering EnumerateKeys,
// so unsorted firmware stays correct, just slow.
func EnumerateKeysPrefix(conn uint, prefix string) []string {
	if prefix == "" {
		return EnumerateKeys(conn)
	}

	total, ok := keyTotal(conn)
	if !ok {
		return nil
	}

	keys, ok := scanKeysByPrefix(total, func(i uint32) (string, bool) {
		return keyAtIndex(conn, i)
	}, prefix)
	if ok && len(keys) > 0 {
		return keys
	}

	var out []string

	for _, k := range EnumerateKeys(conn) {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}

	return out
}

// ReadRawKey reads a single key's raw value over conn, reporting read success.
func ReadRawKey(conn uint, key string) (RawKey, bool) {
	val, res := gosmc.SMCReadKey(conn, key)
	if res != gosmc.IOReturnSuccess {
		return RawKey{}, false
	}

	return RawKey{
		Key:      key,
		DataType: smcTypeToString(val.DataType),
		DataSize: val.DataSize,
		Bytes:    val.Bytes,
	}, true
}

// GetRaw returns all SMC keys with their raw byte values.
func GetRaw() []RawKey {
	conn, err := openSMC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)

		return nil
	}
	defer gosmc.SMCClose(conn)

	names := EnumerateKeys(conn)
	keys := make([]RawKey, 0, len(names))

	for _, name := range names {
		if rk, ok := ReadRawKey(conn, name); ok {
			keys = append(keys, rk)
		}
	}

	return keys
}
