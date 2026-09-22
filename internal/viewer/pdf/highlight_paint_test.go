package pdf

import (
	"image"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestParseQuadPoints_ValidArray(t *testing.T) {
	xRefTable := &model.XRefTable{}
	arr := types.Array{
		types.Float(0), types.Float(10), types.Float(100), types.Float(10),
		types.Float(0), types.Float(0), types.Float(100), types.Float(0),
	}
	got := parseQuadPoints(xRefTable, arr)
	if len(got) != 1 {
		t.Fatalf("expected 1 quad, got %d", len(got))
	}
	want := [8]float64{0, 10, 100, 10, 0, 0, 100, 0}
	if got[0] != want {
		t.Errorf("quad = %v, want %v", got[0], want)
	}
}

func TestParseQuadPoints_MultipleQuads(t *testing.T) {
	xRefTable := &model.XRefTable{}
	arr := make(types.Array, 16)
	for i := range arr {
		arr[i] = types.Float(i)
	}
	got := parseQuadPoints(xRefTable, arr)
	if len(got) != 2 {
		t.Fatalf("expected 2 quads, got %d", len(got))
	}
}

func TestParseQuadPoints_RejectsWrongLength(t *testing.T) {
	xRefTable := &model.XRefTable{}
	cases := []types.Array{
		nil,
		{},
		{types.Float(1), types.Float(2), types.Float(3)}, // not a multiple of 8
	}
	for _, c := range cases {
		if got := parseQuadPoints(xRefTable, c); got != nil {
			t.Errorf("parseQuadPoints(%v) = %v, want nil", c, got)
		}
	}
}

func TestParseColor_DeviceGray(t *testing.T) {
	xRefTable := &model.XRefTable{}
	got := parseColor(xRefTable, types.Array{types.Float(0.5)})
	want := [3]float64{0.5, 0.5, 0.5}
	if got != want {
		t.Errorf("parseColor(gray) = %v, want %v", got, want)
	}
}

func TestParseColor_DeviceRGB(t *testing.T) {
	xRefTable := &model.XRefTable{}
	got := parseColor(xRefTable, types.Array{types.Float(1), types.Float(0), types.Float(0)})
	want := [3]float64{1, 0, 0}
	if got != want {
		t.Errorf("parseColor(rgb) = %v, want %v", got, want)
	}
}

func TestParseColor_DeviceCMYK(t *testing.T) {
	xRefTable := &model.XRefTable{}
	// Pure black in CMYK (K=1) should come out as RGB black.
	got := parseColor(xRefTable, types.Array{types.Float(0), types.Float(0), types.Float(0), types.Float(1)})
	want := [3]float64{0, 0, 0}
	if got != want {
		t.Errorf("parseColor(cmyk black) = %v, want %v", got, want)
	}
}

func TestParseColor_FallsBackToDefaultOnUnknownLength(t *testing.T) {
	xRefTable := &model.XRefTable{}
	got := parseColor(xRefTable, types.Array{types.Float(1), types.Float(1)})
	if got != defaultHighlightColor {
		t.Errorf("parseColor(2 components) = %v, want default %v", got, defaultHighlightColor)
	}
}

func TestCmykToRGB(t *testing.T) {
	// Pure cyan (C=1, everything else 0) has no red.
	got := cmykToRGB(1, 0, 0, 0)
	want := [3]float64{0, 1, 1}
	if got != want {
		t.Errorf("cmykToRGB(cyan) = %v, want %v", got, want)
	}
}

func TestQuadPixelRect_FlipsYAndScales(t *testing.T) {
	// A quad occupying the top-left inch of a 10x10-point page, at 2x scale
	// (144 DPI): PDF y is measured from the bottom, so the top of the page
	// (near y=10) should map to pixel y near 0.
	q := [8]float64{0, 9, 1, 9, 0, 10, 1, 10}
	got := quadPixelRect(q, 10, 2)
	want := image.Rect(0, 0, 2, 2)
	if got != want {
		t.Errorf("quadPixelRect = %v, want %v", got, want)
	}
}

func TestHighlightGeometry_MissingObjectFallsBackGracefully(t *testing.T) {
	xRefTable := &model.XRefTable{}
	quads, col := highlightGeometry(xRefTable, 0) // objNr <= 0: no indirect ref to re-dereference
	if quads != nil {
		t.Errorf("expected nil quads for objNr<=0, got %v", quads)
	}
	if col != defaultHighlightColor {
		t.Errorf("expected default color for objNr<=0, got %v", col)
	}
}
