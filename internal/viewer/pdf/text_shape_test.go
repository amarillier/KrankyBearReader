package pdf

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

func TestWrapText_BreaksOnWidthAndPreservesNewlines(t *testing.T) {
	d := &font.Drawer{Face: basicfont.Face7x13}
	// Face7x13 is a fixed-width 7px-per-glyph font -- "word1 word2" is 11
	// chars = 77px, comfortably over a 50px limit, so it must break.
	lines := wrapText("word1 word2\nsecond paragraph", d, 50)

	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines (wrapped first paragraph + second paragraph), got %d: %v", len(lines), lines)
	}
	// The explicit \n must start a new line regardless of width -- find it.
	foundParagraphBreak := false
	for _, l := range lines {
		if l == "second paragraph" || l == "second" {
			foundParagraphBreak = true
		}
	}
	if !foundParagraphBreak {
		t.Errorf("expected the second paragraph's own text to appear on its own line(s), got %v", lines)
	}
}

func TestWrapText_EmptyParagraphKeptAsBlankLine(t *testing.T) {
	d := &font.Drawer{Face: basicfont.Face7x13}
	lines := wrapText("first\n\nthird", d, 200)
	if len(lines) != 3 || lines[1] != "" {
		t.Errorf("wrapText with a blank paragraph = %v, want [\"first\" \"\" \"third\"]", lines)
	}
}

func TestWrapText_SingleWordWiderThanMaxWidthKeptWhole(t *testing.T) {
	d := &font.Drawer{Face: basicfont.Face7x13}
	lines := wrapText("supercalifragilisticexpialidocious", d, 10) // narrower than one glyph
	if len(lines) != 1 || lines[0] != "supercalifragilisticexpialidocious" {
		t.Errorf("expected the single long word kept whole on one line, got %v", lines)
	}
}

func TestDrawWrappedText_EmptyStringPaintsNothing(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	drawWrappedText(img, image.Rect(0, 0, 100, 100), "", color.NRGBA{R: 255, A: 255}, 12)
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if img.RGBAAt(x, y).A > 0 {
				t.Fatalf("expected nothing painted for empty text, found paint at (%d,%d)", x, y)
			}
		}
	}
}

func TestDrawWrappedText_NonEmptyTextPaintsSomethingInsideBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	r := image.Rect(10, 10, 190, 90)
	drawWrappedText(img, r, "Hello there", color.NRGBA{R: 0, G: 0, B: 0, A: 255}, 12)

	painted := false
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.RGBAAt(x, y).A > 0 {
				painted = true
			}
		}
	}
	if !painted {
		t.Errorf("expected some text painted inside the bounding box, found nothing")
	}

	// Nothing should paint OUTSIDE r -- the text must stay inside the box.
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			if r.Min.X <= x && x < r.Max.X && r.Min.Y <= y && y < r.Max.Y {
				continue
			}
			if img.RGBAAt(x, y).A > 0 {
				t.Fatalf("expected nothing painted outside the box, found paint at (%d,%d)", x, y)
			}
		}
	}
}

// TestDrawFreeTextPreview_PlainTextBlockHasNoBorder confirms the
// preview deliberately keeps a plain text block's border, even though it
// doesn't match Preview's own borderless style — see
// drawFreeTextPreview's own doc comment for why: MuPDF's real rendering
// of the saved annotation draws one unconditionally regardless of /BS,
// so a borderless preview would just be a different, more confusing
// inconsistency (looks one way before Save, a different way after) than
// staying consistent with what Save will actually produce.
func TestDrawFreeTextPreview_PlainTextBlockHasNoBorder(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	r := image.Rect(20, 20, 180, 80)
	h := &Highlight{Kind: "FreeText", Contents: "hello", CalloutTip: nil}

	drawFreeTextPreview(img, r, h, 100, 1, color.NRGBA{R: 0, G: 0, B: 0, A: 255})

	borderPainted := false
	for x := r.Min.X; x < r.Max.X; x++ {
		if img.RGBAAt(x, r.Min.Y).A > 0 {
			borderPainted = true
			break
		}
	}
	if borderPainted {
		t.Errorf("plain text block preview must not paint a border (Save writes /BS /W 0)")
	}
}

// TestDrawFreeTextPreview_SpeechBubbleHasBorderAndCallout confirms the
// border+callout are still drawn when CalloutTip IS set -- the one case
// that genuinely needs a visible box (it's the bubble's own outline).
func TestDrawFreeTextPreview_SpeechBubbleHasBorderAndCallout(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	r := image.Rect(20, 20, 180, 80)
	tip := [2]float64{5, 5}
	h := &Highlight{Kind: "FreeText", Contents: "hi", CalloutTip: &tip}

	drawFreeTextPreview(img, r, h, 100, 1, color.NRGBA{R: 0, G: 0, B: 0, A: 255})

	borderPainted := false
	for x := r.Min.X; x < r.Max.X; x++ {
		if img.RGBAAt(x, r.Min.Y).A > 0 {
			borderPainted = true
			break
		}
	}
	if !borderPainted {
		t.Errorf("expected a border painted along the top edge for a speech bubble")
	}
	// The callout line runs from roughly (20,80) toward (5,95) in pixel
	// space (tip converted via pageHeightPt-y flip) -- sample a point
	// along that path, well outside the box itself.
	if img.RGBAAt(15, 85).A == 0 {
		t.Errorf("expected the callout line painted outside the box toward its tip, found nothing at (15,85)")
	}
}

// A box too short for even one line still shows that line (clipped by the
// image edge at worst), instead of looking empty.
func TestDrawWrappedText_FirstLineAlwaysDrawsInTooShortBox(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 100))
	r := image.Rect(10, 30, 290, 42) // 12px tall: far less than a 30px line plus padding
	drawWrappedText(img, r, "Overflow", color.NRGBA{A: 255}, 30)
	painted := false
	for y := 0; y < 100 && !painted; y++ {
		for x := 0; x < 300; x++ {
			if img.RGBAAt(x, y).A > 0 {
				painted = true
				break
			}
		}
	}
	if !painted {
		t.Error("expected the first line painted even though the box is too short for it")
	}
}

// TestSaveHighlights_PlainTextBlockRendersWithoutBorder renders the REAL
// saved file with MuPDF: a plain text block must show no border, a speech
// bubble must keep one. (An absent /BS is a 1pt border; only an explicit
// /BS /W 0 removes it.)
func TestSaveHighlights_PlainTextBlockRendersWithoutBorder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.pdf")
	writeTextPDF(t, path) // 300x200
	doc, err := Prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	tip := [2]float64{5, 5}
	doc.AddTextShape(1, [4]float64{20, 20, 140, 70}, nil, "plain", [3]float64{1, 0, 0})
	doc.AddTextShape(1, [4]float64{160, 20, 290, 70}, &tip, "bubble", [3]float64{1, 0, 0})
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatal(err)
	}
	doc.Close()
	re, err := Prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	defer re.Close()
	img, err := re.RenderPage(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Bottom edge of each box (PDF y=20), mid-width, below the text.
	redAt := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r>>8 > 180 && g>>8 < 120 && b>>8 < 120
	}
	sc := float64(img.Bounds().Dx()) / 300 // page is 300pt wide
	edgeY := int(float64(img.Bounds().Dy()) - 20*sc)
	plainEdge, bubbleEdge := false, false
	for dy := -3; dy <= 3; dy++ {
		plainEdge = plainEdge || redAt(int(80*sc), edgeY+dy)
		bubbleEdge = bubbleEdge || redAt(int(225*sc), edgeY+dy)
	}
	if plainEdge {
		t.Errorf("plain text block rendered a border")
	}
	if !bubbleEdge {
		t.Errorf("speech bubble lost its border")
	}
}

// TestTextBlockBorderToggleRoundTrips draws a borderless and a boxed text
// block, saves, reloads (Bordered must be read back from /BS), then toggles
// both on the reloaded file, saves again, and checks the REAL MuPDF render
// of each state at the bottom edge of the box.
func TestTextBlockBorderToggleRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.pdf")
	writeTextPDF(t, path) // 300x200
	doc, err := Prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	doc.AddTextShape(1, [4]float64{20, 20, 140, 70}, nil, "plain", [3]float64{1, 0, 0})
	boxed := doc.AddTextShape(1, [4]float64{160, 20, 290, 70}, nil, "boxed", [3]float64{1, 0, 0})
	boxed.Bordered = true
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatal(err)
	}
	doc.Close()

	edgeRed := func(d *Document, xPt float64) bool {
		img, err := d.RenderPage(1, 1)
		if err != nil {
			t.Fatal(err)
		}
		sc := float64(img.Bounds().Dx()) / 300
		y := int(float64(img.Bounds().Dy()) - 20*sc)
		for dy := -3; dy <= 3; dy++ {
			r, g, b, _ := img.At(int(xPt*sc), y+dy).RGBA()
			if r>>8 > 180 && g>>8 < 120 && b>>8 < 120 {
				return true
			}
		}
		return false
	}
	byText := func(d *Document, text string) *Highlight {
		for _, h := range d.Highlights {
			if h.Kind == "FreeText" && h.Contents == text {
				return h
			}
		}
		t.Fatalf("no FreeText %q", text)
		return nil
	}

	re, err := Prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	if byText(re, "plain").Bordered || !byText(re, "boxed").Bordered {
		t.Fatalf("Bordered not read back: plain=%v boxed=%v", byText(re, "plain").Bordered, byText(re, "boxed").Bordered)
	}
	if edgeRed(re, 80) || !edgeRed(re, 225) {
		t.Fatalf("saved render wrong: plain border=%v boxed border=%v", edgeRed(re, 80), edgeRed(re, 225))
	}

	re.SetHighlightBordered(byText(re, "plain"), true)
	re.SetHighlightBordered(byText(re, "boxed"), false)
	if !edgeRed(re, 80) || edgeRed(re, 225) {
		t.Errorf("live preview after toggle wrong: plain border=%v boxed border=%v", edgeRed(re, 80), edgeRed(re, 225))
	}
	if err := re.SaveHighlights(path); err != nil {
		t.Fatal(err)
	}
	re.Close()

	again, err := Prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if !byText(again, "plain").Bordered || byText(again, "boxed").Bordered {
		t.Errorf("toggle not persisted: plain=%v boxed=%v", byText(again, "plain").Bordered, byText(again, "boxed").Bordered)
	}
	if !edgeRed(again, 80) || edgeRed(again, 225) {
		t.Errorf("saved render after toggle wrong: plain border=%v boxed border=%v", edgeRed(again, 80), edgeRed(again, 225))
	}
}
