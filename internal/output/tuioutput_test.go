// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"bytes"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func sensor(key, value string) map[string]any {
	return map[string]any{"key": key, "value": value, "type": "flt"}
}

// A non-terminal writer gets static tables: natural row order, numeric values
// right-aligned and text left-aligned, empty categories skipped, no ANSI escapes.
func TestTableOutput_Static(t *testing.T) {
	GetAll = func() map[string]any {
		return map[string]any{
			"Fans":    map[string]any{"Fan 1": sensor("F0Ac", "1200 rpm"), "Fan Mode": sensor("F0Md", "auto")},
			"Battery": map[string]any{},
			"Temperature": map[string]any{
				"CPU Core 10": sensor("Tp0c", "45.5 °C"),
				"CPU Core 2":  sensor("Tp01", "112.25 °C"),
			},
		}
	}

	var out bytes.Buffer

	TableOutput{writer: &out}.All()

	// Temperature precedes Fans per categoryOrder, though Fans sorts first.
	want := `               Temperature
╭─────────────┬──────┬───────────┬──────╮
│ Description │ Key  │ Value     │ Type │
├─────────────┼──────┼───────────┼──────┤
│ CPU Core 2  │ Tp01 │ 112.25 °C │ flt  │
│ CPU Core 10 │ Tp0c │   45.5 °C │ flt  │
╰─────────────┴──────┴───────────┴──────╯

                  Fans
╭─────────────┬──────┬──────────┬──────╮
│ Description │ Key  │ Value    │ Type │
├─────────────┼──────┼──────────┼──────┤
│ Fan 1       │ F0Ac │ 1200 rpm │ flt  │
│ Fan Mode    │ F0Md │ auto     │ flt  │
╰─────────────┴──────┴──────────┴──────╯

`
	assert.Equal(t, want, out.String())
}

func TestTableOutput_Empty(t *testing.T) {
	GetFans = func() map[string]any { return map[string]any{} }

	var out bytes.Buffer

	TableOutput{writer: &out}.Fans()
	assert.Empty(t, out.String())
}

// press sends one key to m.
func press(m tea.Model, code rune) tea.Model {
	m, _ = m.Update(tea.KeyPressMsg{Code: code})

	return m
}

// sized returns a model for secs in a width×height window, checking that its
// view fills the height exactly and never overflows the width.
func sized(t *testing.T, secs []section, width, height int) tea.Model {
	t.Helper()

	m, _ := tea.Model(newTUIModel(secs)).Update(tea.WindowSizeMsg{Width: width, Height: height})
	lines := strings.Split(m.View().Content, "\n")

	assert.Len(t, lines, height, "view must fill the window height exactly")

	for _, l := range lines {
		assert.LessOrEqual(t, lipgloss.Width(l), width, "line overflows the window: %q", l)
	}

	return m
}

// nineRows is a category of nine rows; with the test's headers one row group
// is 31 columns wide, so an 80-column window fits two groups side by side.
func nineRows() []section {
	rows := map[string]any{}
	for _, k := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9"} {
		rows[k] = sensor(k, "1")
	}

	secs := appendSection(nil, "One", rows)

	return appendSection(secs, "Two", map[string]any{"b": sensor("b", "2")})
}

// Rows flow into as many columns as the width fits, but no more than needed.
func TestTUIModel_Columns(t *testing.T) {
	for _, tc := range []struct {
		width int
		want  string
	}{
		{40, "1-4/9"},  // one column of four rows
		{80, "1-8/9"},  // two columns
		{200, "1-9/9"}, // room for six, three hold every row
	} {
		assert.Contains(t, sized(t, nineRows(), tc.width, 10).View().Content, tc.want, "width %d", tc.width)
	}
}

// Scrolling is clamped to the last page, paging moves a whole page, and each
// category keeps its own offset across tab switches.
func TestTUIModel_Scroll(t *testing.T) {
	m := sized(t, nineRows(), 40, 10)

	m = press(m, tea.KeyPgDown)
	assert.Contains(t, m.View().Content, "5-8/9")
	m = press(m, tea.KeyEnd)
	assert.Contains(t, m.View().Content, "6-9/9")
	m = press(m, tea.KeyDown)
	assert.Contains(t, m.View().Content, "6-9/9", "scroll stops at the last page")
	m = press(m, tea.KeyUp)
	assert.Contains(t, m.View().Content, "5-8/9")

	m = press(m, tea.KeyTab)
	assert.Contains(t, m.View().Content, "1-1/1")
	m = press(m, tea.KeyTab)
	assert.Contains(t, m.View().Content, "5-8/9", "offset is kept per category")

	m = press(m, tea.KeyHome)
	assert.Contains(t, m.View().Content, "1-4/9")

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	assert.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

// Widening the window after scrolling to the end clamps the stored offset.
func TestTUIModel_ResizeClamps(t *testing.T) {
	m := press(sized(t, nineRows(), 40, 10), tea.KeyEnd)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 10})
	assert.Contains(t, m.View().Content, "1-9/9")
}

// All shows categories in categoryOrder, with unlisted ones (Battery) last.
func TestTableOutput_CategoryOrder(t *testing.T) {
	GetAll = func() map[string]any {
		all := map[string]any{}
		for _, c := range []string{"Battery", "Current", "Fans", "Power", "Temperature", "Voltage"} {
			all[c] = map[string]any{"s": sensor("k", "1")}
		}

		return all
	}

	var out bytes.Buffer

	TableOutput{writer: &out}.All()

	var titles []string

	for line := range strings.Lines(out.String()) {
		if s := strings.TrimSpace(line); s != "" && !strings.ContainsAny(s, "│╭╰├") {
			titles = append(titles, s)
		}
	}

	assert.Equal(t, []string{"Temperature", "Power", "Fans", "Voltage", "Current", "Battery"}, titles)
}
