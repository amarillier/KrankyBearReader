package pdf

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// fyneColorToRGB converts any color.Color (the color picker dialog hands
// back whatever concrete type its swatches use) into our internal 0..1 RGB
// representation (Highlight.Color's own convention, matching a PDF /C
// entry). Converts through NRGBA specifically because color.Color.RGBA()
// returns alpha-premultiplied components — going through NRGBAModel first
// un-premultiplies correctly regardless of the input's concrete type.
// Alpha itself is dropped: a highlight's on-page translucency is this
// app's own fixed highlightOverlayAlpha, not something PDF's /C supports.
func fyneColorToRGB(c color.Color) [3]float64 {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return [3]float64{float64(n.R) / 255, float64(n.G) / 255, float64(n.B) / 255}
}

// rgbToFyneColor is fyneColorToRGB's inverse, used to seed the color picker
// with a highlight's (or the toolbar's "next new highlight") current color.
func rgbToFyneColor(rgb [3]float64) color.Color {
	return color.NRGBA{
		R: uint8(rgb[0] * 255),
		G: uint8(rgb[1] * 255),
		B: uint8(rgb[2] * 255),
		A: 255,
	}
}

// showHighlightColorPicker opens Fyne's built-in color picker in Advanced
// mode (full RGB, not just its small preset swatch grid — a highlighter
// isn't limited to a handful of colors), seeded with current, and converts
// whatever gets picked back to our internal representation before calling
// onPicked. Used both by the toolbar's "next new highlight" swatch and the
// Highlights panel's Change Color button.
func showHighlightColorPicker(win fyne.Window, current [3]float64, onPicked func(rgb [3]float64)) {
	d := dialog.NewColorPicker("Highlight Color", "Choose a highlight color", func(c color.Color) {
		if c == nil {
			return // dialog dismissed without picking
		}
		onPicked(fyneColorToRGB(c))
	}, win)
	d.Advanced = true
	d.SetColor(rgbToFyneColor(current))
	d.Show()
}

// colorSwatch is a small tappable, fixed-size color square — the toolbar's
// "next new highlight will be this color" indicator. Deliberately minimal
// (a bare canvas.Rectangle, no label): widget.Button has no way to give it
// an arbitrary fill color, since Importance only selects from the current
// theme's own palette.
type colorSwatch struct {
	widget.BaseWidget
	rect *canvas.Rectangle

	OnTapped func()
}

func newColorSwatch(c color.Color) *colorSwatch {
	s := &colorSwatch{rect: canvas.NewRectangle(c)}
	s.rect.StrokeColor = color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	s.rect.StrokeWidth = 1
	s.ExtendBaseWidget(s)
	return s
}

func (s *colorSwatch) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.rect)
}

func (s *colorSwatch) MinSize() fyne.Size { return fyne.NewSize(24, 24) }

func (s *colorSwatch) Tapped(*fyne.PointEvent) {
	if s.OnTapped != nil {
		s.OnTapped()
	}
}

// SetColor updates the swatch's fill in place.
func (s *colorSwatch) SetColor(c color.Color) {
	s.rect.FillColor = c
	s.rect.Refresh()
}
