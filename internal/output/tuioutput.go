// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	activeTab   = lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(accentColor).Padding(0, 1)
	inactiveTab = lipgloss.NewStyle().Foreground(mutedColor).Padding(0, 1)
	helpStyle   = lipgloss.NewStyle().Foreground(mutedColor)
)

// tuiChrome is the number of terminal lines that are not sensor rows: the tab
// bar, the table's top border, header, header rule and bottom border, the help
// line, and one spare line. The view draws inline rather than on the alternate
// screen, so the last frame stays in scrollback after quitting; the spare line
// keeps a full-height frame from scrolling the terminal.
const tuiChrome = 7

// fieldGap separates the fields inside one row group, matching the two spaces
// of cell padding between the columns of the static tables.
const fieldGap = "  "

// tuiModel is the Bubble Tea model of the scrollable view. The active
// category's rows flow newspaper-style into as many side-by-side columns as
// the terminal width fits, so wide terminals show more rows at once; each
// category keeps its own scroll offset.
type tuiModel struct {
	secs    []section
	widths  [][]int // per section: fixed column widths, so scrolling never reflows them
	offsets []int   // per section: index of the first visible row
	active  int
	width   int
	height  int
}

func newTUIModel(secs []section) tuiModel {
	m := tuiModel{
		secs:    secs,
		widths:  make([][]int, len(secs)),
		offsets: make([]int, len(secs)),
		active:  0,
		width:   0,
		height:  0,
	}

	for i, s := range secs {
		m.widths[i] = columnWidths(s.rows)
	}

	return m
}

// columnWidths sizes each column to its widest cell or header.
func columnWidths(rows [][]string) []int {
	w := make([]int, len(tableHeaders))

	for i, h := range tableHeaders {
		w[i] = lipgloss.Width(h)
		for _, r := range rows {
			w[i] = max(w[i], lipgloss.Width(r[i]))
		}
	}

	return w
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		cols, rows := m.layout()
		page := cols * rows

		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			m.active = (m.active + 1) % len(m.secs)
		case "shift+tab", "left", "h":
			m.active = (m.active + len(m.secs) - 1) % len(m.secs)
		case "up", "k":
			m.offsets[m.active] = m.scrolled(-1)
		case "down", "j":
			m.offsets[m.active] = m.scrolled(1)
		case "pgup", "b", "u":
			m.offsets[m.active] = m.scrolled(-page)
		case "pgdown", "f", "d", "space":
			m.offsets[m.active] = m.scrolled(page)
		case "home", "g":
			m.offsets[m.active] = m.scrolled(-len(m.secs[m.active].rows))
		case "end", "G":
			m.offsets[m.active] = m.scrolled(len(m.secs[m.active].rows))
		}
	}

	return m, nil
}

func (m tuiModel) View() tea.View {
	tabs := make([]string, len(m.secs))

	for i, s := range m.secs {
		if i == m.active {
			tabs[i] = activeTab.Render(s.title)
		} else {
			tabs[i] = inactiveTab.Render(s.title)
		}
	}

	s, widths := m.secs[m.active], m.widths[m.active]
	cols, rows := m.layout()
	// A resize can leave a stored offset past the new end; clamp when drawing.
	off := min(m.offsets[m.active], m.maxOffset())

	// One table, one cell per row group, so lipgloss draws every junction.
	header := make([]string, cols)
	data := make([][]string, rows)

	for c := range cols {
		header[c] = joinFields(tableHeaders, widths)
	}

	for i := range data {
		data[i] = make([]string, cols)

		for c := range cols {
			r := make([]string, len(widths)) // blank filler past the last row
			if j := off + c*rows + i; j < len(s.rows) {
				r = s.rows[j]
			}

			data[i][c] = joinFields(r, widths)
		}
	}

	page := newTable(lipgloss.RoundedBorder(), borderStyle, headerStyle).Headers(header...).Rows(data...).String()

	help := fmt.Sprintf("%d-%d/%d · ↑/↓ scroll · pgup/pgdn page · g/G top/bottom",
		off+1, min(off+cols*rows, len(s.rows)), len(s.rows))

	if len(m.secs) > 1 {
		help += " · ←/→ category"
	}

	// Narrow windows truncate the tab bar and help line instead of wrapping.
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(max(m.width, 1)).Render(lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, tabs...),
		page,
		helpStyle.Render(help+" · q quit"),
	)))

	return v
}

// layout returns how many row columns fit side by side and how many rows each
// holds. Columns beyond what the active category's rows need are not used.
func (m tuiModel) layout() (int, int) {
	rows := max(m.height-tuiChrome, 1)

	// Each group is one table cell: its fields, cell padding and one border;
	// the table adds one more border on the right.
	groupWidth := lipgloss.Width(joinFields(tableHeaders, m.widths[m.active])) + cellStyle.GetHorizontalPadding() + 1
	cols := max((m.width-1)/groupWidth, 1)
	cols = min(cols, max((len(m.secs[m.active].rows)+rows-1)/rows, 1))

	return cols, rows
}

// maxOffset is the largest useful scroll offset: the last page shows the last row.
func (m tuiModel) maxOffset() int {
	cols, rows := m.layout()

	return max(len(m.secs[m.active].rows)-cols*rows, 0)
}

// scrolled returns the active category's offset moved by delta, clamped to its range.
func (m tuiModel) scrolled(delta int) int {
	// Clamp the stored offset first: a resize may have left it past the end.
	cur := min(m.offsets[m.active], m.maxOffset())

	return min(max(cur+delta, 0), m.maxOffset())
}

// joinFields pads each field to its column width and joins them into one row
// group, so every group of a page lines up.
func joinFields(fields []string, widths []int) string {
	out := make([]string, len(fields))

	for i, f := range fields {
		out[i] = f + strings.Repeat(" ", max(widths[i]-lipgloss.Width(f), 0))
	}

	return strings.Join(out, fieldGap)
}
