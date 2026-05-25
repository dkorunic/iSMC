// SPDX-FileCopyrightText: Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package main

import (
	"github.com/dkorunic/iSMC/internal/cmd"
)

// main initializes Cobra.
func main() {
	cmd.Execute()
}
