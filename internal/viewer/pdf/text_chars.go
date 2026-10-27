package pdf

import (
	"math"
	"unicode"
	"unicode/utf8"

	"github.com/gen2brain/go-fitz"
)

// pageChars returns page's (1-based) characters with exact quads (go-fitz
// fork's TextChars), cached like PageText: the text layer only comes from a
// page's content stream, so annotation edits and document rebuilds never
// change it. Entry i corresponds to rune i of PageText(page).
func (d *Document) pageChars(page int) ([]fitz.TextChar, error) {
	d.charCacheMu.Lock()
	defer d.charCacheMu.Unlock()
	if cs, ok := d.charCache[page]; ok {
		return cs, nil
	}
	cs, err := d.doc.TextChars(page - 1) // go-fitz is 0-based
	if err != nil {
		return nil, err
	}
	if d.charCache == nil {
		d.charCache = map[int][]fitz.TextChar{}
	}
	d.charCache[page] = cs
	return cs, nil
}

// charBoundsPDF is a character's axis-aligned bounds converted from
// MuPDF's page space (origin top-left, y down) into PDF space (origin
// bottom-left): [minX, minY, maxX, maxY].
func charBoundsPDF(c fitz.TextChar, pageHeightPt float64) [4]float64 {
	minX, maxX := math.Inf(1), math.Inf(-1)
	minY, maxY := math.Inf(1), math.Inf(-1)
	for i := 0; i < 8; i += 2 {
		x, y := float64(c.Quad[i]), float64(c.Quad[i+1])
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	return [4]float64{minX, pageHeightPt - maxY, maxX, pageHeightPt - minY}
}

// unionRects returns the bounding box of two PDF-space rects.
func unionRects(a, b [4]float64) [4]float64 {
	return [4]float64{math.Min(a[0], b[0]), math.Min(a[1], b[1]), math.Max(a[2], b[2]), math.Max(a[3], b[3])}
}

// lineRects returns one PDF-space rect per text line covered by
// chars[from:to], skipping line-break entries — a match or selection that
// wraps onto a second line gets one box per line, not one tall box that
// would also shade the gap and the text between.
func lineRects(chars []fitz.TextChar, from, to int, pageHeightPt float64) [][4]float64 {
	var rects [][4]float64
	lastLine := -1
	for i := from; i < to && i < len(chars); i++ {
		c := chars[i]
		if c.R == '\n' {
			continue
		}
		b := charBoundsPDF(c, pageHeightPt)
		if c.Line == lastLine && len(rects) > 0 {
			rects[len(rects)-1] = unionRects(rects[len(rects)-1], b)
		} else {
			rects = append(rects, b)
			lastLine = c.Line
		}
	}
	return rects
}

// SearchMatchRects returns exact per-line boxes (PDF space) for the
// occurrenceIndex-th (0-based, within this page) match of query on page,
// from real character positions. Matching runs over the same text
// PageText returns, so the occurrence numbering lines up with
// buildFindState's own count, including a match that wraps across lines.
// ok is false if the page has no text layer or the match can't be found —
// the caller then falls back to SearchMatchRect's font-size estimate.
func (d *Document) SearchMatchRects(page int, query string, useRegex bool, occurrenceIndex int) ([][4]float64, bool) {
	chars, err := d.pageChars(page)
	if err != nil || len(chars) == 0 {
		return nil, false
	}
	matcher, err := compileTextMatchPositions(query, useRegex)
	if err != nil {
		return nil, false
	}
	_, pageHeightPt, err := d.PageBoundsPt(page)
	if err != nil {
		return nil, false
	}
	runes := make([]rune, len(chars))
	for i, c := range chars {
		runes[i] = c.R
	}
	text := string(runes)
	for n, idx := range matcher(text) {
		if n != occurrenceIndex {
			continue
		}
		from := utf8.RuneCountInString(text[:idx[0]])
		to := utf8.RuneCountInString(text[:idx[1]])
		if rects := lineRects(chars, from, to, pageHeightPt); len(rects) > 0 {
			return rects, true
		}
		return nil, false
	}
	return nil, false
}

// textSnapTolerance is how far (in multiples of the nearest character's
// font size, minimum minSnapPt) a drag's start/end may be from text and
// still snap to it.
const (
	textSnapTolerance = 2.0
	minSnapPt         = 8.0
)

// nearestChar returns the index of the real (non-line-break) character
// closest to PDF point (x, y) within the snap tolerance, or -1.
func nearestChar(chars []fitz.TextChar, x, y, pageHeightPt float64) int {
	best, bestDist := -1, math.MaxFloat64
	for i, c := range chars {
		if c.R == '\n' {
			continue
		}
		b := charBoundsPDF(c, pageHeightPt)
		dx := math.Max(0, math.Max(b[0]-x, x-b[2]))
		dy := math.Max(0, math.Max(b[1]-y, y-b[3]))
		dist := math.Hypot(dx, dy)
		if dist < bestDist && dist <= math.Max(float64(c.Size)*textSnapTolerance, minSnapPt) {
			best, bestDist = i, dist
		}
	}
	return best
}

// SnapTextSelection turns a drag from (x1, y1) to (x2, y2) (PDF space)
// into text-snapped highlight quads, the way Preview/Acrobat select text:
// every character in reading order from the one nearest the drag's start
// to the one nearest its end, one quad per line (UL, UR, LL, LR as x,y
// pairs, the convention Highlight.Quads uses). ok is false when either end
// is nowhere near text (a drag over a figure or blank margin), so the
// caller keeps its plain rectangle behavior.
func (d *Document) SnapTextSelection(page int, x1, y1, x2, y2 float64) ([][8]float64, bool) {
	chars, err := d.pageChars(page)
	if err != nil || len(chars) == 0 {
		return nil, false
	}
	_, pageHeightPt, err := d.PageBoundsPt(page)
	if err != nil {
		return nil, false
	}
	a := nearestChar(chars, x1, y1, pageHeightPt)
	b := nearestChar(chars, x2, y2, pageHeightPt)
	if a < 0 || b < 0 {
		return nil, false
	}
	if a > b {
		a, b = b, a
	}
	// Trim leading/trailing whitespace so the band doesn't stick out past
	// the words the user actually covered.
	for a < b && (chars[a].R == '\n' || unicode.IsSpace(chars[a].R)) {
		a++
	}
	for b > a && (chars[b].R == '\n' || unicode.IsSpace(chars[b].R)) {
		b--
	}
	rects := lineRects(chars, a, b+1, pageHeightPt)
	if len(rects) == 0 {
		return nil, false
	}
	quads := make([][8]float64, len(rects))
	for i, r := range rects {
		quads[i] = [8]float64{r[0], r[3], r[2], r[3], r[0], r[1], r[2], r[1]}
	}
	return quads, true
}
