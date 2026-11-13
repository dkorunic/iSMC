// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

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

	m, _ := tea.Model(newTUIModel(secs, nil, 0)).Update(tea.WindowSizeMsg{Width: width, Height: height})
	lines := strings.Split(m.View().Content, "\n")

	assert.Len(t, lines, height-1, "view must fill the window but its last line")

	for _, l := range lines {
		assert.LessOrEqual(t, lipgloss.Width(l), width, "line overflows the window: %q", l)
	}

	return m
}

// nineRows is a category of nine rows; with the test's headers one row group
// is 31 columns wide, so an 80-column window fits two groups side by side, and
// an 11-line window holds four rows per group.
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
		assert.Contains(t, sized(t, nineRows(), tc.width, 11).View().Content, tc.want, "width %d", tc.width)
	}
}

// Scrolling is clamped to the last page, paging moves a whole page, and each
// category keeps its own offset across tab switches.
func TestTUIModel_Scroll(t *testing.T) {
	m := sized(t, nineRows(), 40, 11)

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

// Widening the window after scrolling to the end clamps the stored offset,
// and the first key press scrolls from the clamped position.
func TestTUIModel_ResizeClamps(t *testing.T) {
	m := press(sized(t, nineRows(), 40, 11), tea.KeyEnd)
	assert.Contains(t, m.View().Content, "6-9/9")

	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 11})
	assert.Contains(t, m.View().Content, "2-9/9")

	m = press(m, tea.KeyUp)
	assert.Contains(t, m.View().Content, "1-8/9", "first up after a resize must move")

	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 11})
	assert.Contains(t, m.View().Content, "1-9/9")
}

func TestIsNumeric(t *testing.T) {
	for v, want := range map[string]bool{
		"45.5 °C": true, "-1.2 A": true, "20": true, " 900 rpm": true,
		"M1 Ultra": false, "auto": false, "": false, " ": false, "-": false,
	} {
		assert.Equal(t, want, isNumeric(v), "%q", v)
	}
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

// Coloured values keep every line of the scrollable view the same width:
// column sizing must ignore the ANSI codes colorize adds.
func TestTUIModel_ColoredAlignment(t *testing.T) {
	secs := appendSection(nil, "Temperature", map[string]any{
		"CPU Core 1": sensor("Tp01", "112.25 °C"),
		"CPU Core 2": sensor("Tp05", "45.5 °C"),
		"GPU 1":      sensor("Tg05", "88.0 °C"),
	})
	colorize(secs)

	assert.Contains(t, secs[0].rows[0][2], "\x1b[", "112.25 °C must be coloured")
	assert.NotContains(t, secs[0].rows[1][2], "\x1b[", "45.5 °C must stay plain")

	m, _ := newTUIModel(secs, nil, 0).Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	lines := strings.Split(m.View().Content, "\n")
	table := lines[1 : len(lines)-1] // between the tab bar and the help line

	for _, l := range table {
		assert.Equal(t, lipgloss.Width(table[0]), lipgloss.Width(l), "misaligned line %q", l)
	}
}

// A refresh shows the new reading and keeps the active tab, its scroll offset
// and its column widths (which only grow, so values never make columns jitter).
func TestTUIModel_Refresh(t *testing.T) {
	read := func(cpu string) []section {
		secs := appendSection(nil, "Temperature", map[string]any{"CPU": sensor("Tp01", cpu), "GPU": sensor("Tg05", "40.0 °C")})

		return appendSection(secs, "Fans", map[string]any{"Fan 1": sensor("F0Ac", "1200 rpm")})
	}

	m := newTUIModel(read("100.25 °C"), func() []section { return read("50.0 °C") }, time.Second)
	m.active, m.offsets[1] = 1, 0
	m.offsets[0] = 1
	wide := m.widths[0][2]

	assert.NotNil(t, m.Init(), "a refresh interval must schedule a tick")

	next, cmd := m.Update(refreshMsg{secs: m.load(), gen: m.gen})
	r, ok := next.(tuiModel)
	assert.True(t, ok)

	assert.NotNil(t, cmd, "each refresh schedules the next")
	assert.Equal(t, 1, r.active, "active tab kept")
	assert.Equal(t, 1, r.offsets[0], "scroll offset kept")
	assert.Equal(t, wide, r.widths[0][2], "value column must not shrink")
	assert.Contains(t, r.secs[0].rows[0][2], "50.0 °C", "new reading shown")

	assert.Nil(t, newTUIModel(read("1 °C"), nil, 0).Init(), "no refresh without an interval")
	assert.Nil(t, newTUIModel(read("1 °C"), m.load, NoRefresh).Init(), "--watch 0 disables refresh")
}

func TestTUIModel_PauseToggle(t *testing.T) {
	read := func(v string) []section {
		return appendSection(nil, "Temperature", map[string]any{"CPU": sensor("Tp01", v)})
	}

	update := func(m tuiModel, msg tea.Msg) (tuiModel, tea.Cmd) {
		next, cmd := m.Update(msg)
		r, ok := next.(tuiModel)
		assert.True(t, ok)

		return r, cmd
	}

	p := tea.KeyPressMsg{Code: 'p', Text: "p"}
	m := newTUIModel(read("40.0 °C"), func() []section { return read("50.0 °C") }, time.Second)
	m.width, m.height = 200, 20
	inflight := refreshMsg{secs: m.load(), gen: m.gen}

	m, cmd := update(m, p)
	assert.True(t, m.paused)
	assert.Nil(t, cmd, "pausing schedules nothing")
	assert.Contains(t, m.View().Content, "p resume")

	m, cmd = update(m, inflight)
	assert.Nil(t, cmd, "a read in flight at pause time is dropped")
	assert.Contains(t, m.secs[0].rows[0][2], "40.0 °C", "paused view keeps its values")

	m, cmd = update(m, p)
	assert.False(t, m.paused)
	assert.NotNil(t, cmd, "resuming schedules a tick")

	_, cmd = update(m, inflight)
	assert.Nil(t, cmd, "a pre-pause read must not start a second tick chain")

	off, cmd := update(newTUIModel(read("40.0 °C"), nil, 0), p)
	assert.False(t, off.paused, "p is a no-op without refresh")
	assert.Nil(t, cmd)
}
