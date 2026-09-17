package pdf

import "fyne.io/fyne/v2"

// pagesLayout stacks a fixed number of uniformly-sized page slots vertically
// with no gaps, centered horizontally. Ported from pdfviewer's ui/viewer.go
// pagesLayout: total scrollable height is exactly rowH*len(objects), which
// is what makes the scroll-offset -> page-index math in lazyRenderVisible
// exact division rather than an approximation.
type pagesLayout struct {
	rowW, rowH float32
}

func (l *pagesLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.rowW, l.rowH*float32(len(objects)))
}

func (l *pagesLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	x := (size.Width - l.rowW) / 2
	if x < 0 {
		x = 0
	}
	for i, obj := range objects {
		obj.Move(fyne.NewPos(x, float32(i)*l.rowH))
		obj.Resize(fyne.NewSize(l.rowW, l.rowH))
	}
}

// resizeReportingLayout wraps a single child, filling the available space
// and invoking onResize whenever the container's size changes. Used to
// detect viewport-size changes for live Fit Width/Fit Page rescaling on
// window resize (a minimal version of pdfviewer's "fitWatcher" layout).
type resizeReportingLayout struct {
	onResize func(fyne.Size)
	lastSize fyne.Size
}

func (l *resizeReportingLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(1, 1)
}

func (l *resizeReportingLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, obj := range objects {
		obj.Resize(size)
		obj.Move(fyne.NewPos(0, 0))
	}
	if size != l.lastSize {
		l.lastSize = size
		if l.onResize != nil {
			l.onResize(size)
		}
	}
}
