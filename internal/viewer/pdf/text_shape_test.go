package pdf

import (
	"image"
	"image/color"
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
	drawWrappedText(img, image.Rect(0, 0, 100, 100), "", color.NRGBA{R: 255, A: 255})
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
	drawWrappedText(img, r, "Hello there", color.NRGBA{R: 0, G: 0, B: 0, A: 255})

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

// TestDrawFreeTextPreview_PlainTextBlockStillHasBorder confirms the
// preview deliberately keeps a plain text block's border, even though it
// doesn't match Preview's own borderless style — see
// drawFreeTextPreview's own doc comment for why: MuPDF's real rendering
// of the saved annotation draws one unconditionally regardless of /BS,
// so a borderless preview would just be a different, more confusing
// inconsistency (looks one way before Save, a different way after) than
// staying consistent with what Save will actually produce.
func TestDrawFreeTextPreview_PlainTextBlockStillHasBorder(t *testing.T) {
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
	if !borderPainted {
		t.Errorf("expected a border painted along the top edge even for a plain text block")
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
