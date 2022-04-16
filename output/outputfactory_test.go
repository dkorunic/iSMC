// Copyright (C) 2022 Roland Schaer
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

package output

import (
	"reflect"
	"testing"
)

func TestOutputFactory(t *testing.T) {
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

			"Returns JSONOutput for table output type",
			args{outputType: "json"},
			NewJSONOutput(),
		}, {
			"Returns TableOutput for unknown output type",
			args{outputType: ""},
			NewTableOutput(false),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OutputFactory(tt.args.outputType); reflect.TypeOf(got) != reflect.TypeOf(tt.want) {
				t.Errorf("OutputFactory() = %v, want %v", reflect.TypeOf(got), reflect.TypeOf(tt.want))
			}
		})
	}
}
