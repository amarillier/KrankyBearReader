package viewer

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
)

func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	a := test.NewApp()
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	m := NewManager(win, a)
	m.Build()
	return m, t.TempDir()
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestManager_OpenPaths_ReflectsOpenTabsInOrder(t *testing.T) {
	m, dir := newTestManager(t)
	a := writeTestFile(t, dir, "a.txt", "a")
	b := writeTestFile(t, dir, "b.txt", "b")

	if err := m.OpenFile(a); err != nil {
		t.Fatalf("OpenFile(a): %v", err)
	}
	if err := m.OpenFile(b); err != nil {
		t.Fatalf("OpenFile(b): %v", err)
	}

	got := m.OpenPaths()
	want := []string{filepath.Clean(a), filepath.Clean(b)}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("OpenPaths() = %v, want %v", got, want)
	}
}

func TestManager_OpenPaths_DropsClosedTabs(t *testing.T) {
	m, dir := newTestManager(t)
	a := writeTestFile(t, dir, "a.txt", "a")
	b := writeTestFile(t, dir, "b.txt", "b")

	_ = m.OpenFile(a)
	_ = m.OpenFile(b)
	m.CloseCurrentTab() // closes b, the most-recently-selected

	got := m.OpenPaths()
	if len(got) != 1 || got[0] != filepath.Clean(a) {
		t.Errorf("OpenPaths() after closing b = %v, want [%s]", got, filepath.Clean(a))
	}
}

func TestManager_SaveAndRestoreOpenFilesForNextLaunch(t *testing.T) {
	m, dir := newTestManager(t)
	a := writeTestFile(t, dir, "a.txt", "a")
	_ = m.OpenFile(a)

	m.SaveOpenFilesForNextLaunch()

	got := m.FilesOpenAtLastQuit()
	if len(got) != 1 || got[0] != filepath.Clean(a) {
		t.Errorf("FilesOpenAtLastQuit() = %v, want [%s]", got, filepath.Clean(a))
	}
}

func TestManager_CanSaveCurrentTab_FalseForNonPDFTab(t *testing.T) {
	m, dir := newTestManager(t)
	txt := writeTestFile(t, dir, "a.txt", "just text")
	_ = m.OpenFile(txt)

	if m.CanSaveCurrentTab() {
		t.Error("expected CanSaveCurrentTab() to be false for a plain text tab")
	}
	// Should be a silent no-op, not a panic, when there's nothing to save.
	m.SaveCurrentTab()
}

func TestManager_CanSaveCurrentTab_FalseWithNoTabsOpen(t *testing.T) {
	m, _ := newTestManager(t)
	if m.CanSaveCurrentTab() {
		t.Error("expected CanSaveCurrentTab() to be false with no tabs open")
	}
	m.SaveCurrentTab()
}
