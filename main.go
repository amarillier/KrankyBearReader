package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"

	"reader/internal/startup"
	"reader/internal/viewer"
)

const (
	// appName    = "KrankyBear Reader"
	appVersion = "0.2.0" // see FyneApp.toml
	appAuthor  = "Allan Marillier"
	appID      = "com.github.amarillier.KrankyBearReader"
)

var appName = "KrankyBear Reader"
var appCopyright = buildCopyrightNotice()

func buildCopyrightNotice() string {
	const startYear = 2026
	currentYear := time.Now().Year()
	if currentYear <= startYear {
		return "Copyright (c) Allan Marillier, 2026"
	}
	return fmt.Sprintf("Copyright (c) Allan Marillier, 2026-%d", currentYear)
}

func main() {
	langFlag := flag.String("lang", "", "UI language code (e.g. en, de); overrides the saved preference for this run")
	mesaFallbackFlag := flag.Bool(startup.MesaFallbackFlagName, false, "internal: relaunch flag for the Mesa3D OpenGL fallback (Windows only)")
	flag.Parse()

	// Windows only (no-op elsewhere): prefer real hardware OpenGL, falling
	// back to the bundled Mesa3D software renderer (relaunching once) only
	// if the hardware probe fails. Must run before app.NewWithID — GLFW
	// only supports one Init/Terminate cycle per process, shared with
	// Fyne's own driver (see internal/startup/opengl.go, and CLAUDE.md's
	// "Mesa3D OpenGL fallback" section).
	startup.EnsureWindowsOpenGLReady(*mesaFallbackFlag)

	a := app.NewWithID(appID)
	a.SetIcon(resourceKrankyBearReaderPng)
	setupI18n(a, *langFlag) // load message catalog + resolve UI language before building any UI
	loadTheme(a)

	win := a.NewWindow(appName)
	win.SetIcon(resourceKrankyBearReaderPng)
	win.Resize(mainWindowLaunchSize(a)) // restore previous size (size only - Fyne can't restore position)

	// The main window's close-intercept quits rather than hides (below), so
	// its "open" flag never flips false — Show All/Hide All only ever act on
	// it and whichever of About/Help/Release Notes are currently open too.
	mainManagedWindow := registerManagedWindow(win)
	mainManagedWindow.open = true
	registerBossKeyHideAll(win)

	manager := viewer.NewManager(win, a)
	win.SetContent(manager.Build())
	win.SetMainMenu(buildMenu(a, win, manager))
	setupSystemTray(a, win, manager)

	// Drag-and-drop a file onto the window opens it as a new tab — no
	// extension filtering, DetectFormat handles anything (falling back to a
	// hex view for the truly unrecognized).
	win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		for _, u := range uris {
			if u == nil {
				continue
			}
			if err := manager.OpenFile(u.Path()); err != nil {
				dialog.ShowError(fmt.Errorf("failed to open %s: %w", filepath.Base(u.Path()), err), win)
			}
		}
	})

	// Closing the window quits the app. Deferred via fyne.Do so quit() runs on a
	// clean loop iteration outside whatever callback triggered it — quitting
	// directly from inside a menu-item click or close-intercept callback can hang
	// on Windows (see CLAUDE.md "Quitting cleanly").
	win.SetCloseIntercept(func() { fyne.Do(func() { quitApp(a, win) }) })

	checkForUpdatesAuto(a) // quiet, once-per-day check; dialog only if an update exists

	for _, path := range os.Args[1:] {
		if err := manager.OpenFile(path); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open %s: %w", filepath.Base(path), err), win)
		}
	}

	win.ShowAndRun()
}

// ── Window geometry ──────────────────────────────────────────────────────────
// Fyne has no cross-platform window position/display restore, so only size is
// persisted (see CLAUDE.md "Window size persistence").

const (
	prefWinWidth  = "mainWindowWidth"
	prefWinHeight = "mainWindowHeight"
	minWinWidth   = 400
	minWinHeight  = 300
	maxWinDim     = 8000
	defaultWinW   = 900
	defaultWinH   = 650
)

func mainWindowLaunchSize(a fyne.App) fyne.Size {
	w := a.Preferences().FloatWithFallback(prefWinWidth, defaultWinW)
	h := a.Preferences().FloatWithFallback(prefWinHeight, defaultWinH)
	if w < minWinWidth || w > maxWinDim {
		w = defaultWinW
	}
	if h < minWinHeight || h > maxWinDim {
		h = defaultWinH
	}
	return fyne.NewSize(float32(w), float32(h))
}

func saveMainWindowGeometry(a fyne.App, win fyne.Window) {
	size := win.Canvas().Size()
	a.Preferences().SetFloat(prefWinWidth, float64(size.Width))
	a.Preferences().SetFloat(prefWinHeight, float64(size.Height))
}

// quitApp does teardown in the order CLAUDE.md calls out: stop background work
// first (none yet in this bare template — add tickers/players above this call
// as the app grows), then persist geometry, then quit.
func quitApp(a fyne.App, win fyne.Window) {
	saveMainWindowGeometry(a, win)
	a.Quit()
}

// ── Menu + tray (mirror each other; see CLAUDE.md "System tray + main menu") ──

func buildMenu(a fyne.App, win fyne.Window, manager *viewer.Manager) *fyne.MainMenu {
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("Open...", func() { manager.OpenDialog() }),
		fyne.NewMenuItem("Open Recent...", func() { showRecentDialog(win, manager) }),
		fyne.NewMenuItem("Close Tab", func() { manager.CloseCurrentTab() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() { fyne.Do(func() { quitApp(a, win) }) }),
	)
	viewMenu := fyne.NewMenu("View",
		fyne.NewMenuItem("Show All Windows", func() { showAllWindows(); win.RequestFocus() }),
		fyne.NewMenuItem("Hide All Windows", func() { hideAllWindows() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Light Theme", func() { setLightTheme(a) }),
		fyne.NewMenuItem("Dark Theme", func() { setDarkTheme(a) }),
		fyne.NewMenuItem("System Theme", func() { setSystemTheme(a) }),
	)
	helpItems := []*fyne.MenuItem{
		fyne.NewMenuItem("Help", func() { showHelp(a) }),
	}
	if samples := sampleFilesMenuItem(win, manager); samples != nil {
		helpItems = append(helpItems, samples)
	}
	helpItems = append(helpItems,
		fyne.NewMenuItem("Release Notes", func() { showReleaseNotes(a) }),
		fyne.NewMenuItem("Check for Updates", func() { checkForUpdatesManual(a) }),
		fyne.NewMenuItem("About", func() { showAbout(a) }),
	)
	helpMenu := fyne.NewMenu("Help", helpItems...)
	return fyne.NewMainMenu(fileMenu, viewMenu, helpMenu)
}

// setupSystemTray mirrors the main menu. Tray callbacks fire off the main
// goroutine, so every body is wrapped in fyne.Do (CLAUDE.md "fyne.Do is
// mandatory").
func setupSystemTray(a fyne.App, win fyne.Window, manager *viewer.Manager) {
	desk, ok := a.(desktop.App)
	if !ok {
		return // not a desktop driver
	}
	trayItems := []*fyne.MenuItem{
		fyne.NewMenuItem("Show All Windows", func() { fyne.Do(func() { showAllWindows(); win.RequestFocus() }) }),
		fyne.NewMenuItem("Hide All Windows", func() { fyne.Do(hideAllWindows) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Open...", func() { fyne.Do(func() { win.Show(); win.RequestFocus(); manager.OpenDialog() }) }),
		fyne.NewMenuItem("Open Recent...", func() { fyne.Do(func() { win.Show(); win.RequestFocus(); showRecentDialog(win, manager) }) }),
		fyne.NewMenuItem("Close Tab", func() { fyne.Do(func() { manager.CloseCurrentTab() }) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Light Theme", func() { fyne.Do(func() { setLightTheme(a) }) }),
		fyne.NewMenuItem("Dark Theme", func() { fyne.Do(func() { setDarkTheme(a) }) }),
		fyne.NewMenuItem("System Theme", func() { fyne.Do(func() { setSystemTheme(a) }) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Help", func() { fyne.Do(func() { showHelp(a) }) }),
	}
	if samples := sampleFilesMenuItem(win, manager); samples != nil {
		trayItems = append(trayItems, samples)
	}
	trayItems = append(trayItems,
		fyne.NewMenuItem("Release Notes", func() { fyne.Do(func() { showReleaseNotes(a) }) }),
		fyne.NewMenuItem("Check for Updates", func() { checkForUpdatesManual(a) }),
		fyne.NewMenuItem("About", func() { fyne.Do(func() { showAbout(a) }) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() { fyne.Do(func() { quitApp(a, win) }) }),
	)
	menu := fyne.NewMenu(appName, trayItems...)
	desk.SetSystemTrayMenu(menu)
	desk.SetSystemTrayIcon(resourceKrankyBearReaderPng)

	// Hover tooltip on the tray icon (Windows/macOS; no-op on Linux) --
	// desktop.App has no tooltip setter, but fyne.io/systray (what Fyne's
	// own driver already uses internally for the tray icon) does. Deferred
	// slightly since, unlike Fyne's own SetSystemTrayIcon call above, a raw
	// systray.SetTooltip call has no built-in retry/caching if the tray
	// isn't fully ready yet.
	time.AfterFunc(300*time.Millisecond, func() {
		systray.SetTooltip(appName)
	})
}

// showRecentDialog lists recently opened files (read fresh from preferences
// each time, so no native-menu rebuild is needed). Clicking one opens it;
// "Clear Recent List" empties it. Modeled on pdfviewer's showRecentDialog.
func showRecentDialog(win fyne.Window, manager *viewer.Manager) {
	recent := manager.RecentFiles()
	if len(recent) == 0 {
		dialog.ShowInformation("Recent Files", "No recent files yet.", win)
		return
	}

	var d dialog.Dialog
	list := container.NewVBox()
	for _, p := range recent {
		p := p // capture
		btn := widget.NewButton(filepath.Base(p)+"  —  "+filepath.Dir(p), func() {
			if d != nil {
				d.Hide()
			}
			if err := manager.OpenFile(p); err != nil {
				dialog.ShowError(fmt.Errorf("failed to open %s: %w", filepath.Base(p), err), win)
			}
		})
		btn.Alignment = widget.ButtonAlignLeading
		list.Add(btn)
	}

	clearBtn := widget.NewButton("Clear Recent List", func() {
		manager.ClearRecent()
		if d != nil {
			d.Hide()
		}
	})
	clearBtn.Importance = widget.DangerImportance

	content := container.NewBorder(nil, clearBtn, nil, nil, container.NewVScroll(list))
	d = dialog.NewCustom("Recent Files", "Close", content, win)
	d.Resize(fyne.NewSize(640, 420))
	d.Show()
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
