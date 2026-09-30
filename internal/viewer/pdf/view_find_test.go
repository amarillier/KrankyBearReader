package pdf

import "testing"

func TestCompileTextCounter_PlainText(t *testing.T) {
	counter, err := compileTextCounter("cat", false)
	if err != nil {
		t.Fatalf("compileTextCounter: %v", err)
	}
	cases := []struct {
		s    string
		want int
	}{
		{"no match here", 0},
		{"one Cat here", 1}, // case-insensitive
		{"cat cat cat", 3},
		{"concatenate", 1}, // substring match, not word-boundary
		{"", 0},
	}
	for _, c := range cases {
		if got := counter(c.s); got != c.want {
			t.Errorf("counter(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestCompileTextCounter_Regex(t *testing.T) {
	counter, err := compileTextCounter(`\bcat\b`, true)
	if err != nil {
		t.Fatalf("compileTextCounter: %v", err)
	}
	if got := counter("concatenate a cat, then a cat"); got != 2 {
		t.Errorf("counter with word-boundary regex = %d, want 2 (concatenate excluded)", got)
	}
}

func TestCompileTextCounter_InvalidRegexErrors(t *testing.T) {
	if _, err := compileTextCounter("[invalid(", true); err == nil {
		t.Errorf("expected an error for invalid regex syntax, got nil")
	}
}

func TestCompileTextCounter_EmptyQueryAlwaysZero(t *testing.T) {
	counter, err := compileTextCounter("", false)
	if err != nil {
		t.Fatalf("compileTextCounter: %v", err)
	}
	if got := counter("anything at all"); got != 0 {
		t.Errorf("counter with empty query = %d, want 0", got)
	}
}

// TestFindState_Step_AdvancesThroughEveryOccurrence is the real fix Phase 1
// of the search work targets: stepping must move through every individual
// match, including several on the same page, not just jump to the nearest
// matching page. fromPage tracks whatever the previous step returned, the
// same way the real caller's v.currentPage does after v.jumpToPage — see
// TestFindState_Step_ReanchorsWhenCallerNavigatedAway for what happens
// when that assumption DOESN'T hold (the caller navigated elsewhere in
// between).
func TestFindState_Step_AdvancesThroughEveryOccurrence(t *testing.T) {
	// Page 3 has 2 matches, page 3 the SAME page twice in the flat list --
	// exactly what "several hits on the same page" looks like here.
	s := &findState{matchPages: []int{2, 3, 3, 7}, currentIndex: -1}

	page := s.step(1, 1)
	if page != 2 {
		t.Fatalf("first step forward from page 1 = %d, want 2 (nearest match at/after page 1)", page)
	}
	if page = s.step(page, 1); page != 3 {
		t.Errorf("second step = %d, want 3 (page 3's first occurrence)", page)
	}
	if page = s.step(page, 1); page != 3 {
		t.Errorf("third step = %d, want 3 again (page 3's second occurrence)", page)
	}
	if page = s.step(page, 1); page != 7 {
		t.Errorf("fourth step = %d, want 7", page)
	}
	if page = s.step(page, 1); page != 2 {
		t.Errorf("fifth step (wraps around) = %d, want 2", page)
	}
}

func TestFindState_Step_BackwardWrapsToLast(t *testing.T) {
	s := &findState{matchPages: []int{2, 5, 9}, currentIndex: -1}
	page := s.step(1, -1)
	if page != 9 {
		t.Errorf("first backward step from before any match = %d, want 9 (wraps to last)", page)
	}
	if page = s.step(page, -1); page != 5 {
		t.Errorf("second backward step = %d, want 5", page)
	}
}

// TestFindState_Step_ReanchorsWhenCallerNavigatedAway is the real
// regression test for a bug found via hands-on testing: searching the
// same query again after manually navigating elsewhere (e.g. jumping to
// page 1, or clicking a bookmark) kept advancing the OLD match sequence
// from wherever it had left off, instead of noticing the user had moved
// and starting fresh from where they now are.
func TestFindState_Step_ReanchorsWhenCallerNavigatedAway(t *testing.T) {
	s := &findState{matchPages: []int{2, 5, 9, 20}, currentIndex: -1}

	page := s.step(1, 1)
	if page != 2 {
		t.Fatalf("first step = %d, want 2", page)
	}
	page = s.step(page, 1) // caller followed the jump, as v.jumpToPage always does
	if page != 5 {
		t.Fatalf("second step (sequential) = %d, want 5", page)
	}

	// The user now manually navigates to page 1 themselves (not by
	// following a search jump) -- fromPage=1 no longer matches
	// lastPage=5.
	if page = s.step(1, 1); page != 2 {
		t.Errorf("step after manual navigation to page 1 = %d, want 2 (re-anchored, not a blind advance to 9)", page)
	}

	// Sequential stepping resumes correctly from the re-anchored point.
	if page = s.step(page, 1); page != 5 {
		t.Errorf("next step after re-anchoring = %d, want 5", page)
	}
}

// TestFindState_Step_FirstStepStartsNearCurrentPage confirms the "search
// from where you are" behavior the old page-only find already had is
// preserved: the very first step should land on the nearest match
// at-or-after the current page (forward) or at-or-before it (backward),
// not always match #1.
func TestFindState_Step_FirstStepStartsNearCurrentPage(t *testing.T) {
	s := &findState{matchPages: []int{2, 5, 9, 20}, currentIndex: -1}
	if page := s.step(10, 1); page != 20 {
		t.Errorf("forward from page 10 = %d, want 20 (nearest at/after 10)", page)
	}

	s2 := &findState{matchPages: []int{2, 5, 9, 20}, currentIndex: -1}
	if page := s2.step(10, -1); page != 9 {
		t.Errorf("backward from page 10 = %d, want 9 (nearest at/before 10)", page)
	}
}

// TestFindState_OccurrenceIndexOnPage confirms the on-page search-highlight
// feature's own index math: buildFindBar's find() needs to know which
// occurrence WITHIN the current page's own text to hand to
// Document.SearchMatchRect, not the flat document-wide currentIndex.
func TestFindState_OccurrenceIndexOnPage(t *testing.T) {
	s := &findState{matchPages: []int{2, 3, 3, 3, 7}, currentIndex: -1}

	s.step(1, 1) // -> page 2, index 0
	if got := s.occurrenceIndexOnPage(); got != 0 {
		t.Errorf("page 2's first (only) occurrence = %d, want 0", got)
	}

	s.step(2, 1) // -> page 3, index 1 (page 3's first occurrence)
	if got := s.occurrenceIndexOnPage(); got != 0 {
		t.Errorf("page 3's first occurrence = %d, want 0", got)
	}
	s.step(3, 1) // -> page 3, index 2 (page 3's second occurrence)
	if got := s.occurrenceIndexOnPage(); got != 1 {
		t.Errorf("page 3's second occurrence = %d, want 1", got)
	}
	s.step(3, 1) // -> page 3, index 3 (page 3's third occurrence)
	if got := s.occurrenceIndexOnPage(); got != 2 {
		t.Errorf("page 3's third occurrence = %d, want 2", got)
	}
	s.step(3, 1) // -> page 7, index 4 (page 7's first, and only, occurrence)
	if got := s.occurrenceIndexOnPage(); got != 0 {
		t.Errorf("page 7's first occurrence = %d, want 0", got)
	}
}

func TestFindState_Stale(t *testing.T) {
	var nilState *findState
	if !nilState.stale("x", false) {
		t.Errorf("expected a nil *findState to always report stale")
	}

	s := &findState{query: "cat", useRegex: false}
	if s.stale("cat", false) {
		t.Errorf("expected an unchanged query/regex to not be stale")
	}
	if !s.stale("dog", false) {
		t.Errorf("expected a changed query to be stale")
	}
	if !s.stale("cat", true) {
		t.Errorf("expected a changed regex flag to be stale")
	}
}
