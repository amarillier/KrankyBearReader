package main

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var aboutWindow fyne.Window
var aboutManagedWindow *managedWindow

// showAbout displays the About dialog with app branding, version, and links
// Reusable pattern from KrankyBearClock - customize these for your app:
//   - appName: Your application name
//   - appVersion: Current version string
//   - appAuthor: Author name
//   - appCopyright: Copyright string (can use dynamic year)
//   - resourceKrankyBearReaderPng: Your embedded icon resource
//   - GitHub and License URLs
func showAbout(a fyne.App) {
	if aboutWindow != nil && aboutWindow.Content().Visible() {
		aboutManagedWindow.open = true
		aboutWindow.Show()
		aboutWindow.RequestFocus()
		return
	}

	aboutWindow = a.NewWindow(appName + " - About")
	aboutWindow.SetIcon(resourceKrankyBearReaderPng)

	// App icon - ImageFillContain via newBrandingDialogImage keeps this at
	// brandingImageSizeDialog regardless of the source PNG's native
	// resolution (ImageFillOriginal renders at native size, which blows the
	// window up; see CLAUDE.md).
	icon := newBrandingDialogImage(resourceKrankyBearReaderPng)

	// Reflects the last update check's verdict (see update.go's
	// aheadOfLatestRelease): a small HardHat badge beside the app icon when
	// this build is newer than the latest published GitHub release.
	var iconDisplay fyne.CanvasObject = icon
	if aheadOfLatestRelease.Load() {
		badge := newBrandingBadgeImage(resourceKrankyBearHardHatPng)
		iconDisplay = container.NewHBox(icon, badge)
	}

	// Title and version info
	title := widget.NewLabelWithStyle(appName, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	version := widget.NewLabel("Version: " + appVersion)
	version.Alignment = fyne.TextAlignCenter

	// Description - customize for your app (placeholder; rename-app.sh won't change it)
	description := widget.NewLabel("A tabbed, multi-format document reader — text, JSON/YAML/TOML/XML, Markdown, CSV, images, and PDF")
	description.Alignment = fyne.TextAlignCenter
	description.Wrapping = fyne.TextWrapWord

	// Copyright and author
	copyright := widget.NewLabel(appCopyright)
	copyright.Alignment = fyne.TextAlignCenter
	author := widget.NewLabel("By " + appAuthor)
	author.Alignment = fyne.TextAlignCenter

	// Links - update URLs for your project
	licenseURL, _ := url.Parse("https://github.com/amarillier/KrankyBearReader/blob/main/LICENSE")
	licenseLink := widget.NewHyperlink("License Information", licenseURL)
	licenseLink.Alignment = fyne.TextAlignCenter

	githubURL, _ := url.Parse("https://github.com/amarillier/KrankyBearReader")
	githubLink := widget.NewHyperlink("GitHub Repository", githubURL)
	githubLink.Alignment = fyne.TextAlignCenter

	// Layout
	content := container.NewVBox(
		container.NewCenter(iconDisplay),
		widget.NewSeparator(),
		title,
		version,
		description,
		widget.NewSeparator(),
		copyright,
		author,
		widget.NewSeparator(),
		container.NewCenter(licenseLink),
		container.NewCenter(githubLink),
	)

	aboutWindow.SetContent(container.NewPadded(content))
	aboutWindow.Resize(fyne.NewSize(480, 620))

	aboutWindow.SetCloseIntercept(func() {
		aboutManagedWindow.open = false
		aboutWindow.Hide()
	})
	registerBossKeyHideAll(aboutWindow)

	aboutManagedWindow = registerManagedWindow(aboutWindow)
	aboutManagedWindow.open = true
	aboutWindow.Show()
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
