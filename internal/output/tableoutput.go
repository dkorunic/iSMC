// SPDX-FileCopyrightText: Copyright (C) 2022 Roland Schaer
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	lgtable "charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/term"
)

var tableHeaders = []string{"Description", "Key", "Value", "Type"}

// categoryOrder is the order All shows categories in (tabs, and sections when
// printed); categories not listed (Battery) follow in natural order.
var categoryOrder = []string{"Temperature", "Power", "Fans", "Voltage", "Current"}

// categoryRank returns name's position in categoryOrder, or len(categoryOrder).
func categoryRank(name string) int {
	if i := slices.Index(categoryOrder, name); i >= 0 {
		return i
	}

	return len(categoryOrder)
}

// Basic ANSI colours so the terminal theme decides the exact shade.
var (
	accentColor = lipgloss.Color("6") // cyan
	mutedColor  = lipgloss.Color("8") // bright black

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(accentColor).Padding(0, 1)
	cellStyle   = lipgloss.NewStyle().Padding(0, 1)
	borderStyle = lipgloss.NewStyle().Foreground(mutedColor)
)

// section is one titled table: a sensor category and its rendered rows.
type section struct {
	title string
	rows  [][]string
}

// TableOutput renders sensors as tables. The coloured mode (-o table) opens a
// scrollable full-screen view (tuioutput.go) when stdout and stdin are a
// terminal and the tables are taller than it; otherwise it prints static
// tables, so piped output and short listings behave like a plain CLI. The ASCII
// mode (-o ascii) always prints static, uncoloured tables with ASCII borders.
type TableOutput struct {
	writer  io.Writer
	isASCII bool
}

// NewTableOutput returns a TableOutput that writes to stdout.
func NewTableOutput(isASCII bool) Output {
	return TableOutput{writer: os.Stdout, isASCII: isASCII}
}

func (to TableOutput) All() {
	all := GetAll()
	secs := make([]section, 0, len(all))
	keys := sortedKeys(all)

	slices.SortStableFunc(keys, func(a, b string) int { return cmp.Compare(categoryRank(a), categoryRank(b)) })

	for _, k := range keys {
		if m, ok := all[k].(map[string]any); ok {
			secs = appendSection(secs, k, m)
		}
	}

	to.show(secs)
}

func (to TableOutput) Battery()     { to.show(appendSection(nil, "Battery", GetBattery())) }
func (to TableOutput) Current()     { to.show(appendSection(nil, "Current", GetCurrent())) }
func (to TableOutput) Fans()        { to.show(appendSection(nil, "Fans", GetFans())) }
func (to TableOutput) Hardware()    { to.show(appendSection(nil, "Hardware", GetHardware())) }
func (to TableOutput) Power()       { to.show(appendSection(nil, "Power", GetPower())) }
func (to TableOutput) Temperature() { to.show(appendSection(nil, "Temperature", GetTemperature())) }
func (to TableOutput) Voltage()     { to.show(appendSection(nil, "Voltage", GetVoltage())) }

// appendSection appends smcdata as a section titled name, in natural key order,
// skipping it when it holds no rows. Numeric values (readings) are right-aligned
// to a common width, text values (model names) stay left-aligned, since neither
// renderer supports per-cell alignment.
func appendSection(secs []section, name string, smcdata map[string]any) []section {
	rows := make([][]string, 0, len(smcdata))

	for _, k := range sortedKeys(smcdata) {
		if v, ok := smcdata[k].(map[string]any); ok {
			rows = append(rows, []string{k, fmt.Sprint(v["key"]), fmt.Sprint(v["value"]), fmt.Sprint(v["type"])})
		}
	}

	if len(rows) == 0 {
		return secs
	}

	valueWidth := 0
	for _, r := range rows {
		valueWidth = max(valueWidth, lipgloss.Width(r[2]))
	}

	for _, r := range rows {
		if isNumeric(r[2]) {
			r[2] = strings.Repeat(" ", valueWidth-lipgloss.Width(r[2])) + r[2]
		}
	}

	return append(secs, section{title: name, rows: rows})
}

// isNumeric reports whether v reads as a number ("45.5 °C", "-1.2 A", "20").
func isNumeric(v string) bool {
	v = strings.TrimPrefix(v, "-")

	return v != "" && v[0] >= '0' && v[0] <= '9'
}

// show prints secs statically, or opens the scrollable view when the writer and
// stdin are a terminal and the static form would not fit on screen.
func (to TableOutput) show(secs []section) {
	if len(secs) == 0 {
		return
	}

	static := to.renderStatic(secs)

	f, ok := to.writer.(*os.File)
	if to.isASCII || !ok || !term.IsTerminal(f.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		_, _ = lipgloss.Fprint(to.writer, static)

		return
	}

	_, h, err := term.GetSize(f.Fd())
	if err != nil || lipgloss.Height(static) < h {
		_, _ = lipgloss.Fprint(to.writer, static)

		return
	}

	_, err = tea.NewProgram(newTUIModel(secs), tea.WithOutput(f)).Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "interactive table: %v\n", err)

		_, _ = lipgloss.Fprint(to.writer, static)
	}
}

// renderStatic renders every section as a bordered table under a centred title.
func (to TableOutput) renderStatic(secs []section) string {
	border, borders, title, header := lipgloss.RoundedBorder(), borderStyle, titleStyle, headerStyle
	if to.isASCII {
		border, borders, title, header = lipgloss.ASCIIBorder(), lipgloss.NewStyle(), lipgloss.NewStyle(), cellStyle
	}

	var b strings.Builder

	for _, s := range secs {
		t := lgtable.New().
			Border(border).
			BorderStyle(borders).
			Headers(tableHeaders...).
			Rows(s.rows...).
			StyleFunc(func(row, _ int) lipgloss.Style {
				if row == lgtable.HeaderRow {
					return header
				}

				return cellStyle
			}).
			String()

		b.WriteString(strings.TrimRight(lipgloss.PlaceHorizontal(lipgloss.Width(t), lipgloss.Center, title.Render(s.title)), " "))
		b.WriteString("\n")
		b.WriteString(t)
		b.WriteString("\n\n")
	}

	return b.String()
}
