// SPDX-FileCopyrightText: Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"github.com/dkorunic/iSMC/internal/output"
	"github.com/spf13/cobra"
)

// rootCmd represents temp command.
var tempCmd = &cobra.Command{
	Use:     "temp",
	Aliases: []string{"temperature", "tmp"},
	Short:   "Display temperature sensors",
	Run: func(_ *cobra.Command, args []string) {
		output.Factory(OutputFlag).Temperature()
	},
}

// init registers the temp subcommand with the root command.
func init() {
	rootCmd.AddCommand(tempCmd)
}
