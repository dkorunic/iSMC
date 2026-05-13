// Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var (
	GitTag    = ""
	GitCommit = ""
	GitDirty  = ""
	BuildTime = ""
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of iSMC",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("iSMC %v %v%v, built on %v, with %v\n", GitTag, GitCommit, GitDirty,
			BuildTime, runtime.Version())
	},
}

func init() {
	GitTag = strings.TrimSpace(GitTag)
	GitCommit = strings.TrimSpace(GitCommit)
	GitDirty = strings.TrimSpace(GitDirty)
	BuildTime = strings.TrimSpace(BuildTime)

	rootCmd.AddCommand(versionCmd)
}
