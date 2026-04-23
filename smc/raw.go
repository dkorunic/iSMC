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
	"fmt"
	"os"

	"github.com/dkorunic/iSMC/gosmc"
)

const (
	keyCount = "#KEY"

	// maxKeys is the upper bound on the number of SMC keys to enumerate.
	// Current Apple Silicon Macs report ~1800 keys; this guards against a corrupt/spoofed #KEY value.
	maxKeys = 4096
)

// RawKey holds raw SMC key data for reporting.
type RawKey struct {
	Key      string
	DataType string
	DataSize uint32
	Bytes    gosmc.SMCBytes
}

// GetRaw returns all SMC keys with their raw byte values
func GetRaw() []RawKey {
	conn, err := openSMC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return nil
	}
	defer gosmc.SMCClose(conn)

	// Read total key count from the special #KEY entry.
	countVal, res := gosmc.SMCReadKey(conn, keyCount)
	if res != gosmc.IOReturnSuccess || countVal.DataSize == 0 {
		return nil
	}

	total := min(smcBytesToUint32(countVal.Bytes, countVal.DataSize), maxKeys)
	keys := make([]RawKey, 0, total)

	for i := range total {
		input := &gosmc.SMCKeyData{
			Data8:  gosmc.CMDReadIndex,
			Data32: i,
		}

		output, res := gosmc.SMCCall(conn, gosmc.KernelIndexSMC, input)
		if res != gosmc.IOReturnSuccess {
			continue
		}

		// Convert the uint32 key code to its 4-character ASCII name (big-endian).
		k := output.Key
		keyStr := string([]byte{
			byte(k >> 24),
			byte(k >> 16),
			byte(k >> 8),
			byte(k),
		})

		val, res := gosmc.SMCReadKey(conn, keyStr)
		if res != gosmc.IOReturnSuccess {
			continue
		}

		keys = append(keys, RawKey{
			Key:      keyStr,
			DataType: smcTypeToString(val.DataType),
			DataSize: val.DataSize,
			Bytes:    val.Bytes,
		})
	}

	return keys
}
