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

// rawCmd dumps every SMC key with its raw type and bytes in the same format used by report files.
var rawCmd = &cobra.Command{
	Use:   "raw",
	Short: "Display all raw SMC keys and their byte values",
	Run: func(_ *cobra.Command, _ []string) {
		for _, k := range smc.GetRaw() {
			byteStrs := make([]string, k.DataSize)
			for i := range k.DataSize {
				byteStrs[i] = fmt.Sprintf("%02x", k.Bytes[i])
			}

			decoded := smc.DecodeValue(k.DataType, k.Bytes, k.DataSize)
			if decoded != "" {
				fmt.Printf("  %s  [%-4s]  %s (bytes %s)\n", k.Key, k.DataType, decoded, strings.Join(byteStrs, " "))
			} else {
				fmt.Printf("  %s  [%-4s]  (bytes %s)\n", k.Key, k.DataType, strings.Join(byteStrs, " "))
			}
		}
	},
}

// init registers the raw subcommand with the root command.
func init() {
	rootCmd.AddCommand(rawCmd)
}
