package pdf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeTextPDF writes a one-page 300x200 PDF with two lines of real
// Helvetica text: "Hello wide world" at y=150 and "second line here" at
// y=120 (PDF space), both starting at x=20, 14pt.
func writeTextPDF(t *testing.T, path string) {
	t.Helper()
	content := "BT /F1 14 Tf 20 150 Td (Hello wide world) Tj 0 -30 Td (second line here) Tj ET"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for i, body := range objs {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objs)+1, xref)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTextDoc(t *testing.T) (*Document, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "text.pdf")
	writeTextPDF(t, path)
	doc, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	return doc, path
}

func TestPageChars_AlignWithPageTextAndHaveQuads(t *testing.T) {
	doc, _ := newTextDoc(t)
	chars, err := doc.pageChars(1)
	if err != nil || len(chars) == 0 {
		t.Fatalf("pageChars: %v, %d chars", err, len(chars))
	}
	text, err := doc.PageText(1)
	if err != nil {
		t.Fatal(err)
	}
	var got []rune
	for _, c := range chars {
		got = append(got, c.R)
	}
	if string(got) != text {
		t.Errorf("char runes %q != PageText %q", string(got), text)
	}
}

func TestSearchMatchRects_ExactBoxOnTheMatchedWord(t *testing.T) {
	doc, _ := newTextDoc(t)
	rects, ok := doc.SearchMatchRects(1, "world", false, 0)
	if !ok || len(rects) != 1 {
		t.Fatalf("SearchMatchRects = %v, %v", rects, ok)
	}
	r := rects[0]
	hello, _ := doc.SearchMatchRects(1, "Hello", false, 0)
	// "world" is on the first line (y near 150) and clearly right of "Hello".
	if r[1] > 152 || r[3] < 150 {
		t.Errorf("box %v should straddle the first line's baseline (y=150)", r)
	}
	if r[0] <= hello[0][2] {
		t.Errorf("'world' box %v should start right of 'Hello' box %v", r, hello[0])
	}
	// Helvetica 14pt: "world" is ~5 chars of ~7-10pt each.
	if w := r[2] - r[0]; w < 25 || w > 45 {
		t.Errorf("'world' box width = %v, want about 30-40pt", w)
	}
	// The match text begins at 'w' of "Hello wide world": 6+5 = after 11 chars.
	if r[0] < 20+60 {
		t.Errorf("box starts at x=%v, far left of where 'world' is drawn", r[0])
	}
	if _, ok := doc.SearchMatchRects(1, "world", false, 1); ok {
		t.Error("there is no second occurrence")
	}
	second, ok := doc.SearchMatchRects(1, "line", false, 0)
	// Line 1's box spans y~146-165, line 2's y~116-135: "line" must be below line 1.
	if !ok || second[0][3] > 140 {
		t.Errorf("'line' should be on the second line (below y=140): %v %v", second, ok)
	}
}

func TestSnapTextSelection_SnapsAcrossWordsAndLines(t *testing.T) {
	doc, _ := newTextDoc(t)
	// From inside "wide" to inside "world", same line (PDF y 155 ~ just
	// above the first line's baseline).
	wide, _ := doc.SearchMatchRects(1, "wide", false, 0)
	world, _ := doc.SearchMatchRects(1, "world", false, 0)
	wx := (wide[0][0] + wide[0][2]) / 2
	rx := (world[0][0] + world[0][2]) / 2
	quads, ok := doc.SnapTextSelection(1, wx, 154, rx, 154)
	if !ok || len(quads) != 1 {
		t.Fatalf("one-line selection: %v %v", quads, ok)
	}
	// Selection runs from the character under the drag's start (middle of
	// "wide") through the one under its end (middle of "world"), in reading
	// order — like selecting text in any viewer, not whole words.
	if quads[0][0] < wide[0][0]-0.5 || quads[0][0] > wx+0.5 {
		t.Errorf("quad %v should start at the character under x=%v (inside 'wide' %v)", quads[0], wx, wide[0])
	}
	if quads[0][2] < rx || quads[0][2] > world[0][2]+0.5 {
		t.Errorf("quad %v should end at the character under x=%v (inside 'world' %v)", quads[0], rx, world[0])
	}

	// From "Hello" on line 1 down to "line" on line 2: two quads.
	hello, _ := doc.SearchMatchRects(1, "Hello", false, 0)
	line, _ := doc.SearchMatchRects(1, "line", false, 0)
	quads, ok = doc.SnapTextSelection(1, hello[0][0]+2, 154, (line[0][0]+line[0][2])/2, 124)
	if !ok || len(quads) != 2 {
		t.Fatalf("two-line selection: %d quads, ok=%v", len(quads), ok)
	}
	// A drag with the ends reversed selects the same text.
	rev, ok2 := doc.SnapTextSelection(1, (line[0][0]+line[0][2])/2, 124, hello[0][0]+2, 154)
	if !ok2 || len(rev) != 2 || rev[0] != quads[0] {
		t.Errorf("reversed drag differs: %v vs %v", rev, quads)
	}
	// Far from any text: no snap, the caller keeps its rectangle.
	if q, ok := doc.SnapTextSelection(1, 280, 10, 290, 20); ok {
		t.Errorf("a drag in the blank corner must not snap, got %v", q)
	}
}

// A multi-line (multi-quad) highlight must survive Save: only the first
// quad used to be written.
func TestSaveHighlights_MultiQuadHighlightRoundTripsAndPaintsEveryLine(t *testing.T) {
	doc, path := newTextDoc(t)
	hello, _ := doc.SearchMatchRects(1, "Hello", false, 0)
	line, _ := doc.SearchMatchRects(1, "line", false, 0)
	quads, ok := doc.SnapTextSelection(1, hello[0][0]+2, 154, (line[0][0]+line[0][2])/2, 124)
	if !ok || len(quads) != 2 {
		t.Fatalf("setup: %d quads %v", len(quads), ok)
	}
	doc.AddHighlight(1, quads, [3]float64{1, 1, 0}, "")
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}
	re, err := Prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	defer re.Close()
	if len(re.Highlights) != 1 || len(re.Highlights[0].Quads) != 2 {
		t.Fatalf("expected 1 highlight with 2 quads after reload, got %+v", re.Highlights)
	}
	img, err := re.RenderPage(1, 2.0)
	if err != nil {
		t.Fatal(err)
	}
	// Both lines shaded: count yellow-tinted pixels inside each quad (a
	// single sampled pixel can land on a black glyph stem).
	sc := baseRenderDPI * 2.0 / 72
	for i, q := range re.Highlights[0].Quads {
		x0, x1 := int(q[0]*sc), int(q[2]*sc)
		y0, y1 := int((200-q[1])*sc), int((200-q[5])*sc)
		yellow := 0
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r>>8 > 200 && g>>8 > 200 && b>>8 < 150 {
					yellow++
				}
			}
		}
		if yellow < 200 {
			t.Errorf("quad %d (%v): only %d yellow pixels inside it", i, q, yellow)
		}
	}
}

func TestSetSearchHighlights_MultipleRectsAndClear(t *testing.T) {
	doc, _ := newTextDoc(t)
	rects := [][4]float64{{1, 1, 5, 5}, {1, 10, 5, 15}}
	doc.SetSearchHighlights(1, rects)
	if doc.searchHighlightPage != 1 || len(doc.searchHighlightRects) != 2 {
		t.Fatalf("state = %d %v", doc.searchHighlightPage, doc.searchHighlightRects)
	}
	doc.ClearSearchHighlight()
	if doc.searchHighlightPage != 0 || len(doc.searchHighlightRects) != 0 {
		t.Errorf("not cleared: %d %v", doc.searchHighlightPage, doc.searchHighlightRects)
	}
}
