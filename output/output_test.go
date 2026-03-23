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
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_deepCopy(t *testing.T) {
	type args struct {
		dest map[string]any
		src  map[string]any
	}

	tests := []struct {
		name     string
		args     args
		expected string
	}{
		{
			"Verify dest",
			args{
				dest: map[string]any{
					"key-1": "value-1",
				},
				src: map[string]any{
					"key-2": map[string]any{
						"key-2-1": "value-2-1",
					},
					"key-3": map[string]any{
						"key-3-1": map[string]any{
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
				dest: map[string]any{},
				src: map[string]any{
					"key-1": "value-1",
					"key-2": map[string]any{
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

			actual := toJSON(tt.args.dest)
			assert.JSONEq(t, tt.expected, actual)
		})
	}
}

func Test_merge(t *testing.T) {
	type args struct {
		a map[string]any
		b map[string]any
	}

	tests := []struct {
		name     string
		args     args
		expected map[string]any
	}{
		{
			"Verify merge",
			args{
				map[string]any{
					"key-1": "value-1-a",
					"key-3": map[string]any{
						"key-3-2": map[string]any{
							"key-3-2-1": "value-3-2-1-a",
						},
					},
				},
				map[string]any{
					"key-1": "value-2-b",
					"key-2": "value-2-b",
					"key-3": map[string]any{
						"key-3-1": "value-3-1-b",
						"key-3-2": map[string]any{
							"key-3-2-1": "value-3-2-1-b",
						},
					},
				},
			},
			map[string]any{
				"key-1": "value-2-b",
				"key-2": "value-2-b",
				"key-3": map[string]any{
					"key-3-1": "value-3-1-b",
					"key-3-2": map[string]any{
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
				"Expected %v but was %v",
				tt.expected, actual,
			)
		})
	}
}

// Test_deepCopy_isolation verifies that deepCopy produces a true deep clone: mutating
// a nested map in dest must not affect the original src (TC-14). A shallow copy
// (for k, v := range src { dest[k] = v }) would fail this because both dest and src
// would share the same nested map pointer.
func Test_deepCopy_isolation(t *testing.T) {
	src := map[string]any{
		"sensor": map[string]any{
			"key":   "TC0H",
			"value": "25.0 °C",
		},
	}

	dest := make(map[string]any)
	deepCopy(dest, src)

	// Mutate the nested map in dest
	if nested, ok := dest["sensor"].(map[string]any); ok {
		nested["value"] = "999.0 °C"
	}

	// src must be unaffected — a shallow copy would expose the same nested map
	if nestedSrc, ok := src["sensor"].(map[string]any); ok {
		assert.Equal(t, "25.0 °C", nestedSrc["value"],
			"deepCopy must produce a deep clone; mutating dest must not affect src")
	}
}

// Test_merge_bOverridesA verifies that merge gives precedence to b when both maps
// contain the same flat key (TC-13 supporting test).
func Test_merge_bOverridesA(t *testing.T) {
	a := map[string]any{"CPU Temp": "25.0 °C", "GPU Temp": "40.0 °C"}
	b := map[string]any{"CPU Temp": "30.0 °C"} // b overrides CPU Temp

	result := merge(a, b)

	assert.Equal(t, "30.0 °C", result["CPU Temp"], "merge must give b precedence over a for flat keys")
	assert.Equal(t, "40.0 °C", result["GPU Temp"], "merge must preserve a keys absent from b")
}

// Test_merge_nestedBOverridesA verifies TC-15: that merge recurses into nested maps
// rather than replacing a's whole sub-map with b's. If merge did NOT recurse,
// only b's Temperature entries would appear and a's would be silently dropped.
func Test_merge_nestedBOverridesA(t *testing.T) {
	a := map[string]any{
		"Temperature": map[string]any{
			"CPU Temp": map[string]any{"key": "TC0H", "value": "25.0 °C", "type": "sp78"},
			"GPU Temp": map[string]any{"key": "TG0H", "value": "40.0 °C", "type": "sp78"},
		},
	}
	b := map[string]any{
		"Temperature": map[string]any{
			// HID override for CPU Temp; GPU Temp is absent from b
			"CPU Temp": map[string]any{"key": "TC0H", "value": "27.0 °C", "type": "hid"},
		},
	}

	result := merge(a, b)

	temps, ok := result["Temperature"].(map[string]any)
	assert.True(t, ok, "Temperature category must survive the merge")

	cpuTemp, ok := temps["CPU Temp"].(map[string]any)
	assert.True(t, ok, "CPU Temp must be present after merge")
	assert.Equal(t, "27.0 °C", cpuTemp["value"], "b's CPU Temp value must override a's")

	_, gpuOk := temps["GPU Temp"]
	assert.True(t, gpuOk, "GPU Temp from a must NOT be lost when b only partially covers Temperature")
}

// toJSON marshals src to a JSON string for use in test assertions.
func toJSON(src map[string]any) string {
	jsonStr, _ := json.Marshal(src)

	return string(jsonStr)
}

// getMapForSensor returns a minimal sensor map keyed by sensorName for use in table-driven tests.
func getMapForSensor(sensorName string) map[string]any {
	return map[string]any{
		sensorName: map[string]any{
			"key":   "key",
			"value": "value",
			"type":  "type",
		},
	}
}
