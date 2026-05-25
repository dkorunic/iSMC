// SPDX-FileCopyrightText: Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"github.com/dkorunic/iSMC/internal/output"
	"github.com/spf13/cobra"
)

// rootCmd represents volt command.
var voltCmd = &cobra.Command{
	Use:     "volt",
	Aliases: []string{"voltage", "vol"},
	Short:   "Display voltage sensors",
	Run: func(_ *cobra.Command, args []string) {
		output.Factory(OutputFlag).Voltage()
	},
}

// init registers the volt subcommand with the root command.
func init() {
	rootCmd.AddCommand(voltCmd)
}
