// SPDX-FileCopyrightText: Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"github.com/dkorunic/iSMC/internal/output"
	"github.com/spf13/cobra"
)

// rootCmd represents curr command.
var currCmd = &cobra.Command{
	Use:     "curr",
	Aliases: []string{"current", "cur"},
	Short:   "Display current sensors",
	Run: func(_ *cobra.Command, _ []string) {
		output.Factory(OutputFlag).Current()
	},
}

// init registers the curr subcommand with the root command.
func init() {
	rootCmd.AddCommand(currCmd)
}
