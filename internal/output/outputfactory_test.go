// SPDX-FileCopyrightText: Copyright (C) 2022 Roland Schaer
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"reflect"
	"testing"
)

func TestFactory(t *testing.T) {
	type args struct {
		outputType string
	}

	tests := []struct {
		name string
		args args
		want Output
	}{
		{
			"Returns TableOutput(ASCII) for ascii output type",
			args{outputType: "ascii"},
			NewTableOutput(true),
		}, {
			"Returns TableOutput for table output type",
			args{outputType: "table"},
			NewTableOutput(false),
		}, {
			"Returns JSONOutput for json output type",
			args{outputType: "json"},
			NewJSONOutput(),
		}, {
			// TC-22 guard: influx must return InfluxOutput, not TableOutput
			"Returns InfluxOutput for influx output type",
			args{outputType: "influx"},
			NewInfluxOutput(),
		}, {
			"Returns TableOutput for unknown output type",
			args{outputType: ""},
			NewTableOutput(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Factory(tt.args.outputType); reflect.TypeOf(got) != reflect.TypeOf(tt.want) {
				t.Errorf("Factory() = %v, want %v", reflect.TypeOf(got), reflect.TypeOf(tt.want))
			}
		})
	}
}
