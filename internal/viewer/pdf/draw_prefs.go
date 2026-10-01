package pdf

import "fyne.io/fyne/v2"

// Remembers the last color/line-weight a user picked for drawing a new
// shape/highlight across app launches — a real, if small, convenience
// requested directly ("I don't want every new highlight to start back at
// yellow/1pt every time"), rather than always resetting to
// defaultHighlightColor/defaultShapeBorderWidthPt. Stored as plain floats
// via fyne.CurrentApp().Preferences(), the same mechanism main.go's own
// window-size persistence and Manager's recent-files/last-page use.

const (
	prefHighlightColorR = "reader.highlightColor.r"
	prefHighlightColorG = "reader.highlightColor.g"
	prefHighlightColorB = "reader.highlightColor.b"
	prefLineWidthPt     = "reader.lineWidthPt"
)

func loadDefaultHighlightColor() [3]float64 {
	prefs := fyne.CurrentApp().Preferences()
	r := prefs.FloatWithFallback(prefHighlightColorR, defaultHighlightColor[0])
	g := prefs.FloatWithFallback(prefHighlightColorG, defaultHighlightColor[1])
	b := prefs.FloatWithFallback(prefHighlightColorB, defaultHighlightColor[2])
	return [3]float64{r, g, b}
}

func saveDefaultHighlightColor(rgb [3]float64) {
	prefs := fyne.CurrentApp().Preferences()
	prefs.SetFloat(prefHighlightColorR, rgb[0])
	prefs.SetFloat(prefHighlightColorG, rgb[1])
	prefs.SetFloat(prefHighlightColorB, rgb[2])
}

func loadDefaultLineWidthPt() float64 {
	return fyne.CurrentApp().Preferences().FloatWithFallback(prefLineWidthPt, defaultShapeBorderWidthPt)
}

func saveDefaultLineWidthPt(widthPt float64) {
	fyne.CurrentApp().Preferences().SetFloat(prefLineWidthPt, widthPt)
}
