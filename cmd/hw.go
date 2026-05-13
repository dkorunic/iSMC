// Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"github.com/dkorunic/iSMC/output"
	"github.com/spf13/cobra"
)

// hwCmd represents the hw command.
var hwCmd = &cobra.Command{
	Use:     "hw",
	Aliases: []string{"hardware", "info"},
	Short:   "Display hardware information",
	Run: func(_ *cobra.Command, args []string) {
		output.Factory(OutputFlag).Hardware()
	},
}

// init registers the hw subcommand with the root command.
func init() {
	rootCmd.AddCommand(hwCmd)
}
