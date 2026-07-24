// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package smc

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/dkorunic/iSMC/gosmc"
)

const (
	keyCount = "#KEY"

	// Guards against a corrupt/spoofed #KEY; real Macs report ~1800.
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

// EnumerateKeys returns every SMC key name over conn without reading values, so a
// caller sampling a fixed subset can enumerate once and then read only its keys.
func EnumerateKeys(conn uint) []string {
	countVal, res := gosmc.SMCReadKey(conn, keyCount)
	if res != gosmc.IOReturnSuccess || countVal.DataSize == 0 {
		return nil
	}

	total := min(smcBytesToUint32(countVal.Bytes, countVal.DataSize), maxKeys)
	names := make([]string, 0, total)

	for i := range total {
		input := &gosmc.SMCKeyData{
			Data8:  gosmc.CMDReadIndex,
			Data32: i,
		}

		output, res := gosmc.SMCCall(conn, gosmc.KernelIndexSMC, input)
		if res != gosmc.IOReturnSuccess {
			continue
		}

		// Decode the uint32 key code as 4-char big-endian ASCII.
		var b [4]byte

		binary.BigEndian.PutUint32(b[:], output.Key)
		names = append(names, string(b[:]))
	}

	return names
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
