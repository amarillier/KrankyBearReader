package viewer

import (
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

const (
	prefRecentFiles = "reader.recentFiles"
	maxRecentFiles  = 15
)

// Manager owns the tabbed document area: opening files (from the menu, tray,
// drag-drop, command line, or the Recent Files list all funnel through
// OpenFile, mirroring the single-entry-point pattern in pdfviewer's
// PDFViewer.LoadPDF), the recent-files list, and window-level shortcuts.
type Manager struct {
	app fyne.App
	win fyne.Window

	tabs        *container.DocTabs
	placeholder fyne.CanvasObject
	root        *fyne.Container

	openPaths map[string]*container.TabItem
	records   map[*container.TabItem]*tabRecord
	lastDir   fyne.ListableURI
}

// tabRecord holds a tab's optional hooks (see tabHooks in tab.go), keyed by
// the tab's own *container.TabItem so both close-on-tab-close and
// typedKey-on-selected-tab dispatch can find them.
type tabRecord struct {
	close    func()
	typedKey func(*fyne.KeyEvent)
}

// NewManager creates a Manager. Call Build to get the content for
// win.SetContent.
func NewManager(win fyne.Window, a fyne.App) *Manager {
	return &Manager{
		app:       a,
		win:       win,
		openPaths: map[string]*container.TabItem{},
		records:   map[*container.TabItem]*tabRecord{},
	}
}

// Build constructs the tab area and placeholder-when-empty state, and wires
// window-level keyboard shortcuts. Call once.
func (m *Manager) Build() fyne.CanvasObject {
	m.tabs = container.NewDocTabs()
	m.tabs.CloseIntercept = func(item *container.TabItem) {
		m.tabs.Remove(item)
		m.forgetTab(item)
		m.refreshRoot()
	}

	m.placeholder = container.NewCenter(widget.NewLabel(
		"Open a file to get started\n(File → Open..., or drag a file onto this window)"))
	m.root = container.NewStack(m.placeholder)

	m.setupShortcuts()
	return m.root
}

func (m *Manager) forgetTab(item *container.TabItem) {
	if rec, ok := m.records[item]; ok {
		if rec.close != nil {
			rec.close()
		}
		delete(m.records, item)
	}
	for p, it := range m.openPaths {
		if it == item {
			delete(m.openPaths, p)
			return
		}
	}
}

func (m *Manager) refreshRoot() {
	if len(m.tabs.Items) == 0 {
		m.root.Objects = []fyne.CanvasObject{m.placeholder}
	} else {
		m.root.Objects = []fyne.CanvasObject{m.tabs}
	}
	m.root.Refresh()
}

// OpenFile is the single entry point for opening a document: the menu, tray,
// Recent Files list, drag-drop, and command-line arguments all call this. If
// path is already open, its existing tab is focused instead of duplicated.
func (m *Manager) OpenFile(path string) error {
	path = filepath.Clean(path)

	if item, ok := m.openPaths[path]; ok {
		m.tabs.Select(item)
		return nil
	}

	format, err := openAndDetect(path)
	if err != nil {
		return err
	}

	item := &container.TabItem{Text: filepath.Base(path)}
	item.Content = newTabContent(m.win, path, format, func(hooks tabHooks) {
		m.records[item] = &tabRecord{close: hooks.close, typedKey: hooks.typedKey}
	})
	m.openPaths[path] = item

	m.tabs.Append(item)
	m.tabs.Select(item)
	m.addRecent(path)
	m.refreshRoot()
	return nil
}

// OpenDialog prompts for a file to open via OpenFile. No extension filter is
// applied — this is a general-purpose reader, and an unrecognized format
// still gets a hex-view fallback rather than being refused.
func (m *Manager) OpenDialog() {
	fd := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if r == nil {
			return // user cancelled
		}
		defer r.Close()

		path := r.URI().Path()
		if parent, perr := storage.Parent(r.URI()); perr == nil {
			if lister, lerr := storage.ListerForURI(parent); lerr == nil {
				m.lastDir = lister
			}
		}
		if err := m.OpenFile(path); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open %s: %w", filepath.Base(path), err), m.win)
		}
	}, m.win)
	if m.lastDir != nil {
		fd.SetLocation(m.lastDir)
	}
	fd.Show()
}

// CloseCurrentTab closes the selected tab, if any.
func (m *Manager) CloseCurrentTab() {
	item := m.tabs.Selected()
	if item == nil {
		return
	}
	m.tabs.Remove(item)
	m.forgetTab(item)
	m.refreshRoot()
}

// RecentFiles returns recently opened paths, most-recent-first.
func (m *Manager) RecentFiles() []string {
	return m.app.Preferences().StringList(prefRecentFiles)
}

// ClearRecent empties the recent-files list.
func (m *Manager) ClearRecent() {
	m.app.Preferences().SetStringList(prefRecentFiles, []string{})
}

func (m *Manager) addRecent(path string) {
	existing := m.RecentFiles()
	updated := make([]string, 0, len(existing)+1)
	updated = append(updated, path)
	for _, p := range existing {
		if p != path {
			updated = append(updated, p)
		}
	}
	if len(updated) > maxRecentFiles {
		updated = updated[:maxRecentFiles]
	}
	m.app.Preferences().SetStringList(prefRecentFiles, updated)
}

// setupShortcuts registers Ctrl/Cmd+O (open) and Ctrl/Cmd+W (close tab) on the
// window's canvas, mirroring pdfviewer's setupShortcuts. It also installs a
// single canvas-level typed-key dispatcher: unlike the fixed Ctrl+O/Ctrl+W
// shortcuts, a PDF tab's own page-navigation keys (Space/PageDown/arrows)
// must only apply while a PDF tab is the one currently selected — a JSON or
// CSV tab shouldn't page-turn on Space. Since fyne.Canvas has only one
// SetOnTypedKey slot (a second call would silently replace the first, which
// would break with more than one PDF tab open), this dispatches to whichever
// tab is currently selected rather than each tab installing its own.
func (m *Manager) setupShortcuts() {
	c := m.win.Canvas()
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: fyne.KeyModifierShortcutDefault},
		func(fyne.Shortcut) { m.OpenDialog() })
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyW, Modifier: fyne.KeyModifierShortcutDefault},
		func(fyne.Shortcut) { m.CloseCurrentTab() })

	c.SetOnTypedKey(func(ev *fyne.KeyEvent) {
		item := m.tabs.Selected()
		if item == nil {
			return
		}
		if rec, ok := m.records[item]; ok && rec.typedKey != nil {
			rec.typedKey(ev)
		}
	})
}
