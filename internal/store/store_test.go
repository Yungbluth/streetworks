package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var testMeta = Meta{Built: "2026-10-23T13:17:00Z", Version: 2, Permits: 3}

func testPermits() []Permit {
	return []Permit{
		{ID: 0, Place: 4, Year: 2023, Cat: 1, Detail: Detail{
			Permit: "T23SY041246", Type: "Street Cut", Status: "Issued", Date: "2023-05-01", DateBasis: "issued",
			Description: "Sidewalk and curb ramp rebuild", Applicant: "Southwest Gas", Filed: "100 Main St",
			Flags: []string{"street read from location text"},
		}},
		{ID: 1, Place: 4, Year: 2006, Cat: 0, Detail: Detail{
			Permit: "T06EX000123", Type: "Excavation", Status: "Final", Date: "2006-01-02", DateBasis: "issued",
			Description: "Water service tap", Filed: "200 Oak Ave",
		}},
		{ID: 2, Place: 9, Year: 0, Cat: 0, Detail: Detail{
			Permit: "T19CO000777", Type: "Conduit", Status: "Issued", NoWork: true,
			Description: "Fiber conduit bore", Applicant: "Cox", Filed: "300 Pine Rd",
		}},
	}
}

func fixture(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.db")
	if err := Write(path, testMeta, testPermits()); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMeta(t *testing.T) {
	m, err := fixture(t).Meta(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m != testMeta {
		t.Errorf("got %+v, want %+v", m, testMeta)
	}
}

func TestLookup(t *testing.T) {
	got, err := fixture(t).Lookup(context.Background(), []int{2, 0, 0, 99})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d permits, want 2: %v", len(got), got)
	}
	if got[2].Permit != "T19CO000777" || !got[2].NoWork {
		t.Errorf("permit 2 = %+v", got[2])
	}
	want := []string{"street read from location text"}
	if !reflect.DeepEqual(got[0].Flags, want) {
		t.Errorf("flags = %v, want %v", got[0].Flags, want)
	}
}

func TestByNumber(t *testing.T) {
	p, err := fixture(t).ByNumber(context.Background(), "  t23sy041246 ")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 0 || p.Place != 4 {
		t.Errorf("got id %d place %d, want 0 and 4", p.ID, p.Place)
	}
}

func TestByNumberMissing(t *testing.T) {
	_, err := fixture(t).ByNumber(context.Background(), "T00XX000000")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.db")
	if err := Write(path, testMeta, testPermits()); err != nil {
		t.Fatal(err)
	}
	bad := testPermits()
	bad[1].ID = bad[0].ID // violates the primary key
	if err := Write(path, testMeta, bad); err == nil {
		t.Fatal("expected the second write to fail")
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := db.Meta(context.Background())
	if err != nil || m.Permits != 3 {
		t.Errorf("meta = %+v, err = %v", m, err)
	}
	var n int
	if err := db.db.QueryRow("SELECT count(*) FROM permits").Scan(&n); err != nil || n != 3 {
		t.Errorf("rows = %d, err = %v", n, err)
	}
	if tmp, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(tmp) != 0 {
		t.Errorf("left behind %v", tmp)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("tmp file remains: %v", err)
	}
}
