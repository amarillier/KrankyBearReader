package pdf

import (
	"math"
	"testing"
)

const geomEpsilon = 0.0001

func approxEqual(a, b float64) bool { return math.Abs(a-b) < geomEpsilon }

func TestHexagonVertices_SixPointsOnEllipseTopFirst(t *testing.T) {
	v := hexagonVertices(100, 200, 50, 30)
	if len(v) != 6 {
		t.Fatalf("expected 6 vertices, got %d", len(v))
	}
	// First vertex is straight up from center (angle -90°): (cx, cy - ry)
	// in PDF space (+ry is "up", so -90° means cy - ry... wait: cos(-90)=0,
	// sin(-90)=-1, so y = cy + ry*sin(-90) = cy - ry).
	if !approxEqual(v[0][0], 100) || !approxEqual(v[0][1], 200-30) {
		t.Errorf("first vertex = %v, want (100, %v)", v[0], 200-30.0)
	}
	// Every vertex must lie exactly on the ellipse: ((x-cx)/rx)^2 + ((y-cy)/ry)^2 == 1.
	for i, p := range v {
		nx, ny := (p[0]-100)/50, (p[1]-200)/30
		if !approxEqual(nx*nx+ny*ny, 1) {
			t.Errorf("vertex %d = %v not on the ellipse (normalized radius^2 = %v)", i, p, nx*nx+ny*ny)
		}
	}
}

func TestStarVertices_TenPointsAlternatingRadius(t *testing.T) {
	v := starVertices(0, 0, 100, 100) // circular (rx==ry) for simple radius math
	if len(v) != 10 {
		t.Fatalf("expected 10 vertices, got %d", len(v))
	}
	for i, p := range v {
		r := math.Hypot(p[0], p[1])
		want := 100.0
		if i%2 == 1 {
			want = 100.0 * starInnerRadiusRatio
		}
		if !approxEqual(r, want) {
			t.Errorf("vertex %d radius = %v, want %v (i%%2=%d)", i, r, want, i%2)
		}
	}
}

func TestVerticesBoundsPt(t *testing.T) {
	v := [][2]float64{{10, 5}, {-3, 20}, {7, -2}}
	minX, minY, maxX, maxY := verticesBoundsPt(v)
	if minX != -3 || minY != -2 || maxX != 10 || maxY != 20 {
		t.Errorf("verticesBoundsPt(%v) = (%v,%v,%v,%v), want (-3,-2,10,20)", v, minX, minY, maxX, maxY)
	}
}
