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
// Always draws the border, even for a plain text block — see
// newAnnotationForShape's own doc comment on why: Preview's own
// borderless "Text" tool works by baking a custom appearance stream with
// no border path in it at all, and MuPDF's own default-appearance
// synthesis (what this app's authored, no-baked-/AP FreeText relies on)
// draws a border unconditionally, confirmed empirically to ignore /BS's
// width (0 or absent both still show one) — there's no parameter to omit
// it via the authoring path pdfcpu exposes. Keeping the preview
// borderED here too, rather than borderless, is deliberate: a borderless
// preview that gains a border the instant Save runs (once MuPDF's own
// rendering takes over) would be a worse, more confusing inconsistency
// than just not matching Preview's own style at all.
func drawFreeTextPreview(img *image.RGBA, r image.Rectangle, h *Highlight, pageHeightPt, scale float64, col color.NRGBA) {
	strokePx := float64(defaultShapeStrokeWidthPx(scale))
	drawRectOutline(img, r, col, int(strokePx))
	if h.CalloutTip != nil {
		anchor := [2]float64{float64(r.Min.X), float64(r.Max.Y)} // the box's own bottom-left corner, matching calloutTipFor's own anchor choice
		tip := [2]float64{h.CalloutTip[0] * scale, (pageHeightPt - h.CalloutTip[1]) * scale}
		strokeLine(img, anchor, tip, strokePx, col)
	}
	drawWrappedText(img, r, h.Contents, col)
}

// drawWrappedText paints text word-wrapped to fit within r (padded by
// textShapePaddingPx on every side), using golang.org/x/image's own
// basicfont.Face7x13 — a fixed, already-available bitmap font (Fyne
// itself depends on golang.org/x/image transitively), good enough for a
// draw-time preview; the real, saved annotation's own rendering (once
// MuPDF paints it from the authored /Contents+/DA) isn't tied to this
// font choice at all. Silently clips any lines past r's own bottom edge
// rather than growing the box or shrinking the font — a real box-sizing/
// auto-fit feature is edit-UI scope, not initial drawing.
func drawWrappedText(img *image.RGBA, r image.Rectangle, text string, col color.NRGBA) {
	if text == "" {
		return
	}
	face := basicfont.Face7x13
	lineHeight := face.Height
	maxWidth := r.Dx() - 2*textShapePaddingPx
	if maxWidth <= 0 {
		return
	}

	drawer := &font.Drawer{Dst: img, Src: image.NewUniform(col), Face: face}
	y := r.Min.Y + textShapePaddingPx + face.Ascent
	for _, line := range wrapText(text, drawer, maxWidth) {
		if y+face.Descent > r.Max.Y-textShapePaddingPx {
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
