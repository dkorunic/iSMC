// Copyright (C) 2019  Dinko Korunic
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
