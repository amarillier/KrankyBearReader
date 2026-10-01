package pdf

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestDrawPrefs_FallBackToDefaultsWhenNothingSaved(t *testing.T) {
	_ = test.NewApp()

	if got := loadDefaultHighlightColor(); got != defaultHighlightColor {
		t.Errorf("expected fallback to defaultHighlightColor %v, got %v", defaultHighlightColor, got)
	}
	if got := loadDefaultLineWidthPt(); got != defaultShapeBorderWidthPt {
		t.Errorf("expected fallback to defaultShapeBorderWidthPt %v, got %v", defaultShapeBorderWidthPt, got)
	}
}

func TestDrawPrefs_SaveThenLoadRoundTrips(t *testing.T) {
	_ = test.NewApp()

	want := [3]float64{0.25, 0.5, 0.75}
	saveDefaultHighlightColor(want)
	if got := loadDefaultHighlightColor(); got != want {
		t.Errorf("expected color %v, got %v", want, got)
	}

	saveDefaultLineWidthPt(5)
	if got := loadDefaultLineWidthPt(); got != 5 {
		t.Errorf("expected line width 5, got %v", got)
	}
}
