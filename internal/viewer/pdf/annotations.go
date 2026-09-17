package pdf

import (
	"os"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Highlight is one PDF annotation worth surfacing in the Highlights and
// Notes panel: a highlight/underline/strikeout/squiggly markup, or a
// text/popup note. Link annotations are deliberately excluded — they aren't
// "highlights or notes". Read-only: this app doesn't add or edit
// annotations, only lists what other apps (Preview, Acrobat, ...) already
// wrote, matching Preview's own "Highlights and Notes" sidebar.
type Highlight struct {
	Page     int
	Kind     string
	Contents string
}

// highlightKinds are the annotation types worth listing, and their display
// label. Anything not in this map (Link, Widget, Stamp, ...) is skipped.
var highlightKinds = map[model.AnnotationType]string{
	model.AnnHighLight: "Highlight",
	model.AnnUnderline: "Underline",
	model.AnnStrikeOut: "Strikeout",
	model.AnnSquiggly:  "Squiggly",
	model.AnnText:      "Note",
	model.AnnPopup:     "Note",
}

// LoadHighlights reads every highlight/underline/strikeout/squiggly/note
// annotation in path via pdfcpu, sorted by page. Not an error if the PDF has
// none — confirmed via manual testing (see the plan this shipped with) that
// go-fitz's renderer doesn't paint these into the page image itself, so this
// list (with jump-to-page) is what makes them visible here at all, even
// though the highlighted text itself won't glow on the rendered page yet.
func LoadHighlights(path string) ([]*Highlight, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	pgAnnots, err := api.Annotations(f, nil, conf)
	if err != nil {
		return nil, nil
	}
	return flattenHighlights(pgAnnots), nil
}

// flattenHighlights turns pdfcpu's map[page]map[type]Annot into a flat,
// page-sorted list, keeping only the types in highlightKinds. Split out from
// LoadHighlights so this logic is testable without a real PDF file — the
// pdfcpu types here can be constructed directly in memory.
func flattenHighlights(pgAnnots map[int]model.PgAnnots) []*Highlight {
	var out []*Highlight
	for page, byType := range pgAnnots {
		for typ, annot := range byType {
			label, ok := highlightKinds[typ]
			if !ok {
				continue
			}
			for _, a := range annot.Map {
				out = append(out, &Highlight{
					Page: page,
					Kind: label,
					// Content(), not ContentString(): MarkupAnnotation
					// overrides ContentString() to wrap the text in literal
					// quotes for CLI/debug display (confirmed by reading
					// pdfcpu's source after a test caught it) — not what we
					// want in a UI list.
					Contents: a.Content(),
				})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Page < out[j].Page })
	return out
}
