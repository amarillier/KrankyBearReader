package pdf

import (
	"fmt"
	"strconv"
	"strings"
)

// lineWeightPresets are the stroke/border widths (PDF user-space points)
// offered by both the toolbar's "next new shape" selector and the
// Highlights panel's "Change Line Weight" dialog — a plain widget.Select
// of preset values, the same "pick from a short list" pattern zoomSelect
// already uses for zoom levels, rather than a free-form numeric entry
// a mis-typed value could turn into an invisible (0pt) or absurd
// (negative, or huge) border.
var lineWeightPresets = []float64{1, 2, 3, 5, 8}

// lineWeightLabel/parseLineWeightLabel convert a preset width to/from its
// own toolbar/dialog display string ("1pt", "2pt", ...) — %g rather than a
// fixed decimal count so every current preset (all whole numbers) shows
// without a trailing ".0", while still handling a fractional preset
// correctly if one's ever added.
func lineWeightLabel(widthPt float64) string {
	return fmt.Sprintf("%gpt", widthPt)
}

func parseLineWeightLabel(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "pt"), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// lineWeightOptions is lineWeightPresets' own display-string list, for
// widget.NewSelect.
func lineWeightOptions() []string {
	opts := make([]string, len(lineWeightPresets))
	for i, w := range lineWeightPresets {
		opts[i] = lineWeightLabel(w)
	}
	return opts
}
