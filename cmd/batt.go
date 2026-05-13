// Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"github.com/dkorunic/iSMC/output"
	"github.com/spf13/cobra"
)

// rootCmd represents batt command.
var battCmd = &cobra.Command{
	Use:     "batt",
	Aliases: []string{"battery", "bat"},
	Short:   "Display battery status",
	Run: func(_ *cobra.Command, args []string) {
		output.Factory(OutputFlag).Battery()
	},
}

// init registers the batt subcommand with the root command.
func init() {
	rootCmd.AddCommand(battCmd)
}
