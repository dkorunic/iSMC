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

// SensorStat is SMC key to description mapping
type SensorStat struct {
	Key  string // SMC key name
	Desc string // SMC key description
}

//go:generate ./gen-sensors.sh sensors.go

var sensorOutputHeader = table.Row{"Description", "Key", "Value", "Type"} // row header definition

// printGeneric prints a table of SMC keys, decription and decoded values with units
func printGeneric(desc, unit string, smcSlice []SensorStat) {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		log.Errorf("unable to open Apple SMC; return code %v\n", res)
		os.Exit(1)
	}
	defer gosmc.SMCClose(c)

	fmt.Println(desc)
	t := table.NewWriter()
	defer func() {
		t.Render()
		fmt.Printf("\n")
	}()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleColoredBright)
	t.AppendHeader(sensorOutputHeader)

	sort.Slice(smcSlice, func(i, j int) bool { return smcSlice[i].Key < smcSlice[j].Key })

	for _, v := range smcSlice {
		key := v.Key
		desc := v.Desc

		if !strings.Contains(key, KeyWildcard) {
			getKeyAndPrint(c, key, t, desc, unit)
			continue
		}

		for i := 0; i < 10; i++ {
			tmpKey := strings.Replace(key, KeyWildcard, strconv.Itoa(i), 1)
			tmpDesc := strings.Replace(desc, KeyWildcard, strconv.Itoa(i+1), 1)
			getKeyAndPrint(c, tmpKey, t, tmpDesc, unit)
		}

	}
}

func getKeyAndPrint(c uint, key string, t table.Writer, desc string, unit string) {
	f, ty, err := getKeyFloat32(c, key)
	if err != nil {
		return
	}

	// TODO: Do better task at ignoring and reporting invalid/missing values
	if f != 0.0 && f != -127.0 && f != -0.0 {
		if f < 0.0 {
			f = -f
		}

		t.AppendRow([]interface{}{
			desc,
			key, fmt.Sprintf("%6.1f %s", f, unit), ty,
		})
	}
}

// PrintTemp prints detected temperature sensor results
func PrintTemp() {
	printGeneric("Temperature:", "°C", AppleTemp)
}

// PrintTemp prints detected power sensor results
func PrintPower() {
	printGeneric("Power:", "W", ApplePower)
}

// PrintTemp prints detected voltage sensor results
func PrintVoltage() {
	printGeneric("Voltage:", "V", AppleVoltage)
}

// PrintTemp prints detected current sensor results
func PrintCurrent() {
	printGeneric("Current:", "A", AppleCurrent)
}

// PrintTemp prints detected fan results
func PrintFans() {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		log.Errorf("unable to open Apple SMC; return code %v\n", res)
		os.Exit(1)
	}
	defer gosmc.SMCClose(c)

	fmt.Println("Fans:")
	t := table.NewWriter()
	defer func() {
		t.Render()
		fmt.Printf("\n")
	}()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleColoredBright)
	t.AppendHeader(sensorOutputHeader)

	n, ty, _ := getKeyUint32(c, FanNum) // Get number of fans
	t.AppendRow([]interface{}{
		fmt.Sprintf("%v", "Fan Count"), FanNum, fmt.Sprintf("%8v", n),
		ty,
	})

	for i := uint32(0); i < n; i++ {
		for _, v := range AppleFans {
			key := fmt.Sprintf(v.Key, i)
			desc := fmt.Sprintf(v.Desc, i+1)

			f, ty, err := getKeyFloat32(c, key)
			if err != nil {
				log.Errorf("unable to get SMC key %v: %v", key, err)

				return
			}

			if f != 0.0 && f != -127.0 && f != -0.0 {
				t.AppendRow([]interface{}{desc, key, fmt.Sprintf("%4.0f rpm", f), ty})
			}
		}
	}
}

// PrintTemp prints detected battery results
// TODO: Needs battery info decoding (hex_ SMC key type)
func PrintBatt() {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		log.Errorf("unable to open Apple SMC; return code %v\n", res)
		os.Exit(1)
	}
	defer gosmc.SMCClose(c)

	fmt.Println("Battery:")
	t := table.NewWriter()
	defer func() {
		t.Render()
		fmt.Printf("\n")
	}()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleColoredBright)
	t.AppendHeader(sensorOutputHeader)

	n, ty1, _ := getKeyUint32(c, BattNum) // Get number of batteries
	i, ty2, _ := getKeyUint32(c, BattInf) // Get battery info (needs bit decoding)
	b, ty3, _ := getKeyBool(c, BattPwr)   // Get AC status

	t.AppendRow([]interface{}{
		fmt.Sprintf("%v", "Battery Count"), BattNum,
		fmt.Sprintf("%8v", n), ty1,
	})
	t.AppendRow([]interface{}{
		fmt.Sprintf("%v", "Battery Info"), BattInf,
		fmt.Sprintf("%8v", i), ty2,
	}) // TODO: Needs decoding!
	t.AppendRow([]interface{}{
		fmt.Sprintf("%v", "Battery Powered"), BattPwr,
		fmt.Sprintf("%8v", b), ty3,
	})
}

// unknown/scan
