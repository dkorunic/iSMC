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

package smc

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jedib0t/go-pretty/table"
	"github.com/panotza/gosmc"
	log "github.com/sirupsen/logrus"
)

const (
	AppleSMC    = "AppleSMC"
	FanNum      = "FNum"
	BattNum     = "BNum"
	BattPwr     = "BATP"
	BattInf     = "BSIn"
	KeyWildcard = "%"
)

// SensorStat is SMC key to description mapping.
type SensorStat struct {
	Key  string // SMC key name
	Desc string // SMC key description
}

//go:generate ./gen-sensors.sh sensors.go

var sensorOutputHeader = table.Row{"Description", "Key", "Value", "Type"} // row header definition

// printGeneric prints a table of SMC keys, description and decoded values with units.
func printGeneric(t table.Writer, desc, unit string, smcSlice []SensorStat) {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		log.Errorf("unable to open Apple SMC; return code %v\n", res)
		os.Exit(1)
	}
	defer gosmc.SMCClose(c)

	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleColoredBright)
	t.AppendHeader(sensorOutputHeader)

	sort.Slice(smcSlice, func(i, j int) bool { return smcSlice[i].Key < smcSlice[j].Key })

	for _, v := range smcSlice {
		key := v.Key
		desc := v.Desc

		if !strings.Contains(key, KeyWildcard) {
			getKeyAndPrint(t, c, key, desc, unit)

			continue
		}

		for i := 0; i < 10; i++ {
			tmpKey := strings.Replace(key, KeyWildcard, strconv.Itoa(i), 1)
			tmpDesc := strings.Replace(desc, KeyWildcard, strconv.Itoa(i+1), 1)
			getKeyAndPrint(t, c, tmpKey, tmpDesc, unit)
		}
	}
}

// getKeyAndPrint fetches key value for a given SMC key and prints a table entry.
func getKeyAndPrint(t table.Writer, c uint, key string, desc string, unit string) {
	val, smcType, err := getKeyFloat32(c, key)
	if err != nil {
		return
	}

	// TODO: Do better task at ignoring and reporting invalid/missing values
	if val != -127.0 && val != 0.0 {
		if val < 0.0 {
			val = -val
		}

		t.AppendRow([]interface{}{
			desc,
			key,
			fmt.Sprintf("%6.1f %s", val, unit),
			smcType,
		})
	}
}

// PrintTemp prints detected temperature sensor results.
func PrintTemp(t table.Writer) {
	printGeneric(t, "Temperature:", "°C", AppleTemp)
}

// PrintPower prints detected power sensor results.
func PrintPower(t table.Writer) {
	printGeneric(t, "Power:", "W", ApplePower)
}

// PrintVoltage prints detected voltage sensor results.
func PrintVoltage(t table.Writer) {
	printGeneric(t, "Voltage:", "V", AppleVoltage)
}

// PrintCurrent prints detected current sensor results.
func PrintCurrent(t table.Writer) {
	printGeneric(t, "Current:", "A", AppleCurrent)
}

// PrintFans prints detected fan results.
func PrintFans(t table.Writer) {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		log.Errorf("unable to open Apple SMC; return code %v\n", res)
		os.Exit(1)
	}
	defer gosmc.SMCClose(c)

	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleColoredBright)
	t.AppendHeader(sensorOutputHeader)

	val, smcType, _ := getKeyUint32(c, FanNum) // Get number of fans
	t.AppendRow([]interface{}{
		fmt.Sprintf("%v", "Fan Count"),
		FanNum,
		fmt.Sprintf("%8v", val),
		smcType,
	})

	for i := uint32(0); i < val; i++ {
		for _, v := range AppleFans {
			key := fmt.Sprintf(v.Key, i)
			desc := fmt.Sprintf(v.Desc, i+1)

			val, smcType, err := getKeyFloat32(c, key)
			if err != nil {
				log.Errorf("unable to get SMC key %v: %v", key, err)

				return
			}

			if val != -127.0 && val != 0.0 {
				if val < 0.0 {
					val = -val
				}

				t.AppendRow([]interface{}{
					desc,
					key,
					fmt.Sprintf("%4.0f rpm", val),
					smcType,
				})
			}
		}
	}
}

// PrintBatt prints detected battery results.
// TODO: Needs battery info decoding (hex_ SMC key type).
func PrintBatt(t table.Writer) {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		log.Errorf("unable to open Apple SMC; return code %v\n", res)
		os.Exit(1)
	}
	defer gosmc.SMCClose(c)

	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleColoredBright)
	t.AppendHeader(sensorOutputHeader)

	n, smcType1, _ := getKeyUint32(c, BattNum) // Get number of batteries
	i, smcType2, _ := getKeyUint32(c, BattInf) // Get battery info (needs bit decoding)
	b, smcType3, _ := getKeyBool(c, BattPwr)   // Get AC status

	t.AppendRow([]interface{}{
		fmt.Sprintf("%v", "Battery Count"),
		BattNum,
		fmt.Sprintf("%8v", n),
		smcType1,
	})
	t.AppendRow([]interface{}{
		fmt.Sprintf("%v", "Battery Info"),
		BattInf,
		fmt.Sprintf("%8v", i),
		smcType2,
	}) // TODO: Needs decoding!
	t.AppendRow([]interface{}{
		fmt.Sprintf("%v", "Battery Powered"),
		BattPwr,
		fmt.Sprintf("%8v", b),
		smcType3,
	})
}
