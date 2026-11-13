// SPDX-FileCopyrightText: Copyright (C) 2022 Roland Schaer
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

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

	warmStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))            // yellow
	hotStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
	criticalStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")) // bright red
)

// Temperature levels in °C. Apple Silicon hotspot keys legitimately read
// 96–113 °C under sustained load while macOS stays at Nominal pressure, so no
// single temperature is "critical": that level comes from Thermal Pressure.
const (
	tempWarm = 80.0
	tempHot  = 95.0
)

// batteryHealthService is the full-charge share of design capacity (%) below
// which Apple recommends battery service.
const batteryHealthService = 80.0

// Fan levels as a fraction of the same fan's maximum speed. A fan at maximum
// is working hard, not failing, so fans never reach "critical".
const (
	fanWarm = 0.6
	fanHot  = 0.85
)

// section is one titled table: a sensor category and its rendered rows.
type section struct {
	title string
	rows  [][]string
}

// TableOutput renders sensors as tables. The coloured mode (-o table) opens a
// scrollable, terminal-high view (tuioutput.go) when stdout and stdin are a
// terminal and the tables are taller than it; otherwise it prints static
// tables, so piped output and short listings behave like a plain CLI. The ASCII
// mode (-o ascii) always prints static, uncoloured tables with ASCII borders.
type TableOutput struct {
	writer  io.Writer
	isASCII bool
	every   time.Duration // >0: the scrollable view re-reads sensors at this interval
}

// NewTableOutput returns a TableOutput that writes to stdout.
func NewTableOutput(isASCII bool) Output {
	return TableOutput{writer: os.Stdout, isASCII: isASCII, every: 0}
}

func (to TableOutput) All() { to.show(allSections) }

// allSections reads every category, ordered by categoryOrder.
func allSections() []section {
	all := GetAll()
	secs := make([]section, 0, len(all))
	keys := sortedKeys(all)

	slices.SortStableFunc(keys, func(a, b string) int { return cmp.Compare(categoryRank(a), categoryRank(b)) })

	for _, k := range keys {
		if m, ok := all[k].(map[string]any); ok {
			secs = appendSection(secs, k, m)
		}
	}

	return secs
}

// one returns a reader for the single category name, fetched by get.
func one(name string, get func() map[string]any) func() []section {
	return func() []section { return appendSection(nil, name, get()) }
}

func (to TableOutput) Battery()     { to.show(one("Battery", GetBattery)) }
func (to TableOutput) Current()     { to.show(one("Current", GetCurrent)) }
func (to TableOutput) Fans()        { to.show(one("Fans", GetFans)) }
func (to TableOutput) Hardware()    { to.show(one("Hardware", GetHardware)) }
func (to TableOutput) Power()       { to.show(one("Power", GetPower)) }
func (to TableOutput) Temperature() { to.show(one("Temperature", GetTemperature)) }
func (to TableOutput) Voltage()     { to.show(one("Voltage", GetVoltage)) }

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

// isNumeric reports whether v reads as a number ("45.5 °C", "-1.2 A", "20",
// " 900 rpm" as fans format it with %4.0f).
func isNumeric(v string) bool {
	v = strings.TrimPrefix(strings.TrimLeft(v, " "), "-")

	return v != "" && v[0] >= '0' && v[0] <= '9'
}

// colorize paints each value cell by how close the reading is to its limit:
// temperatures against fixed levels, fan speeds against the same fan's maximum.
// Power, voltage and current have no generic limit and stay plain. It runs
// after appendSection's alignment; widths ignore the ANSI codes it adds.
func colorize(secs []section) {
	for _, s := range secs {
		byDesc := make(map[string]string, len(s.rows))
		for _, r := range s.rows {
			byDesc[r[0]] = r[2]
		}

		for _, r := range s.rows {
			if st, ok := severity(r[0], r[2], byDesc); ok {
				r[2] = st.Render(r[2])
			}
		}
	}
}

// severity returns the style for one reading, or false when it is normal or
// has no known limit. byDesc maps the section's descriptions to their values.
func severity(desc, value string, byDesc map[string]string) (lipgloss.Style, bool) {
	if desc == thermalPressureDesc {
		return pressureStyle(value)
	}

	v, unit, ok := reading(value)
	if !ok {
		return lipgloss.Style{}, false
	}

	if desc == "Battery Health" && v < batteryHealthService {
		return warmStyle, true
	}

	switch unit {
	case "°C":
		return levelStyle(v, tempWarm, tempHot, math.Inf(1))
	case "rpm":
		for _, sfx := range []string{" Current Speed", " Target Speed"} {
			if fan, found := strings.CutSuffix(desc, sfx); found {
				if maxRPM, _, ok := reading(byDesc[fan+" Maximum Speed"]); ok && maxRPM > 0 {
					return levelStyle(v/maxRPM, fanWarm, fanHot, math.Inf(1))
				}
			}
		}
	}

	return lipgloss.Style{}, false
}

// pressureStyle colours the macOS thermal pressure level: Moderate is warm,
// Heavy hot, and Trapping or Sleeping (macOS is throttling hard or about to
// sleep the machine) critical. Nominal stays plain.
func pressureStyle(level string) (lipgloss.Style, bool) {
	switch level {
	case "Moderate":
		return warmStyle, true
	case "Heavy":
		return hotStyle, true
	case "Trapping", "Sleeping":
		return criticalStyle, true
	default:
		return lipgloss.Style{}, false
	}
}

// levelStyle maps v onto the warm/hot/critical styles by its thresholds.
func levelStyle(v, warm, hot, critical float64) (lipgloss.Style, bool) {
	switch {
	case v >= critical:
		return criticalStyle, true
	case v >= hot:
		return hotStyle, true
	case v >= warm:
		return warmStyle, true
	default:
		return lipgloss.Style{}, false
	}
}

// reading splits a formatted value ("45.3 °C", " 900 rpm") into number and unit.
func reading(value string) (float64, string, bool) {
	f := strings.Fields(value)
	if len(f) != 2 {
		return 0, "", false
	}

	v, err := strconv.ParseFloat(f[0], 64)

	return v, f[1], err == nil
}

// interactive reports whether the coloured view can run: the writer and stdin
// are a terminal and the mode is not ASCII.
func (to TableOutput) interactive() bool {
	f, ok := to.writer.(*os.File)

	return !to.isASCII && ok && term.IsTerminal(f.Fd()) && term.IsTerminal(os.Stdin.Fd())
}

// show prints the sections build reads statically, or opens the scrollable
// view when interactive and the static form would not fit on screen. With a
// refresh interval the view always opens and re-reads sensors through build.
func (to TableOutput) show(build func() []section) {
	load := func() []section {
		secs := build()
		if !to.isASCII {
			colorize(secs)
		}

		return secs
	}

	secs := load()
	if len(secs) == 0 {
		return
	}

	static := to.renderStatic(secs)

	if !to.interactive() {
		_, _ = lipgloss.Fprint(to.writer, static)

		return
	}

	f, _ := to.writer.(*os.File)

	_, h, err := term.GetSize(f.Fd())
	if to.every <= 0 && (err != nil || lipgloss.Height(static) < h) {
		_, _ = lipgloss.Fprint(to.writer, static)

		return
	}

	_, err = tea.NewProgram(newTUIModel(secs, load, to.every), tea.WithOutput(f)).Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "interactive table: %v\n", err)

		_, _ = lipgloss.Fprint(to.writer, static)
	}
}

// newTable returns a table with the given border, border style and header
// style; body cells use cellStyle. Both the static tables and the scrollable
// view build on it, so they cannot drift apart.
func newTable(border lipgloss.Border, borders, header lipgloss.Style) *lgtable.Table {
	return lgtable.New().
		Border(border).
		BorderStyle(borders).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == lgtable.HeaderRow {
				return header
			}

			return cellStyle
		})
}

// renderStatic renders every section as a bordered table under a centred title.
func (to TableOutput) renderStatic(secs []section) string {
	border, borders, title, header := lipgloss.RoundedBorder(), borderStyle, titleStyle, headerStyle
	if to.isASCII {
		border, borders, title, header = lipgloss.ASCIIBorder(), lipgloss.NewStyle(), lipgloss.NewStyle(), cellStyle
	}

	var b strings.Builder

	for _, s := range secs {
		t := newTable(border, borders, header).Headers(tableHeaders...).Rows(s.rows...).String()

		b.WriteString(strings.TrimRight(lipgloss.PlaceHorizontal(lipgloss.Width(t), lipgloss.Center, title.Render(s.title)), " "))
		b.WriteString("\n")
		b.WriteString(t)
		b.WriteString("\n\n")
	}

	return b.String()
}
