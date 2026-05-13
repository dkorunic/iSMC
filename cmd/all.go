// Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"github.com/dkorunic/iSMC/output"
	"github.com/spf13/cobra"
)

// rootCmd represents all commands.
var allCmd = &cobra.Command{
	Use:     "all",
	Aliases: []string{"everything", "*"},
	Short:   "Display all known sensors, fans and battery status",
	Run: func(_ *cobra.Command, args []string) {
		output.Factory(OutputFlag).All()
	},
}

// init registers the all subcommand with the root command.
func init() {
	rootCmd.AddCommand(allCmd)
}
