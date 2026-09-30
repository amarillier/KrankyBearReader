package pdf

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// textLine is one line of text as MuPDF's own HTML export (go-fitz's
// existing, unforked Document.HTML) reports it: a position anchor
// (top/left, HTML/CSS convention — origin top-left of the page) plus a
// line height and a representative font size, both in PDF-space points.
// This is the ONLY per-line position data go-fitz exposes at all — no
// per-word x-position, no explicit line width (confirmed empirically
// against a real file's HTML dump: every <p> carries exactly top/left/
// line-height, nothing else, and nested <span>s carry font-size but no
// position of their own) — so SearchMatchRect's own horizontal offset/
// width is necessarily an estimate built from fontSizePt, not a real
// character-box lookup. See CLAUDE.md for the investigation that found
// this and why it doesn't need another go-fitz fork.
type textLine struct {
	topPt, leftPt, heightPt, fontSizePt float64
	text                                string
}

// htmlLineRe matches one <p style="top:Xpt;left:Ypt;line-height:Zpt">...</p>
// block the way MuPDF's HTML export always emits it — confirmed against a
// real 576x756pt page's output, every line following this exact style-
// attribute order and unit. (?s) so a line's own inner content can't
// accidentally break the match if it ever spans a literal newline in the
// raw HTML.
var htmlLineRe = regexp.MustCompile(`(?s)<p style="top:([\d.]+)pt;left:([\d.]+)pt;line-height:([\d.]+)pt"[^>]*>(.*?)</p>`)

// fontSizeRe finds the first font-size on a line's own nested <span> — the
// line's dominant size in practice (mixed bold/color runs within one line
// still share the same font-size in every real file seen so far).
var fontSizeRe = regexp.MustCompile(`font-size:([\d.]+)pt`)

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// parseTextLines extracts every text line's position+text from a page's
// raw MuPDF HTML export. A line with no recognizable font-size falls back
// to its own line-height as a rough proxy — close enough for the same
// estimate SearchMatchRect already makes.
func parseTextLines(htmlStr string) []textLine {
	matches := htmlLineRe.FindAllStringSubmatch(htmlStr, -1)
	lines := make([]textLine, 0, len(matches))
	for _, m := range matches {
		top, _ := strconv.ParseFloat(m[1], 64)
		left, _ := strconv.ParseFloat(m[2], 64)
		height, _ := strconv.ParseFloat(m[3], 64)
		inner := m[4]
		fontSize := height
		if fm := fontSizeRe.FindStringSubmatch(inner); fm != nil {
			if v, err := strconv.ParseFloat(fm[1], 64); err == nil && v > 0 {
				fontSize = v
			}
		}
		text := html.UnescapeString(htmlTagRe.ReplaceAllString(inner, ""))
		lines = append(lines, textLine{topPt: top, leftPt: left, heightPt: height, fontSizePt: fontSize, text: text})
	}
	return lines
}

// pageTextLines returns page's (1-based) text lines with position data,
// caching per page the same way PageText does — find() calls this on
// every next/prev step.
func (d *Document) pageTextLines(page int) ([]textLine, error) {
	d.lineCacheMu.Lock()
	defer d.lineCacheMu.Unlock()

	if lines, ok := d.lineCache[page]; ok {
		return lines, nil
	}
	htmlStr, err := d.doc.HTML(page-1, false) // go-fitz is 0-based
	if err != nil {
		return nil, err
	}
	lines := parseTextLines(htmlStr)
	d.lineCache[page] = lines
	return lines, nil
}

// compileTextMatchPositions is compileTextCounter's relative that reports
// each match's own [start,end) byte offsets within s, rather than just a
// count — SearchMatchRect needs to know exactly where within a line the
// match falls, to estimate its horizontal box.
func compileTextMatchPositions(query string, useRegex bool) (func(s string) [][2]int, error) {
	if query == "" {
		return func(string) [][2]int { return nil }, nil
	}
	if useRegex {
		re, err := regexp.Compile(query)
		if err != nil {
			return nil, err
		}
		return func(s string) [][2]int {
			matches := re.FindAllStringIndex(s, -1)
			out := make([][2]int, len(matches))
			for i, m := range matches {
				out[i] = [2]int{m[0], m[1]}
			}
			return out
		}, nil
	}
	lower := strings.ToLower(query)
	return func(s string) [][2]int {
		var out [][2]int
		ls := strings.ToLower(s)
		start := 0
		for {
			i := strings.Index(ls[start:], lower)
			if i < 0 {
				break
			}
			absStart := start + i
			absEnd := absStart + len(lower)
			out = append(out, [2]int{absStart, absEnd})
			start = absEnd
		}
		return out
	}, nil
}

// avgCharWidthFactor estimates a line's average glyph advance width as a
// fraction of its own font size — go-fitz's HTML export gives no real
// per-character metrics at all (see this file's own doc comment), so this
// is a deliberate, disclosed approximation: close enough to land the
// highlight box on the right word, not a substitute for real character
// positions.
//
// 0.42, not the more textbook-sounding 0.5: calibrated against two real
// failures found via hands-on testing on a real multi-column PDF (a
// match landing in the WRONG COLUMN entirely, not just imprecisely
// within the right one — see CLAUDE.md for the full investigation).
// Error compounds with every preceding character on the line, so it's
// worst for a match late in a long line; 0.5 overshot a ~171pt-wide
// column by 12pt and a ~257pt-wide one by 23pt on two independently
// confirmed real cases, landing the box in a neighboring column (at the
// same row height as text there, which is what made it look like a
// wrong-word match rather than an imprecise one). 0.42 leaves ~17-21pt
// of margin on both of those same real measurements — still an
// approximation, not exact per-character metrics, but tuned against
// real, confirmed overshoot rather than a generic typographic guess.
const avgCharWidthFactor = 0.42

// searchHighlightLeftPadFrac/RightPadFrac widen the raw estimate below by
// a fraction of the MATCH's own estimated width, asymmetrically more on
// the left. Found necessary via real hands-on testing immediately after
// avgCharWidthFactor's own calibration above: even tuned against two real
// overshoot measurements, a single global constant can't track every
// sentence's own letter-frequency mix exactly, so a small residual
// rightward drift remains on some real matches — no longer landing in
// the wrong column, just sitting a little right of the actual word
// within the right one (e.g. "baitfish" wrapping a column's last line,
// confirmed on the same real TWRA file: the box landed past the word
// entirely, in the trailing whitespace before the line's own period).
// Rather than chase tighter per-character accuracy for diminishing
// returns (see avgCharWidthFactor's own doc comment — recalibrating
// again only helps until the next sentence with a different letter mix),
// widening the box makes it visually read as "found the word" despite
// that residual drift. Scaled by the match's own width (not a fixed
// point value) so a longer query gets proportionally more padding too.
const (
	searchHighlightLeftPadFrac  = 0.6
	searchHighlightRightPadFrac = 0.15
)

// lineMatchRect estimates a PDF-space (origin bottom-left, matching
// Highlight.Rect's own convention) rect for the byte-offset range idx
// within line.text, given the page's own height (needed to flip line's
// HTML/CSS top-left-origin anchor into PDF space).
func lineMatchRect(line textLine, idx [2]int, pageHeightPt float64) [4]float64 {
	avgCharWidth := line.fontSizePt * avgCharWidthFactor
	startRune := utf8.RuneCountInString(line.text[:idx[0]])
	endRune := utf8.RuneCountInString(line.text[:idx[1]])

	x0 := line.leftPt + float64(startRune)*avgCharWidth
	x1 := line.leftPt + float64(endRune)*avgCharWidth
	matchWidth := x1 - x0
	x0 -= matchWidth * searchHighlightLeftPadFrac
	if x0 < line.leftPt {
		x0 = line.leftPt
	}
	x1 += matchWidth * searchHighlightRightPadFrac

	y0 := pageHeightPt - line.topPt - line.heightPt
	y1 := pageHeightPt - line.topPt
	return [4]float64{x0, y0, x1, y1}
}

// SearchMatchRect estimates the on-page rect of the occurrenceIndex-th
// (0-based, counted left-to-right/top-to-bottom within this one page —
// see findState.occurrenceIndexOnPage for how a caller gets this from its
// own flat, document-wide match index) occurrence of query on page.
//
// Necessarily approximate, not a real character-box lookup — go-fitz has
// no such API at all. This instead walks the page's own HTML-export line
// positions (pageTextLines) and estimates a horizontal offset/width from
// the line's own font size. A match that only exists by joining two HTML
// lines (e.g. spanning a hyphenated line-wrap) won't be found here even
// though buildFindState's own flat-PageText count still includes it — an
// accepted gap for an approximate feature, not a bug to chase.
func (d *Document) SearchMatchRect(page int, query string, useRegex bool, occurrenceIndex int) ([4]float64, bool) {
	lines, err := d.pageTextLines(page)
	if err != nil {
		return [4]float64{}, false
	}
	matcher, err := compileTextMatchPositions(query, useRegex)
	if err != nil {
		return [4]float64{}, false
	}
	_, pageHeightPt, err := d.PageBoundsPt(page)
	if err != nil {
		return [4]float64{}, false
	}
	count := 0
	for _, line := range lines {
		for _, idx := range matcher(line.text) {
			if count == occurrenceIndex {
				return lineMatchRect(line, idx, pageHeightPt), true
			}
			count++
		}
	}
	return [4]float64{}, false
}

// SetSearchHighlight marks (page, rect) as the current find-bar match's
// approximate on-page box for paintHighlights to draw — mirrors
// SetSelectedHighlight's own pattern, including clearing the whole render
// cache rather than just the affected page (see that method's own doc
// comment for why). Page 0 clears it.
func (d *Document) SetSearchHighlight(page int, rect [4]float64) {
	if d.searchHighlightPage == page && d.searchHighlightRect == rect {
		return
	}
	d.searchHighlightPage = page
	d.searchHighlightRect = rect
	d.cache.Clear()
}

// ClearSearchHighlight removes any current search highlight box.
func (d *Document) ClearSearchHighlight() {
	d.SetSearchHighlight(0, [4]float64{})
}
