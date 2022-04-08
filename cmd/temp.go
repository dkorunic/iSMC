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
	"github.com/dkorunic/iSMC/output"
	"github.com/spf13/cobra"
)

// rootCmd represents temp command.
var tempCmd = &cobra.Command{
	Use:     "temp",
	Aliases: []string{"temperature", "tmp"},
	Short:   "Display temperature sensors",
	Run: func(cmd *cobra.Command, args []string) {
		output.OutputFactory(OutputFlag).Temperature()
	},
}

func init() {
	rootCmd.AddCommand(tempCmd)
}
