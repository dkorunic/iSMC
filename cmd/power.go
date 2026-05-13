// Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"github.com/dkorunic/iSMC/output"
	"github.com/spf13/cobra"
)

// rootCmd represents power command.
var powerCmd = &cobra.Command{
	Use:     "power",
	Aliases: []string{"pow"},
	Short:   "Display power sensors",
	Run: func(_ *cobra.Command, args []string) {
		output.Factory(OutputFlag).Power()
	},
}

// init registers the power subcommand with the root command.
func init() {
	rootCmd.AddCommand(powerCmd)
}
