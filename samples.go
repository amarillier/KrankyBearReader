package main

import (
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"reader/internal/apppaths"
	"reader/internal/viewer"
)

// sampleFileNames lists the bundled sample files, in menu order, each
// demonstrating one supported viewer. Kept in sync by hand with
// assets/samples/ -- there's no need for these to be dynamically discovered,
// since the point is a short, curated tour rather than an exhaustive listing.
var sampleFileNames = []string{
	"Sample.json",
	"Sample.yaml",
	"Sample.toml",
	"Sample.xml",
	"Sample.md",
	"Sample.csv",
	"Sample.tsv",
	"Sample.txt",
	"Sample.png",
	"Sample.bin",
}

// sampleFilesMenuItem builds a "Sample Files" submenu with one item per
// bundled sample (Help -> Sample Files -> Sample.json, ...), each opening in
// its own tab via the same Manager.OpenFile entry point as every other launch
// surface. Returns nil if none of the samples can be found (e.g. a standalone
// binary copied without its accompanying assets) -- mirrors ReleaseNotes'
// "gracefully absent, not a broken menu item" handling.
func sampleFilesMenuItem(win fyne.Window, manager *viewer.Manager) *fyne.MenuItem {
	var items []*fyne.MenuItem
	for _, name := range sampleFileNames {
		path := apppaths.ResolveBundledAsset(filepath.Join("assets", "samples", name))
		if path == "" {
			continue
		}
		name, path := name, path // capture
		items = append(items, fyne.NewMenuItem(name, func() {
			if err := manager.OpenFile(path); err != nil {
				dialog.ShowError(fmt.Errorf("failed to open sample %s: %w", name, err), win)
			}
		}))
	}
	if len(items) == 0 {
		return nil
	}
	return &fyne.MenuItem{Label: "Sample Files", ChildMenu: fyne.NewMenu("", items...)}
}
