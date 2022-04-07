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

	"github.com/spf13/cobra"
)

// rootCmd represents all commands.
var allCmd = &cobra.Command{
	Use:     "all",
	Aliases: []string{"everything", "*"},
	Short:   "Display all known sensors, fans and battery status",
	Run: func(cmd *cobra.Command, args []string) {
		tempCmd.Run(cmd, args)
		fmt.Println("")
		fansCmd.Run(cmd, args)
		fmt.Println("")
		battCmd.Run(cmd, args)
		fmt.Println("")
		powerCmd.Run(cmd, args)
		fmt.Println("")
		voltCmd.Run(cmd, args)
		fmt.Println("")
		currCmd.Run(cmd, args)
	},
}
