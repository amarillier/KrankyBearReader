package pdf

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// defaultFontSizePt is the text size a FreeText annotation gets when none
// is chosen, and what a loaded one reports when its /DA has no Tf operator.
const defaultFontSizePt = 12.0

// fontSizePresets are the text sizes (PDF points) offered for Text/Speech
// Bubble annotations — a closed list for the same reason lineWeightPresets
// is: a free-form entry could produce unreadable or absurd text.
var fontSizePresets = []float64{10, 12, 14, 18, 24}

func fontSizeLabel(sizePt float64) string { return fmt.Sprintf("%gpt", sizePt) }

func parseFontSizeLabel(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "pt"), 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

func fontSizeOptions() []string {
	opts := make([]string, len(fontSizePresets))
	for i, s := range fontSizePresets {
		opts[i] = fontSizeLabel(s)
	}
	return opts
}

// hasFontSize reports whether kind's FontSize field is meaningful — only
// FreeText (text block / speech bubble) has text of its own to size.
func hasFontSize(kind string) bool { return kind == "FreeText" }

// fontSizeChanged reports whether h's text size has diverged from how it
// was loaded. A separate predicate from highlightDictChanges (rather than
// a fifth return value) so that function's many call sites stay untouched;
// callers OR it in alongside lineWidthChanged, since it needs the same
// delete-and-recreate treatment: pdfcpu never writes a Tf for FreeText, so
// the size lives in a /DA this app patches in after authoring (see
// applyFreeTextFontSize), and a bare /DA rewrite wouldn't update a baked /AP.
func fontSizeChanged(h *Highlight) bool {
	return hasFontSize(h.Kind) && h.FontSize != h.origFontSize
}

// freeTextStyleChanged is fontSizeChanged plus a border change: both live in
// fields a bare dict rewrite can't make visible over a baked /AP, so both
// take the delete-and-recreate path.
func freeTextStyleChanged(h *Highlight) bool {
	return fontSizeChanged(h) || (hasFontSize(h.Kind) && h.Bordered != h.origBordered)
}

// freeTextBordered reports whether an existing FreeText draws a border: a
// /CL (callout) always does, an explicit /BS decides by its width, and with no
// /BS a baked /AP means the other app's own appearance (assumed borderless,
// like Preview's Text tool) while no /AP means MuPDF's default 1pt border.
func freeTextBordered(xr *model.XRefTable, objNr int) bool {
	d, err := xr.DereferenceDict(*types.NewIndirectRef(objNr, 0))
	if err != nil || d == nil {
		return true
	}
	if _, ok := d["CL"]; ok {
		return true
	}
	if _, ok := d["BS"]; ok {
		return parseBorderWidth(xr, d, defaultShapeBorderWidthPt) > 0
	}
	_, hasAP := d["AP"]
	return !hasAP
}

var daFontSizeRe = regexp.MustCompile(`([0-9]*\.?[0-9]+)\s+Tf`)

// parseDAFontSize extracts the size from a /DA string such as
// "/Helv 14 Tf 0 0 0 rg", returning defaultFontSizePt when absent or 0
// (0 means "auto" in the PDF spec).
func parseDAFontSize(da string) float64 {
	if m := daFontSizeRe.FindStringSubmatch(da); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0 {
			return v
		}
	}
	return defaultFontSizePt
}

// parseDAColor extracts the text fill color from a /DA string: the last
// "r g b rg", "g g" or "c m y k k" operator wins, as in a content stream.
// Gray and CMYK are converted to RGB (other apps — Preview among them —
// write either, not just rg).
func parseDAColor(da string) ([3]float64, bool) {
	var nums []float64
	var out [3]float64
	found := false
	for _, tok := range strings.Fields(da) {
		if v, err := strconv.ParseFloat(tok, 64); err == nil {
			nums = append(nums, v)
			continue
		}
		n := len(nums)
		switch tok {
		case "rg":
			if n >= 3 {
				out, found = [3]float64{nums[n-3], nums[n-2], nums[n-1]}, true
			}
		case "g":
			if n >= 1 {
				out, found = [3]float64{nums[n-1], nums[n-1], nums[n-1]}, true
			}
		case "sc", "scn":
			switch {
			case n >= 4:
				c, m, y, k := nums[n-4], nums[n-3], nums[n-2], nums[n-1]
				out, found = [3]float64{(1 - c) * (1 - k), (1 - m) * (1 - k), (1 - y) * (1 - k)}, true
			case n >= 3:
				out, found = [3]float64{nums[n-3], nums[n-2], nums[n-1]}, true
			case n >= 1:
				out, found = [3]float64{nums[n-1], nums[n-1], nums[n-1]}, true
			}
		case "k":
			if n >= 4 {
				c, m, y, k := nums[n-4], nums[n-3], nums[n-2], nums[n-1]
				out, found = [3]float64{(1 - c) * (1 - k), (1 - m) * (1 - k), (1 - y) * (1 - k)}, true
			}
		}
		nums = nums[:0]
	}
	return out, found
}

var dsColorRe = regexp.MustCompile(`(?i)(?:^|[;\s])color:\s*#([0-9a-f]{6})`)

// parseDSColor reads "color: #RRGGBB" from a /DS default-style string,
// where some apps keep the text color instead of /DA.
func parseDSColor(ds string) ([3]float64, bool) {
	m := dsColorRe.FindStringSubmatch(ds)
	if m == nil {
		return [3]float64{}, false
	}
	v, err := strconv.ParseUint(m[1], 16, 32)
	if err != nil {
		return [3]float64{}, false
	}
	return [3]float64{float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255}, true
}

// parseAPTextSize returns the effective text size of the last text object
// in an appearance-stream's content: the Tf size times the Tm matrix scale
// (Preview writes "/TT1 1 Tf" with the real size in Tm). 0 if none found.
func parseAPTextSize(content string) float64 {
	var nums []float64
	size, scale := 0.0, 1.0
	for _, tok := range strings.Fields(content) {
		if v, err := strconv.ParseFloat(tok, 64); err == nil {
			nums = append(nums, v)
			continue
		}
		switch tok {
		case "Tf":
			if len(nums) >= 1 {
				size = nums[len(nums)-1]
			}
		case "Tm":
			if len(nums) >= 6 {
				scale = math.Hypot(nums[len(nums)-6], nums[len(nums)-5])
			}
		}
		nums = nums[:0]
	}
	if size <= 0 {
		return 0
	}
	return math.Round(size*scale*10) / 10
}

// freeTextAppearanceContent returns the decoded content stream of d's
// normal appearance (/AP /N), or "" if it has none / isn't a stream.
func freeTextAppearanceContent(xRefTable *model.XRefTable, d types.Dict) string {
	ap, err := xRefTable.DereferenceDict(d["AP"])
	if err != nil || ap == nil {
		return ""
	}
	o, err := xRefTable.Dereference(ap["N"])
	if err != nil {
		return ""
	}
	sd, ok := o.(types.StreamDict)
	if !ok {
		return ""
	}
	if err := sd.Decode(); err != nil {
		return ""
	}
	return string(sd.Content)
}

// freeTextStyle reads a FreeText annotation's text size and color. Size
// comes from /DA's Tf. Color is the text color: /DA's fill operator, else
// /DS's "color:", else (when the dict has no /C either) black — never the
// yellow highlighter default, which is what an unreadable color used to
// fall back to. /C is only used when nothing above yields a color, since
// other apps use it for the box's background, not the text. colorOK is
// false when the caller should keep its own /C-derived color.
func freeTextStyle(xRefTable *model.XRefTable, objNr int) (sizePt float64, col [3]float64, colorOK bool) {
	sizePt = defaultFontSizePt
	if objNr <= 0 {
		return
	}
	d, err := xRefTable.DereferenceDict(*types.NewIndirectRef(objNr, 0))
	if err != nil || d == nil {
		return
	}
	// A baked appearance stream is what every viewer actually shows, and
	// Preview's /DA is stale next to it (its text can be red/13.5pt while
	// /DA still says "0 g" and 12) — so it wins when it has the answer.
	apColor, apColorOK, apSize := [3]float64{}, false, 0.0
	if content := freeTextAppearanceContent(xRefTable, d); content != "" {
		apColor, apColorOK = parseDAColor(content)
		apSize = parseAPTextSize(content)
	}
	if da, ok := d["DA"].(types.StringLiteral); ok {
		sizePt = parseDAFontSize(string(da))
	}
	if apSize > 0 {
		sizePt = apSize
	}
	if apColorOK {
		return sizePt, apColor, true
	}
	if da, ok := d["DA"].(types.StringLiteral); ok {
		if c, ok := parseDAColor(string(da)); ok {
			return sizePt, c, true
		}
	}
	if ds, ok := d["DS"].(types.StringLiteral); ok {
		if c, ok := parseDSColor(string(ds)); ok {
			return sizePt, c, true
		}
	}
	if _, hasC := d["C"]; !hasC {
		return sizePt, [3]float64{0, 0, 0}, true
	}
	return
}

// applyFreeTextFontSize prepends a Tf operator to the freshly-added
// annotation's /DA. pdfcpu's FreeTextAnnotation.RenderDict ignores its own
// fontSize parameter ("TODO Implement Tf operator" in its source), writing
// only the color, so this is the only way to get a size into the file.
// Helv is one of the standard 14 fonts MuPDF resolves by name.
func applyFreeTextFontSize(ctx *model.Context, ref *types.IndirectRef, sizePt float64) error {
	d, err := ctx.XRefTable.DereferenceDict(*ref)
	if err != nil || d == nil {
		return fmt.Errorf("reading new FreeText annotation: %v", err)
	}
	da, _ := d["DA"].(types.StringLiteral)
	d.Update("DA", types.StringLiteral(fmt.Sprintf("/Helv %g Tf%s", sizePt, string(da))))
	return nil
}

// applyFreeTextNoBorder writes an explicit /BS << /W 0 >> on a borderless
// text block. pdfcpu omits /BS at width 0, and an absent /BS means "1pt default"
// to MuPDF, so the explicit zero has to be patched in after adding.
func applyFreeTextNoBorder(ctx *model.Context, ref *types.IndirectRef) error {
	d, err := ctx.XRefTable.DereferenceDict(*ref)
	if err != nil || d == nil {
		return fmt.Errorf("reading new FreeText annotation: %v", err)
	}
	d.Update("BS", types.Dict{"Type": types.Name("Border"), "W": types.Float(0), "S": types.Name("S")})
	return nil
}

var (
	previewFontOnce sync.Once
	previewFont     *opentype.Font
)

// previewFace returns a scalable face at sizePx pixels for the hand-paint
// preview. Go Regular stands in for the Helvetica MuPDF will use.
func previewFace(sizePx float64) (font.Face, error) {
	previewFontOnce.Do(func() { previewFont, _ = opentype.Parse(goregular.TTF) })
	if previewFont == nil {
		return nil, fmt.Errorf("parsing preview font")
	}
	return opentype.NewFace(previewFont, &opentype.FaceOptions{Size: sizePx, DPI: 72})
}

// growFreeTextToFit enlarges h's box DOWNWARD (the top edge stays put) when
// its text, wrapped at the current width and size, needs more height than
// the box has — best effort, from Go Regular's metrics rather than
// MuPDF's Helvetica, with a little slack. Never shrinks, never widens, so
// a deliberately roomy box is left alone. The user can still resize it.
func growFreeTextToFit(h *Highlight) {
	if !hasFontSize(h.Kind) || h.Rect == [4]float64{} || h.Contents == "" || h.FontSize <= 0 {
		return
	}
	face, err := previewFace(h.FontSize) // size in px == pt at 72 DPI
	if err != nil {
		return
	}
	defer face.Close()
	const pad = 4.0 // border + inner margin, per side, in points
	maxWidth := int(h.Rect[2] - h.Rect[0] - 2*pad)
	if maxWidth <= 0 {
		return
	}
	lines := wrapText(h.Contents, &font.Drawer{Face: face}, maxWidth)
	lineH := float64(face.Metrics().Height.Ceil())
	need := float64(len(lines))*lineH + 2*pad
	if have := h.Rect[3] - h.Rect[1]; need > have {
		h.Rect[1] = h.Rect[3] - need
	}
}
