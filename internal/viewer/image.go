package viewer

import (
	"image"
	_ "image/gif"  // register GIF decoding for decodedImageSize's image.DecodeConfig
	_ "image/jpeg" // register JPEG decoding for decodedImageSize's image.DecodeConfig
	_ "image/png"  // register PNG decoding for decodedImageSize's image.DecodeConfig
	"os"

	_ "golang.org/x/image/bmp"  // register BMP decoding for decodedImageSize's image.DecodeConfig
	_ "golang.org/x/image/webp" // register WebP decoding for decodedImageSize's image.DecodeConfig

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// NewImageView renders path as an image, defaulting to fit-window scaling
// with a toggle to view it at native resolution (scrollable if larger than
// the window).
func NewImageView(path string) fyne.CanvasObject {
	img := canvas.NewImageFromFile(path)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(200, 200))

	nativeSize, hasNativeSize := decodedImageSize(path)

	scroll := container.NewScroll(img)

	// canvas.Image.Resize scales/repositions according to FillMode, but
	// flipping FillMode alone (without an explicit Resize) leaves the image
	// at whatever size it last got from a real window-resize event -- a
	// container's Layout (which is what actually calls content.Resize) only
	// runs when the container itself is resized, not on a bare Refresh().
	// Confirmed via manual testing: switching back to "Fit to Window" after
	// shrinking the window did nothing until the window was resized again.
	// So each toggle must Resize the image itself to the size appropriate
	// for its new mode, not rely on Refresh() or wait for the next window
	// resize to fix it.
	fitting := true
	toggle := widget.NewButton("View at 100%", nil)
	toggle.OnTapped = func() {
		if fitting {
			img.FillMode = canvas.ImageFillOriginal
			toggle.SetText("Fit to Window")
			if hasNativeSize {
				img.Resize(nativeSize)
			}
		} else {
			img.FillMode = canvas.ImageFillContain
			toggle.SetText("View at 100%")
			img.Resize(scroll.Size())
		}
		fitting = !fitting
		img.Refresh()
	}

	toolbar := container.NewHBox(toggle)
	return container.NewBorder(toolbar, nil, nil, nil, scroll)
}

// decodedImageSize reads just the image header to get its native pixel
// dimensions (used to size the "View at 100%" mode), without decoding the
// full pixel data.
func decodedImageSize(path string) (fyne.Size, bool) {
	f, err := os.Open(path)
	if err != nil {
		return fyne.Size{}, false
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return fyne.Size{}, false
	}
	return fyne.NewSize(float32(cfg.Width), float32(cfg.Height)), true
}
