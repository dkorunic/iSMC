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

package cmd

import (
	"fmt"
	"strings"

	"github.com/dkorunic/iSMC/smc"
	"github.com/spf13/cobra"
)

// hexDigits is the lowercase hex alphabet, indexed directly by nibble value.
// Used by formatBytesHex to avoid a per-byte fmt.Sprintf call.
const hexDigits = "0123456789abcdef"

// formatBytesHex renders size bytes from b as space-separated lowercase
// two-digit hex pairs (e.g. "01 0a ff"), using a single pre-sized
// strings.Builder instead of one fmt.Sprintf per byte. For a full SMC key
// enumeration (~1800 keys × up to 32 bytes) this avoids tens of thousands of
// allocations compared to the previous []string + strings.Join approach.
func formatBytesHex(b [32]byte, size uint32) string {
	if size == 0 {
		return ""
	}

	if size > 32 {
		size = 32
	}

	var sb strings.Builder

	sb.Grow(int(size)*3 - 1) // "xx" per byte + " " separator between bytes

	for i := uint32(0); i < size; i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}

		sb.WriteByte(hexDigits[b[i]>>4])
		sb.WriteByte(hexDigits[b[i]&0x0f])
	}

	return sb.String()
}

// rawCmd dumps every SMC key with its raw type and bytes in the same format used by report files.
var rawCmd = &cobra.Command{
	Use:   "raw",
	Short: "Display all raw SMC keys and their byte values",
	Run: func(_ *cobra.Command, _ []string) {
		for _, k := range smc.GetRaw() {
			byteStr := formatBytesHex([32]byte(k.Bytes), k.DataSize)

			decoded := smc.DecodeValue(k.DataType, k.Bytes, k.DataSize)
			if decoded != "" {
				fmt.Printf("  %s  [%-4s]  %s (bytes %s)\n", k.Key, k.DataType, decoded, byteStr)
			} else {
				fmt.Printf("  %s  [%-4s]  (bytes %s)\n", k.Key, k.DataType, byteStr)
			}
		}
	},
}

// init registers the raw subcommand with the root command.
func init() {
	rootCmd.AddCommand(rawCmd)
}
