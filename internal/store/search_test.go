package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchQuery(t *testing.T) {
	tests := []struct{ in, want string }{
		{"curb ramp", `"curb" "ramp"*`},
		{"  Southwest   GAS ", `"southwest" "gas"*`},
		{"NEAR(a b)", `"near" "a" "b"*`},
		{`a"b`, `"a" "b"*`},
		{"x OR y", `"x" "or" "y"*`},
		{`"t23sy0412"`, `"t23sy0412"*`},
	}
	for _, tt := range tests {
		got, err := MatchQuery(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("MatchQuery(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestMatchQueryErrors(t *testing.T) {
	const tooShort = "Type at least 2 letters or digits."
	for _, in := range []string{"", "a", "***", "--", `""`} {
		_, err := MatchQuery(in)
		if !errors.Is(err, ErrBadQuery) {
			t.Errorf("MatchQuery(%q) error = %v, want ErrBadQuery", in, err)
		} else if err.Error() != tooShort {
			t.Errorf("MatchQuery(%q) message = %q", in, err.Error())
		}
	}

	_, err := MatchQuery(strings.Repeat("a", 101))
	if !errors.Is(err, ErrBadQuery) || err.Error() != "Searches can be up to 100 characters." {
		t.Errorf("long query error = %v", err)
	}
	if _, err := MatchQuery(strings.Repeat("a", 100)); err != nil {
		t.Errorf("100 characters rejected: %v", err)
	}
	// the limit counts characters, not bytes
	if _, err := MatchQuery(strings.Repeat("é", 100)); err != nil {
		t.Errorf("100 multibyte characters rejected: %v", err)
	}
}

func ids(hits []Hit) []int {
	out := make([]int, len(hits))
	for i, h := range hits {
		out[i] = h.ID
	}
	return out
}

func TestSearchFindsDescription(t *testing.T) {
	hits, err := fixture(t).Search(context.Background(), "curb", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1: %+v", len(hits), hits)
	}
	h := hits[0]
	if h.ID != 0 || h.Place != 4 || h.Year != 2023 || h.Number != "T23SY041246" {
		t.Errorf("hit = %+v", h)
	}
	if !strings.Contains(strings.ToLower(h.Snippet), "<mark>curb</mark>") {
		t.Errorf("snippet = %q", h.Snippet)
	}
}

func TestSearchEscapesSnippet(t *testing.T) {
	ps := testPermits()
	ps = append(ps, Permit{ID: 3, Place: 1, Year: 2024, Cat: 0, Detail: Detail{
		Permit: "T24XX000001", Type: "Excavation", Status: "Issued", Date: "2024-01-01", DateBasis: "issued",
		Description: "<b>bore</b> & tap", Filed: "1 A St",
	}})
	path := filepath.Join(t.TempDir(), "s.db")
	if err := Write(path, Meta{Built: "x", Version: 2, Permits: len(ps)}, ps); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	hits, err := db.Search(context.Background(), "bore", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.ID != 3 {
			continue
		}
		s := h.Snippet
		if !strings.Contains(s, "&lt;b&gt;") || !strings.Contains(s, "&amp;") || strings.Contains(s, "<b>") ||
			!strings.Contains(s, "<mark>bore</mark>") {
			t.Errorf("snippet not escaped and marked: %q", s)
		}
		return
	}
	t.Fatalf("permit 3 not found in %+v", hits)
}

func TestSearchLimit(t *testing.T) {
	db := fixture(t)
	ctx := context.Background()
	hits, err := db.Search(ctx, "water", 1)
	if err != nil || len(hits) != 1 {
		t.Errorf("limit 1: %d hits, %v", len(hits), err)
	}
	// both 0 and 500 must be accepted and fall back to a sane page size
	for _, limit := range []int{0, 500, -3} {
		if _, err := db.Search(ctx, "water", limit); err != nil {
			t.Errorf("limit %d: %v", limit, err)
		}
	}
	if got := clampLimit(0); got != 20 {
		t.Errorf("clampLimit(0) = %d", got)
	}
	if got := clampLimit(500); got != 50 {
		t.Errorf("clampLimit(500) = %d", got)
	}
}

func TestSearchInjection(t *testing.T) {
	db := fixture(t)
	for _, q := range []string{"NEAR", "a OR b", "-x", "col:x", "x*"} {
		_, err := db.Search(context.Background(), q, 20)
		if err != nil && !errors.Is(err, ErrBadQuery) {
			t.Errorf("%q: unexpected error %v", q, err)
		}
	}
}
