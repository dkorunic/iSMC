// Copyright (C) 2026  Dinko Korunic
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
