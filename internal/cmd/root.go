// SPDX-FileCopyrightText: Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var OutputFlag string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "iSMC",
	Short: "Apple SMC information tool",
	Long: `Apple SMC CLI tool that can decode and display temperature, fans, battery, power, voltage and current
information for various hardware in your Apple Mac hardware.`,
	Run: func(cmd *cobra.Command, args []string) {
		allCmd.Run(cmd, args)
	},
}

// Execute runs the root Cobra command and exits with a non-zero status on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// init registers the persistent --output flag on the root command.
func init() {
	rootCmd.PersistentFlags().StringVarP(&OutputFlag, "output", "o", "table", "Output format (ascii, table, json, influx)")
}
