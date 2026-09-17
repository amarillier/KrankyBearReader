package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// managedWindow tracks whether the user currently wants a given window open.
// Fyne has no Window.IsVisible(), so this is tracked by hand: registration
// happens once per window (on first creation), the reuse branch of each
// show* function and Show() itself set open = true, and each window's
// SetCloseIntercept sets open = false before Hide(). That way showAllWindows
// restores exactly the set hideAllWindows put away, not every window ever
// created. Mirrors the convention established in ../KrankyBearInstallerBear
// and ../KrankyBearClipboardSentinel (see CLAUDE.md "Hide all / show all
// windows").
type managedWindow struct {
	win  fyne.Window
	open bool
}

var managedWindows []*managedWindow

func registerManagedWindow(win fyne.Window) *managedWindow {
	mw := &managedWindow{win: win}
	managedWindows = append(managedWindows, mw)
	return mw
}

// hideAllWindows hides every window the user currently has open, leaving
// already-closed windows alone.
func hideAllWindows() {
	for _, mw := range managedWindows {
		if mw.open {
			mw.win.Hide()
		}
	}
}

// showAllWindows restores exactly the set of windows hideAllWindows hid.
func showAllWindows() {
	for _, mw := range managedWindows {
		if mw.open {
			mw.win.Show()
		}
	}
}

// registerBossKeyHideAll wires the Alt+H "boss key" (CLAUDE.md: avoid Cmd+H,
// macOS reserves it) on win's canvas to hide every open window. Canvas
// shortcuts only fire on a focused window, so this is registered on every
// managed window, not just the main one, so the boss key works no matter
// which one currently has focus. There is deliberately no matching
// show-all hotkey: reveal via tray/menu instead (a hidden window can't be
// un-hidden by a canvas shortcut anyway, since it isn't focused).
func registerBossKeyHideAll(win fyne.Window) {
	win.Canvas().AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyH, Modifier: fyne.KeyModifierAlt},
		func(fyne.Shortcut) { hideAllWindows() },
	)
}
