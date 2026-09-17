package viewer

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// compileMatcher returns a predicate for "does s contain a match", used by
// formats that only need to filter/include-or-exclude (CSV row filtering,
// the JSON/YAML/TOML tree's existing filter-to-matches behavior) rather
// than needing exact match positions. Case-insensitive substring match when
// useRegex is false; a compiled regexp when true — the one place that
// decision is made, so every format's find gets the regex option for free
// instead of reimplementing this branch.
func compileMatcher(query string, useRegex bool) (func(s string) bool, error) {
	if query == "" {
		return func(string) bool { return false }, nil
	}
	if useRegex {
		re, err := regexp.Compile(query)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	}
	lower := strings.ToLower(query)
	return func(s string) bool { return strings.Contains(strings.ToLower(s), lower) }, nil
}

// findAllOffsets returns the rune offset of every match of query within
// text, for formats that need actual positions (Text/Markdown's
// cursor/scroll jump) rather than just a yes/no filter. Offsets are in
// runes, not bytes, so callers can index into []rune(text) or count
// characters per line safely for non-ASCII content.
func findAllOffsets(text, query string, useRegex bool) ([]int, error) {
	if query == "" {
		return nil, nil
	}
	if useRegex {
		re, err := regexp.Compile(query)
		if err != nil {
			return nil, err
		}
		locs := re.FindAllStringIndex(text, -1)
		offsets := make([]int, len(locs))
		for i, l := range locs {
			offsets[i] = utf8.RuneCountInString(text[:l[0]])
		}
		return offsets, nil
	}

	lower := strings.ToLower(text)
	lq := strings.ToLower(query)
	var offsets []int
	start := 0
	for {
		idx := strings.Index(lower[start:], lq)
		if idx < 0 {
			break
		}
		byteOff := start + idx
		offsets = append(offsets, utf8.RuneCountInString(text[:byteOff]))
		start = byteOff + len(lq)
	}
	return offsets, nil
}

// rowColForRuneOffset converts a rune offset into text into a (row, col)
// pair suitable for widget.Entry's CursorRow/CursorColumn — both counted in
// runes per line, matching Entry's own convention.
func rowColForRuneOffset(text string, runeOffset int) (row, col int) {
	runes := []rune(text)
	if runeOffset > len(runes) {
		runeOffset = len(runes)
	}
	for _, r := range runes[:runeOffset] {
		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return row, col
}

// findBar is the shared find/filter UI used by every format's viewer: a
// query entry, a regex toggle, prev/next, and a status label. One look and
// feel app-wide instead of a bespoke search box per format.
type findBar struct {
	content fyne.CanvasObject
	entry   *widget.Entry
	regex   *widget.Check
	status  *widget.Label
}

// newFindBar builds the bar. onQuery fires on every keystroke or regex-
// toggle change (for live filtering formats like CSV/JSON); onNext/onPrev
// fire from the buttons or Enter (shift+Enter for prev, where supported).
func newFindBar(onQuery func(query string, useRegex bool), onNext, onPrev func()) *findBar {
	fb := &findBar{
		entry:  widget.NewEntry(),
		regex:  widget.NewCheck("Regex", nil),
		status: widget.NewLabel(""),
	}
	fb.entry.SetPlaceHolder("Find...")

	fire := func() {
		if onQuery != nil {
			onQuery(fb.entry.Text, fb.regex.Checked)
		}
	}
	fb.entry.OnChanged = func(string) { fire() }
	fb.entry.OnSubmitted = func(string) {
		if onNext != nil {
			onNext()
		}
	}
	fb.regex.OnChanged = func(bool) { fire() }

	prevBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if onPrev != nil {
			onPrev()
		}
	})
	nextBtn := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() {
		if onNext != nil {
			onNext()
		}
	})

	fb.content = container.NewBorder(nil, nil, nil,
		container.NewHBox(fb.regex, prevBtn, nextBtn, fb.status),
		fb.entry)
	return fb
}

// SetStatus updates the bar's status label (e.g. "3 of 12", "no matches",
// or a regex compile error).
func (fb *findBar) SetStatus(text string) {
	fb.status.SetText(text)
}
