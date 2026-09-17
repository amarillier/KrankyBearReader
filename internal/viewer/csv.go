package viewer

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// candidateDelimiters are checked in order; comma is also the fallback when no
// candidate looks consistent, since it's by far the most common default.
var candidateDelimiters = []rune{',', '\t', ';', '|'}

// DetectDelimiter guesses a delimited-text file's separator by counting each
// candidate's occurrences per line across a sample and picking whichever is
// both present and appears the same number of times on the most lines (a
// consistent per-row field count is the strongest signal of the real
// delimiter). Falls back to comma when no candidate is consistent.
func DetectDelimiter(sample []byte) rune {
	scanner := bufio.NewScanner(bytes.NewReader(sample))
	const maxLines = 10

	counts := make(map[rune][]int, len(candidateDelimiters))
	lines := 0
	for scanner.Scan() && lines < maxLines {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		for _, d := range candidateDelimiters {
			counts[d] = append(counts[d], strings.Count(line, string(d)))
		}
		lines++
	}
	if lines == 0 {
		return ','
	}

	best := ','
	bestConsistentLines := 0
	for _, d := range candidateDelimiters {
		perLine := counts[d]
		if len(perLine) == 0 || perLine[0] == 0 {
			continue
		}
		want := perLine[0]
		consistent := 0
		for _, c := range perLine {
			if c == want {
				consistent++
			}
		}
		if consistent > bestConsistentLines {
			bestConsistentLines = consistent
			best = d
		}
	}
	return best
}

// ParseDelimited parses data with the given delimiter into a header row and
// the remaining data rows. A file with only a header (or no rows at all)
// returns an empty data slice, not an error.
func ParseDelimited(data []byte, delimiter rune) (header []string, rows [][]string, err error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delimiter
	r.FieldsPerRecord = -1 // tolerate ragged rows rather than failing the whole file
	r.LazyQuotes = true

	all, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(all) == 0 {
		return nil, nil, nil
	}
	return all[0], all[1:], nil
}

const maxCSVColumnWidth float32 = 300

// NewCSVView renders delimited data as a table with a frozen, bold header row
// taken from the file's own first line (not Fyne's default A-Z/1-10 labels),
// plus a find bar that filters to rows with a matching cell.
func NewCSVView(header []string, rows [][]string) fyne.CanvasObject {
	cols := len(header)
	visible := rows // filtered view; equals rows with no active query

	table := widget.NewTable(
		func() (int, int) { return len(visible), cols },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			if id.Row < len(visible) && id.Col < len(visible[id.Row]) {
				label.SetText(visible[id.Row][id.Col])
			} else {
				label.SetText("")
			}
		},
	)
	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		l := widget.NewLabel("")
		l.TextStyle = fyne.TextStyle{Bold: true}
		return l
	}
	table.UpdateHeader = func(id widget.TableCellID, obj fyne.CanvasObject) {
		label := obj.(*widget.Label)
		switch {
		case id.Row == -1 && id.Col >= 0 && id.Col < len(header):
			label.SetText(header[id.Col])
		case id.Row == -1:
			label.SetText(fmt.Sprintf("Column %d", id.Col+1))
		default:
			label.SetText(fmt.Sprintf("%d", id.Row+1))
		}
	}

	for col := 0; col < cols; col++ {
		table.SetColumnWidth(col, sampleColumnWidth(header, rows, col))
	}

	current := -1 // current row within visible, for Next/Prev
	scrollToCurrent := func() {
		if current >= 0 && current < len(visible) {
			table.ScrollTo(widget.TableCellID{Row: current, Col: 0})
		}
	}

	var bar *findBar
	bar = newFindBar(
		func(query string, useRegex bool) {
			matches, err := compileMatcher(query, useRegex)
			if err != nil {
				bar.SetStatus(err.Error())
				return
			}
			if query == "" {
				visible = rows
			} else {
				filtered := make([][]string, 0, len(rows))
				for _, row := range rows {
					for _, cell := range row {
						if matches(cell) {
							filtered = append(filtered, row)
							break
						}
					}
				}
				visible = filtered
			}
			current = -1
			table.Refresh()
			if query == "" {
				bar.SetStatus("")
			} else {
				bar.SetStatus(fmt.Sprintf("%d row(s)", len(visible)))
			}
		},
		func() { // next
			if len(visible) == 0 {
				return
			}
			current = (current + 1) % len(visible)
			scrollToCurrent()
		},
		func() { // prev
			if len(visible) == 0 {
				return
			}
			current--
			if current < 0 {
				current = len(visible) - 1
			}
			scrollToCurrent()
		},
	)

	return container.NewBorder(bar.content, nil, nil, nil, table)
}

// sampleColumnWidth estimates a pleasant column width from the header and the
// first few data rows, capped so one very long cell can't blow out the table.
func sampleColumnWidth(header []string, rows [][]string, col int) float32 {
	widest := len(safeCell(header, col))
	const sampleRows = 20
	for i := 0; i < len(rows) && i < sampleRows; i++ {
		if l := len(safeCell(rows[i], col)); l > widest {
			widest = l
		}
	}
	width := float32(widest)*7 + 20 // rough monospace-ish px-per-char estimate + padding
	if width < 80 {
		width = 80
	}
	if width > maxCSVColumnWidth {
		width = maxCSVColumnWidth
	}
	return width
}

func safeCell(row []string, col int) string {
	if col < len(row) {
		return row[col]
	}
	return ""
}
