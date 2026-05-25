// SPDX-FileCopyrightText: Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"github.com/dkorunic/iSMC/internal/output"
	"github.com/spf13/cobra"
)

// rootCmd represents fans command.
var fansCmd = &cobra.Command{
	Use:     "fans",
	Aliases: []string{"fan"},
	Short:   "Display fans status",
	Run: func(_ *cobra.Command, args []string) {
		output.Factory(OutputFlag).Fans()
	},
}

// init registers the fans subcommand with the root command.
func init() {
	rootCmd.AddCommand(fansCmd)
}
