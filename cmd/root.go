// @license
// Copyright (C) 2019  Dinko Korunic
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// +build darwin

package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

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

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(tempCmd)
	rootCmd.AddCommand(fansCmd)
	rootCmd.AddCommand(battCmd)
	rootCmd.AddCommand(powerCmd)
	rootCmd.AddCommand(voltCmd)
	rootCmd.AddCommand(currCmd)
	rootCmd.AddCommand(allCmd)
}
