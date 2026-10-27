package pdf

import (
	"image"
	"image/color"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// textShapePaddingPx pads the wrapped text away from a FreeText shape's
// own border on every side — a bare box-edge-to-glyph-edge text block
// looks cramped even at a coarse bitmap font size.
const textShapePaddingPx = 4

// drawFreeTextPreview hand-paints a freshly-drawn (ObjNr == 0), not-yet-
// saved FreeText shape (a plain text block, or a speech bubble when
// CalloutTip is set — see AddTextShape) for paintHighlights' own
// needsHandPaint gate: the box border (reusing the generic
// drawRectOutline, same as Square's own stroke), the callout line/tail
// when present, and the actual caption text wrapped inside the box —
// this app's first hand-painted TEXT, not just geometry, so an already-
// saved FreeText still relies entirely on MuPDF's own rendering; this is
// preview-only.
//
// A plain text block (no CalloutTip) is borderless, matching Preview's own
// "Text" tool: SaveHighlights writes an explicit /BS /W 0 for it
// (applyFreeTextNoBorder), which MuPDF honours — only an ABSENT /BS falls
// back to a 1pt border. A speech bubble keeps its border, since the box is
// the bubble's own outline.
func drawFreeTextPreview(img *image.RGBA, r image.Rectangle, h *Highlight, pageHeightPt, scale float64, col color.NRGBA) {
	// Always defaultShapeBorderWidthPt, never h.LineWidth — FreeText is
	// deliberately excluded from hasLineWidth (see its own doc comment):
	// a configurable width here would silently diverge from what Save
	// produces (fixed 1pt for a bubble, none for a plain block).
	strokePx := float64(shapeStrokeWidthPx(defaultShapeBorderWidthPt, scale))
	if h.Bordered || h.CalloutTip != nil {
		drawRectOutline(img, r, col, int(strokePx))
	}
	if h.CalloutTip != nil {
		anchor := [2]float64{float64(r.Min.X), float64(r.Max.Y)} // the box's own bottom-left corner, matching calloutTipFor's own anchor choice
		tip := [2]float64{h.CalloutTip[0] * scale, (pageHeightPt - h.CalloutTip[1]) * scale}
		strokeLine(img, anchor, tip, strokePx, col)
	}
	size := h.FontSize
	if size <= 0 {
		size = defaultFontSizePt
	}
	drawWrappedText(img, r, h.Contents, col, size*scale)
}

// drawWrappedText paints text word-wrapped to fit within r (padded by
// textShapePaddingPx on every side), using golang.org/x/image's own
// a scalable Go Regular face at sizePx pixels (previewFace; falls back to
// basicfont.Face7x13 if it can't be built), good enough for a
// draw-time preview; the real, saved annotation's own rendering (once
// MuPDF paints it from the authored /Contents+/DA) isn't tied to this
// font choice at all. Silently clips any lines past r's own bottom edge
// rather than growing the box or shrinking the font — a real box-sizing/
// auto-fit feature is edit-UI scope, not initial drawing.
func drawWrappedText(img *image.RGBA, r image.Rectangle, text string, col color.NRGBA, sizePx float64) {
	if text == "" {
		return
	}
	var face font.Face = basicfont.Face7x13
	if f, err := previewFace(sizePx); err == nil {
		face = f
		defer f.Close()
	}
	metrics := face.Metrics()
	lineHeight := metrics.Height.Ceil()
	ascent, descent := metrics.Ascent.Ceil(), metrics.Descent.Ceil()
	maxWidth := r.Dx() - 2*textShapePaddingPx
	if maxWidth <= 0 {
		return
	}

	drawer := &font.Drawer{Dst: img, Src: image.NewUniform(col), Face: face}
	y := r.Min.Y + textShapePaddingPx + ascent
	for i, line := range wrapText(text, drawer, maxWidth) {
		// The first line always draws, even when it doesn't fit: MuPDF's
		// own rendering lets text overflow a too-short box rather than
		// dropping it, and a preview that shows an empty box (as it did for
		// an 18pt size change on a 20pt-tall Preview text block) reads as
		// "the text vanished".
		if i > 0 && y+descent > r.Max.Y-textShapePaddingPx {
			break
		}
		drawer.Dot = fixed.Point26_6{X: fixed.I(r.Min.X + textShapePaddingPx), Y: fixed.I(y)}
		drawer.DrawString(line)
		y += lineHeight
	}
}

// wrapText greedily packs text's words into lines no wider than maxWidth
// (measured via drawer's own face, so wrapping always matches what
// drawWrappedText is about to actually draw), preserving the caller's own
// newlines as paragraph breaks. A single word wider than maxWidth on its
// own is kept whole, not hyphenated or clipped mid-word — this is a
// preview, not a layout engine.
func wrapText(text string, drawer *font.Drawer, maxWidth int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			candidate := line + " " + w
			if drawer.MeasureString(candidate).Round() > maxWidth {
				lines = append(lines, line)
				line = w
			} else {
				line = candidate
			}
		}
		lines = append(lines, line)
	}
	return lines
}
