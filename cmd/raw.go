// Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"fmt"
	"strings"

	"github.com/dkorunic/iSMC/smc"
	"github.com/spf13/cobra"
)

// Hex alphabet indexed by nibble; avoids per-byte fmt.Sprintf.
const hexDigits = "0123456789abcdef"

// formatBytesHex renders size bytes from b as space-separated lowercase
// two-digit hex pairs (e.g. "01 0a ff").
func formatBytesHex(b [32]byte, size uint32) string {
	if size == 0 {
		return ""
	}

	if size > 32 {
		size = 32
	}

	var sb strings.Builder

	sb.Grow(int(size)*3 - 1)

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
