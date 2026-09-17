package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// releaseNotesBody resolves paths via os.Executable, which in `go test`
// points at a temp test binary, not this repo's own executable directory -
// so this only exercises the pure fallback text. The real disk-read paths
// are simple enough (os.ReadFile against a real sibling/assets file) to be
// covered adequately by manual verification, matching this template's
// existing convention of not chasing test coverage for thin OS-boundary
// code (see CLAUDE.md's own build/verify habits).
func TestReleaseNotesBody_NotFoundMentionsGitHub(t *testing.T) {
	body := releaseNotesBody()
	if !strings.Contains(body, "could not be found") {
		t.Errorf("expected not-found message, got: %q", body)
	}
	if !strings.Contains(body, "github.com/amarillier/KrankyBearReader") {
		t.Errorf("expected GitHub link in not-found message, got: %q", body)
	}
}

// TestReleaseNotesResolution_ChecksBothDirsAndPrefersMarkdown exercises the
// actual selection logic (not just the slice literals) against a fake
// executable directory: .md beats .txt when both exist, and a file in the
// "assets" subdirectory (this template's own macOS .app bundle layout,
// where package.sh lands ReleaseNotes.txt at Contents/MacOS/assets/, one
// level below the binary at Contents/MacOS/) is still found even though
// Windows/Linux both land it flat beside the binary instead.
func TestReleaseNotesResolution_ChecksBothDirsAndPrefersMarkdown(t *testing.T) {
	dir := t.TempDir()

	resolve := func() (string, bool) {
		for _, sub := range releaseNotesSearchDirs {
			for _, name := range releaseNotesFileNames {
				if data, err := os.ReadFile(filepath.Join(dir, sub, name)); err == nil {
					return string(data), true
				}
			}
		}
		return "", false
	}

	if _, ok := resolve(); ok {
		t.Fatalf("expected nothing found in an empty directory")
	}

	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "ReleaseNotes.txt"), []byte("mac bundle notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolve(); !ok || got != "mac bundle notes" {
		t.Fatalf("expected assets/ReleaseNotes.txt to be found (macOS layout), got %q, ok=%v", got, ok)
	}

	if err := os.WriteFile(filepath.Join(dir, "ReleaseNotes.txt"), []byte("flat notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolve(); !ok || got != "flat notes" {
		t.Fatalf("expected the flat (Windows/Linux layout) file to win once present, got %q, ok=%v", got, ok)
	}

	if err := os.WriteFile(filepath.Join(dir, "ReleaseNotes.md"), []byte("# flat markdown"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolve(); !ok || got != "# flat markdown" {
		t.Fatalf("expected .md to take priority over .txt in the same directory, got %q, ok=%v", got, ok)
	}
}

// TestReleaseNotesFileNames_FindsLowercaseVariant covers a sibling project
// that named the file all-lowercase (releasenotes.txt/.md) instead of this
// template's own PascalCase convention - Linux's case-sensitive filesystem
// means these are genuinely different filenames, not just a style choice.
func TestReleaseNotesFileNames_FindsLowercaseVariant(t *testing.T) {
	dir := t.TempDir()

	resolve := func() (string, bool) {
		for _, sub := range releaseNotesSearchDirs {
			for _, name := range releaseNotesFileNames {
				if data, err := os.ReadFile(filepath.Join(dir, sub, name)); err == nil {
					return string(data), true
				}
			}
		}
		return "", false
	}

	if err := os.WriteFile(filepath.Join(dir, "releasenotes.txt"), []byte("lowercase notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolve(); !ok || got != "lowercase notes" {
		t.Fatalf("expected releasenotes.txt (lowercase) to be found, got %q, ok=%v", got, ok)
	}
}
