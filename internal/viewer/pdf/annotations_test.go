package pdf

import (
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func makeTestAnnotation(contents string) model.AnnotationRenderer {
	rect := types.RectForDim(100, 10)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	return model.NewHighlightAnnotation(*rect, 0, contents, "", "", 0, nil, 0, 0, 0, "", nil, nil, "", "", quad)
}

func TestFlattenHighlights_SortsByPageAndKeepsOnlyKnownKinds(t *testing.T) {
	pgAnnots := map[int]model.PgAnnots{
		3: {
			model.AnnHighLight: model.Annot{Map: model.AnnotMap{1: makeTestAnnotation("page three highlight")}},
		},
		1: {
			model.AnnUnderline: model.Annot{Map: model.AnnotMap{2: makeTestAnnotation("page one underline")}},
			// Link annotations aren't "highlights or notes" and must be excluded.
			model.AnnLink: model.Annot{Map: model.AnnotMap{3: makeTestAnnotation("a link, not a highlight")}},
		},
	}

	got := flattenHighlights(pgAnnots)

	if len(got) != 2 {
		t.Fatalf("expected 2 highlights (Link excluded), got %d: %+v", len(got), got)
	}
	if got[0].Page != 1 || got[1].Page != 3 {
		t.Errorf("expected results sorted by page (1, 3), got (%d, %d)", got[0].Page, got[1].Page)
	}
	if got[0].Kind != "Underline" {
		t.Errorf("expected Kind %q, got %q", "Underline", got[0].Kind)
	}
	if got[0].Contents != "page one underline" {
		t.Errorf("expected Contents %q, got %q", "page one underline", got[0].Contents)
	}
}

func TestFlattenHighlights_NoAnnotationsReturnsEmpty(t *testing.T) {
	got := flattenHighlights(map[int]model.PgAnnots{})
	if len(got) != 0 {
		t.Errorf("expected no highlights, got %d", len(got))
	}
}
