package pdf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFontSizeLabels_RoundTripEveryPreset(t *testing.T) {
	for _, p := range fontSizePresets {
		got, ok := parseFontSizeLabel(fontSizeLabel(p))
		if !ok || got != p {
			t.Errorf("preset %v: round trip gave %v, %v", p, got, ok)
		}
	}
	for _, bad := range []string{"", "abc", "0pt", "-4pt"} {
		if _, ok := parseFontSizeLabel(bad); ok {
			t.Errorf("parseFontSizeLabel(%q) should be rejected", bad)
		}
	}
	if len(fontSizeOptions()) != len(fontSizePresets) {
		t.Errorf("fontSizeOptions length mismatch")
	}
}

func TestParseDAFontSize(t *testing.T) {
	cases := map[string]float64{
		"/Helv 18 Tf 0.00 0.00 1.00 rg": 18,
		"/Helv 10.5 Tf":                 10.5,
		" 0.00 0.00 0.00 rg":            defaultFontSizePt, // pdfcpu's own, no Tf
		"/Helv 0 Tf":                    defaultFontSizePt, // 0 means auto
		"":                              defaultFontSizePt,
	}
	for in, want := range cases {
		if got := parseDAFontSize(in); got != want {
			t.Errorf("parseDAFontSize(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHasFontSize_OnlyFreeText(t *testing.T) {
	if !hasFontSize("FreeText") {
		t.Error("FreeText should have a font size")
	}
	for _, k := range []string{"Square", "Highlight", "Line", "Ink", "Polygon"} {
		if hasFontSize(k) {
			t.Errorf("%s should not have a font size", k)
		}
	}
}

// Saves a text block at a chosen size, reloads, and confirms both the
// recorded size and — via MuPDF's real render of the saved file — that the
// size actually reaches the page (bigger preset paints visibly more ink).
func TestSaveHighlights_FreeTextFontSizeRoundTripsAndRenders(t *testing.T) {
	render := func(size float64) (int, float64) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.pdf")
		writeMinimalPDF(t, path)
		doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
		h := doc.AddTextShape(1, [4]float64{10, 10, 190, 190}, nil, "Hello text size", [3]float64{0, 0, 0})
		h.FontSize = size
		if err := doc.SaveHighlights(path); err != nil {
			t.Fatalf("SaveHighlights: %v", err)
		}
		re, err := Prepare(path)
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		defer re.Close()
		if len(re.Highlights) != 1 {
			t.Fatalf("expected 1 highlight, got %d", len(re.Highlights))
		}
		img, err := re.RenderPage(1, 2.0)
		if err != nil {
			t.Fatalf("RenderPage: %v", err)
		}
		return countNonWhitePixelsForTest(img), re.Highlights[0].FontSize
	}
	smallInk, smallSize := render(10)
	bigInk, bigSize := render(24)
	if smallSize != 10 || bigSize != 24 {
		t.Errorf("reloaded sizes = %v, %v; want 10, 24", smallSize, bigSize)
	}
	// 180x180pt box at 2x of baseRenderDPI/72 would be tens of thousands of
	// pixels if MuPDF filled it solid (the bug authoring /C caused — the
	// box was painted in the text's own color, hiding the text entirely).
	scale := (baseRenderDPI * 2.0) / 72.0
	if boxArea := int(180 * scale * 180 * scale); bigInk > boxArea/4 {
		t.Errorf("24pt text block looks solid-filled (%d non-white pixels of a %d-pixel box); /C must not fill the box", bigInk, boxArea)
	}
	if bigInk <= smallInk {
		t.Errorf("expected 24pt to paint more than 10pt in MuPDF's real render, got small=%d big=%d", smallInk, bigInk)
	}
}

// Change size on an already-saved text block, recolor it afterwards, save
// again: the size must survive both (recolor recreates the annotation, so
// FontSize has to be carried by the Highlight, not just the old dict).
func TestSaveHighlights_FontSizeChangeAndRecolorAfterSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)
	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	doc.AddTextShape(1, [4]float64{10, 10, 190, 190}, nil, "resize me", [3]float64{0, 0, 0})
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("first save: %v", err)
	}
	h := doc.Highlights[0]
	if h.FontSize != defaultFontSizePt {
		t.Fatalf("expected default size %v after first save, got %v", defaultFontSizePt, h.FontSize)
	}
	doc.SetHighlightFontSize(h, 18)
	if !fontSizeChanged(h) {
		t.Fatal("expected fontSizeChanged after SetHighlightFontSize")
	}
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("second save: %v", err)
	}
	h = doc.Highlights[0]
	if h.FontSize != 18 {
		t.Errorf("after size change FontSize = %v, want 18", h.FontSize)
	}
	doc.SetHighlightColor(h, [3]float64{1, 0, 0})
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("third save: %v", err)
	}
	if got := doc.Highlights[0].FontSize; got != 18 {
		t.Errorf("recolor lost the text size: got %v, want 18", got)
	}
	if got := doc.Highlights[0].Color; got != [3]float64{1, 0, 0} {
		t.Errorf("FreeText color after save/reload = %v, want red (read back from /DA, since /C is deliberately omitted)", got)
	}
	if len(doc.Highlights) != 1 {
		t.Errorf("expected exactly 1 highlight (no duplicates), got %d", len(doc.Highlights))
	}
}

func TestParseDAColor_RgGrayCmykAndLastWins(t *testing.T) {
	cases := []struct {
		in   string
		want [3]float64
		ok   bool
	}{
		{"/Helv 12 Tf 1 0 0 rg", [3]float64{1, 0, 0}, true},
		{"1 0 0 rg /Helvetica 14 Tf", [3]float64{1, 0, 0}, true},
		{"/Helv 12 Tf 0.5 g", [3]float64{0.5, 0.5, 0.5}, true},
		{"/Helv 12 Tf 0 1 1 0 k", [3]float64{1, 0, 0}, true},
		{"0 0 1 rg 0 1 0 rg", [3]float64{0, 1, 0}, true},
		{"/Helv 12 Tf", [3]float64{}, false},
	}
	for _, c := range cases {
		got, ok := parseDAColor(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseDAColor(%q) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
	if c, ok := parseDSColor("font: Helvetica 12.0pt; text-align:left; color: #FF0000"); !ok || c != [3]float64{1, 0, 0} {
		t.Errorf("parseDSColor = %v,%v", c, ok)
	}
}

// Real Preview-authored text block (test/ fixture): its /DA is a stale
// "0 g" at 12pt, while the baked appearance stream draws it red at ~13.5pt.
// The loader must report what's actually shown.
func TestLoadHighlights_PreviewFreeTextColorAndSizeFromAppearance(t *testing.T) {
	path := filepath.Join("..", "..", "..", "test", "TWRA-TN-Hunting-Guide_2026-27.-bookmarkTEST-bookmarked.pdf")
	if _, err := os.Stat(path); err != nil {
		t.Skip("fixture not present:", err)
	}
	hs, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights: %v", err)
	}
	for _, h := range hs {
		if h.Kind != "FreeText" || h.Contents != "Text is here!" {
			continue
		}
		if h.Color[0] < 0.9 || h.Color[1] > 0.4 || h.Color[2] > 0.4 {
			t.Errorf("Color = %v, want the red the appearance stream paints", h.Color)
		}
		if h.FontSize < 13 || h.FontSize > 14 {
			t.Errorf("FontSize = %v, want about 13.5", h.FontSize)
		}
		return
	}
	t.Skip("fixture has no 'Text is here!' FreeText")
}

func TestParseAPTextSize(t *testing.T) {
	c := "q 1 0 0 -1 0 0 cm BT 13.54433 0 0 -13.54433 194.7 652.2 Tm /TT1 1 Tf [ (T) ] TJ ET Q"
	if got := parseAPTextSize(c); got != 13.5 {
		t.Errorf("parseAPTextSize = %v, want 13.5", got)
	}
	if got := parseAPTextSize("BT /F1 18 Tf (x) Tj ET"); got != 18 {
		t.Errorf("plain Tf = %v, want 18", got)
	}
}

func TestGrowFreeTextToFit(t *testing.T) {
	// A snug 20pt-tall box with 24pt text must grow downward only.
	h := &Highlight{Kind: "FreeText", Rect: [4]float64{100, 600, 226, 620}, Contents: "Text is here!", FontSize: 24}
	growFreeTextToFit(h)
	if h.Rect[3] != 620 || h.Rect[0] != 100 || h.Rect[2] != 226 {
		t.Errorf("top/left/right must not move: %v", h.Rect)
	}
	if h.Rect[3]-h.Rect[1] < 24 {
		t.Errorf("box should now be tall enough for 24pt text, got height %v", h.Rect[3]-h.Rect[1])
	}
	// A roomy box is left alone.
	roomy := &Highlight{Kind: "FreeText", Rect: [4]float64{0, 0, 300, 300}, Contents: "short", FontSize: 12}
	growFreeTextToFit(roomy)
	if roomy.Rect != [4]float64{0, 0, 300, 300} {
		t.Errorf("roomy box changed: %v", roomy.Rect)
	}
	// Long text at a narrow width wraps to several lines and grows more.
	narrow := &Highlight{Kind: "FreeText", Rect: [4]float64{0, 100, 80, 120}, Contents: "several words that must wrap onto lines", FontSize: 12}
	growFreeTextToFit(narrow)
	if narrow.Rect[3]-narrow.Rect[1] < 40 {
		t.Errorf("wrapped text should need more height, got %v", narrow.Rect[3]-narrow.Rect[1])
	}
}

func TestSetHighlightFontSize_GrowsBoxAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)
	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	doc.AddTextShape(1, [4]float64{20, 150, 146, 170}, nil, "Text is here!", [3]float64{1, 0, 0})
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatal(err)
	}
	h := doc.Highlights[0]
	doc.SetHighlightFontSize(h, 24)
	if h.Rect[3]-h.Rect[1] < 24 || !h.geometryMoved {
		t.Fatalf("expected the box to grow and be flagged for saving: %v moved=%v", h.Rect, h.geometryMoved)
	}
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatal(err)
	}
	if got := doc.Highlights[0].Rect; got[3]-got[1] < 24 {
		t.Errorf("grown box not persisted: %v", got)
	}
}
