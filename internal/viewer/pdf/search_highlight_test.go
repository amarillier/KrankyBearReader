package pdf

import "testing"

// Sample drawn from a real page's MuPDF HTML export (see CLAUDE.md for
// the investigation this came from) — two lines, one with a bold run
// mixed into a plain one, confirming parseTextLines handles nested tags
// and picks up the font-size from wherever it first appears.
const sampleHTML = `<div id="page51" style="width:576.0pt;height:756.0pt">
<p style="top:725.3pt;left:36.0pt;line-height:8.5pt"><b><span style="font-family:Agenda,serif;font-size:8.5pt">2026&#x2013;2027 </span></b><span style="font-family:Agenda,serif;font-size:8.5pt">Hunting Guide</span></p>
<p style="top:107.6pt;left:36.2pt;line-height:18.0pt"><span style="font-family:Agenda,serif;font-size:14.0pt">Deer season opens soon</span></p>
</div>`

func TestParseTextLines_ExtractsPositionsAndPlainText(t *testing.T) {
	lines := parseTextLines(sampleHTML)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %+v", len(lines), lines)
	}

	l0 := lines[0]
	if l0.topPt != 725.3 || l0.leftPt != 36.0 || l0.heightPt != 8.5 {
		t.Errorf("line 0 position = top:%v left:%v height:%v, want 725.3/36.0/8.5", l0.topPt, l0.leftPt, l0.heightPt)
	}
	if l0.fontSizePt != 8.5 {
		t.Errorf("line 0 fontSizePt = %v, want 8.5", l0.fontSizePt)
	}
	// The en-dash entity must be decoded, and nested <b>/<span> tags
	// stripped, leaving plain, mergeable text.
	want := "2026–2027 Hunting Guide"
	if l0.text != want {
		t.Errorf("line 0 text = %q, want %q", l0.text, want)
	}

	l1 := lines[1]
	if l1.fontSizePt != 14.0 {
		t.Errorf("line 1 fontSizePt = %v, want 14.0", l1.fontSizePt)
	}
	if l1.text != "Deer season opens soon" {
		t.Errorf("line 1 text = %q", l1.text)
	}
}

func TestParseTextLines_NoFontSizeFallsBackToLineHeight(t *testing.T) {
	html := `<p style="top:10.0pt;left:5.0pt;line-height:12.0pt">plain</p>`
	lines := parseTextLines(html)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0].fontSizePt != 12.0 {
		t.Errorf("fontSizePt fallback = %v, want 12.0 (the line's own line-height)", lines[0].fontSizePt)
	}
}

func TestParseTextLines_EmptyOrNoMatchesReturnsEmpty(t *testing.T) {
	if lines := parseTextLines(`<div id="page0" style="width:200.0pt;height:200.0pt"></div>`); len(lines) != 0 {
		t.Errorf("expected no lines for a textless page, got %d", len(lines))
	}
}

func TestCompileTextMatchPositions_PlainTextCaseInsensitive(t *testing.T) {
	matcher, err := compileTextMatchPositions("season", false)
	if err != nil {
		t.Fatalf("compileTextMatchPositions: %v", err)
	}
	got := matcher("Deer Season opens, hunting season closes")
	want := [][2]int{{5, 11}, {27, 33}}
	if len(got) != len(want) {
		t.Fatalf("got %v matches, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("match %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestCompileTextMatchPositions_Regex(t *testing.T) {
	matcher, err := compileTextMatchPositions(`\d{4}`, true)
	if err != nil {
		t.Fatalf("compileTextMatchPositions: %v", err)
	}
	got := matcher("season 2026-2027")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %v", got)
	}
}

func TestCompileTextMatchPositions_InvalidRegexErrors(t *testing.T) {
	if _, err := compileTextMatchPositions("[", true); err == nil {
		t.Error("expected an error for an invalid regex")
	}
}

func TestCompileTextMatchPositions_EmptyQueryMatchesNothing(t *testing.T) {
	matcher, err := compileTextMatchPositions("", false)
	if err != nil {
		t.Fatalf("compileTextMatchPositions: %v", err)
	}
	if got := matcher("anything at all"); len(got) != 0 {
		t.Errorf("expected no matches for an empty query, got %v", got)
	}
}

func TestLineMatchRect_EstimatesBoxFromFontSizeAndFlipsToPdfSpace(t *testing.T) {
	// A line anchored 100pt from the page's top, 18pt tall, 10pt font,
	// on a 792pt-tall page (US Letter) -- top/left are HTML/CSS
	// (origin top-left); the returned rect must be in PDF space
	// (origin bottom-left).
	line := textLine{topPt: 100, leftPt: 50, heightPt: 18, fontSizePt: 10, text: "hello world"}
	idx := [2]int{6, 11} // "world"

	rect := lineMatchRect(line, idx, 792)

	wantY0 := 792.0 - 100 - 18 // 674
	wantY1 := 792.0 - 100      // 692
	if rect[1] != wantY0 || rect[3] != wantY1 {
		t.Errorf("rect y-range = [%v,%v], want [%v,%v]", rect[1], rect[3], wantY0, wantY1)
	}
	// The raw (pre-padding) estimate, then widened by
	// searchHighlightLeftPadFrac/RightPadFrac of the match's own width --
	// see lineMatchRect's own doc comment for why the box is deliberately
	// wider than the raw per-character estimate.
	avgCharWidth := 10.0 * avgCharWidthFactor
	rawX0 := 50 + 6*avgCharWidth
	rawX1 := 50 + 11*avgCharWidth
	matchWidth := rawX1 - rawX0
	wantX0 := rawX0 - matchWidth*searchHighlightLeftPadFrac
	wantX1 := rawX1 + matchWidth*searchHighlightRightPadFrac
	if rect[0] != wantX0 || rect[2] != wantX1 {
		t.Errorf("rect x-range = [%v,%v], want [%v,%v]", rect[0], rect[2], wantX0, wantX1)
	}
}

func TestLineMatchRect_HandlesMultiByteRunesByCountingRunesNotBytes(t *testing.T) {
	// "café" has a 2-byte 'é' -- a byte-offset-based width estimate would
	// overshoot; lineMatchRect must count runes instead.
	line := textLine{topPt: 0, leftPt: 0, heightPt: 10, fontSizePt: 10, text: "café bar"}
	// byte range for "bar" (starts after "café " = 4 runes + space, but
	// "café" is 5 bytes since é is 2 bytes) -- construct via a real find.
	start := len("café ") // byte offset
	idx := [2]int{start, start + len("bar")}

	rect := lineMatchRect(line, idx, 100)

	avgCharWidth := 10.0 * avgCharWidthFactor
	wantStartRune := 5 // "café " is 5 runes: c,a,f,é,space
	wantEndRune := 8
	rawX0 := 0 + float64(wantStartRune)*avgCharWidth
	rawX1 := 0 + float64(wantEndRune)*avgCharWidth
	wantX0 := rawX0 - (rawX1-rawX0)*searchHighlightLeftPadFrac
	if wantX0 < line.leftPt {
		wantX0 = line.leftPt
	}
	if rect[0] != wantX0 {
		t.Errorf("rect x0 = %v, want %v (rune-counted, not byte-counted)", rect[0], wantX0)
	}
}

// TestLineMatchRect_PaddingNeverCrossesTheLineOwnLeftMargin confirms the
// left-padding clamp: a match starting right at (or near) the line's own
// start must never push x0 before line.leftPt -- that would drift into
// whatever sits to the left of this column, the exact class of bug
// avgCharWidthFactor's own calibration (see its doc comment) fixed for
// the unpadded estimate; padding must not reintroduce a smaller version
// of it.
func TestLineMatchRect_PaddingNeverCrossesTheLineOwnLeftMargin(t *testing.T) {
	line := textLine{topPt: 0, leftPt: 200, heightPt: 10, fontSizePt: 10, text: "bait fish"}
	idx := [2]int{0, 4} // "bait", at the very start of the line

	rect := lineMatchRect(line, idx, 100)

	if rect[0] < line.leftPt {
		t.Errorf("rect x0 = %v, want >= line.leftPt (%v) -- left padding must clamp at the line's own start", rect[0], line.leftPt)
	}
	if rect[0] != line.leftPt {
		t.Errorf("rect x0 = %v, want exactly line.leftPt (%v) since the raw estimate already starts there and padding would push negative", rect[0], line.leftPt)
	}
}
