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

//go:build darwin

package output

import (
	"fmt"
	"reflect"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
)

func Test_deepCopy(t *testing.T) {
	type args struct {
		dest map[string]interface{}
		src  map[string]interface{}
	}
	tests := []struct {
		name     string
		args     args
		expected string
	}{
		{
			"Verify dest",
			args{
				dest: map[string]interface{}{
					"key-1": "value-1",
				},
				src: map[string]interface{}{
					"key-2": map[string]interface{}{
						"key-2-1": "value-2-1",
					},
					"key-3": map[string]interface{}{
						"key-3-1": map[string]interface{}{
							"key-3-1-1": "value-3-1-1",
						},
					},
				},
			},
			`{"key-1":"value-1","key-2":{"key-2-1":"value-2-1"},"key-3":{"key-3-1":{"key-3-1-1":"value-3-1-1"}}}`,
		},
		{
			"Verify empty dest",
			args{
				dest: map[string]interface{}{},
				src: map[string]interface{}{
					"key-1": "value-1",
					"key-2": map[string]interface{}{
						"key-2-1": "value-2-1",
					},
				},
			},
			`{"key-1":"value-1","key-2":{"key-2-1":"value-2-1"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deepCopy(tt.args.dest, tt.args.src)

			actual := toJson(tt.args.dest)
			assert.JSONEq(t, actual, tt.expected)
		})
	}
}

func Test_merge(t *testing.T) {
	type args struct {
		a map[string]interface{}
		b map[string]interface{}
	}
	tests := []struct {
		name     string
		args     args
		expected map[string]interface{}
	}{
		{
			"Verify merge",
			args{
				map[string]interface{}{
					"key-1": "value-1-a",
					"key-3": map[string]interface{}{
						"key-3-2": map[string]interface{}{
							"key-3-2-1": "value-3-2-1-a",
						},
					},
				},
				map[string]interface{}{
					"key-1": "value-2-b",
					"key-2": "value-2-b",
					"key-3": map[string]interface{}{
						"key-3-1": "value-3-1-b",
						"key-3-2": map[string]interface{}{
							"key-3-2-1": "value-3-2-1-b",
						},
					},
				},
			},
			map[string]interface{}{
				"key-1": "value-2-b",
				"key-2": "value-2-b",
				"key-3": map[string]interface{}{
					"key-3-1": "value-3-1-b",
					"key-3-2": map[string]interface{}{
						"key-3-2-1": "value-3-2-1-b",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := merge(tt.args.a, tt.args.b)
			assert.True(
				t,
				reflect.DeepEqual(actual, tt.expected),
				fmt.Sprintf("Expected %v but was %v", tt.expected, actual),
			)
		})
	}
}

func toJson(src map[string]interface{}) string {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	jsonStr, _ := json.Marshal(src)
	return string(jsonStr)
}

func getMapForSensor(sensorName string) map[string]interface{} {
	return map[string]interface{}{
		sensorName: map[string]interface{}{
			"key":   "key",
			"value": "value",
			"type":  "type",
		},
	}
}
