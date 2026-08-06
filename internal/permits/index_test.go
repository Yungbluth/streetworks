package permits

import (
	"fmt"
	"strings"
	"testing"
	"time"

)

func rec(permit, cat, status, street string, block int, year int) Record {
	r := Record{Permit: permit, Category: cat, Status: status, Street: street, Block: block,
		HasGeo: true, Lat: 32.2 + float64(block)/1e5, Lng: -110.9}
	if year > 0 {
		r.When = time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC)
		r.Date = r.When.Format("2006-01-02")
		r.DateBasis = "DATE_ISSUED"
	}
	return r
}

func TestFilterMatrix(t *testing.T) {
	recs := []Record{
		rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019),
		rec("2", "Excavation", "VOID", "E A ST", 100, 2020),
		rec("3", "Barricade", "CLOSED", "E A ST", 100, 2021),
		rec("4", "Combo", "ISSUED", "E A ST", 100, 2022),
		rec("5", "Civic event", "CLOSED", "E A ST", 100, 2023),
		rec("6", "Excavation", "CLOSED", "E A ST", 100, 0), // undated
	}
	ix := Build(recs)
	count := func(f Filter) int {
		bs := ix.Blocks(f)
		if len(bs) == 0 {
			return 0
		}
		return bs[0].Count
	}
	cats := func(c ...string) map[string]bool {
		m := map[string]bool{}
		for _, x := range c {
			m[x] = true
		}
		return m
	}
	cases := []struct {
		name string
		f    Filter
		want int
	}{
		{"no filter: voided hidden, undated included", Filter{}, 5},
		{"include void", Filter{NoWork: true}, 6},
		{"from only", Filter{From: 2021}, 3},
		{"to only", Filter{To: 2020}, 1},
		{"year range inclusive both ends", Filter{From: 2020, To: 2022, NoWork: true}, 3},
		{"single year", Filter{From: 2022, To: 2022}, 1},
		{"range with no data", Filter{From: 2030, To: 2031}, 0},
		{"inverted range matches nothing", Filter{From: 2023, To: 2019}, 0},
		{"one category", Filter{Cats: cats("Barricade")}, 1},
		{"two categories", Filter{Cats: cats("Barricade", "Combo")}, 2},
		{"unknown category", Filter{Cats: cats("Nope")}, 0},
		{"category + year", Filter{Cats: cats("Excavation"), From: 2019, To: 2019}, 1},
		{"category + void + year", Filter{Cats: cats("Excavation"), From: 2020, To: 2020, NoWork: true}, 1},
		{"year filter excludes undated", Filter{From: 2000, To: 2100}, 4},
	}
	for _, c := range cases {
		if got := count(c.f); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestBuildMetaCountsExactly(t *testing.T) {
	bad := rec("B", "Excavation", "CLOSED", "E A ST", 100, 0)
	bad.DateFlag = "DATE_ISSUED 2049-09-03 is implausible"
	text := rec("T", "Excavation", "CLOSED", "E A ST", 100, 2020)
	text.StreetSource = "LOCATION"
	nogeo := rec("G", "Barricade", "CLOSED", "E A ST", 200, 2021)
	nogeo.HasGeo = false
	noblock := rec("N", "Combo", "CLOSED", "", -1, 2021)
	recs := []Record{
		rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019),
		rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019), // repeat row
		rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019), // and again
		rec("2", "Excavation", "VOID", "E A ST", 100, 2025),
		bad, text, nogeo, noblock,
	}
	m := Build(recs).Meta
	checks := map[string][2]int{
		"Rows": {m.Rows, 8}, "UniquePermits": {m.UniquePermits, 6}, "DuplicateRows": {m.DuplicateRows, 2},
		"NoWork": {m.NoWork, 1}, "NoDate": {m.NoDate, 1}, "BadDates": {m.BadDates, 1},
		"StreetFromText": {m.StreetFromText, 1}, "NoGeo": {m.NoGeo, 1}, "NoBlock": {m.NoBlock, 1}, "Blocks": {m.Blocks, 2},
	}
	for k, v := range checks {
		if v[0] != v[1] {
			t.Errorf("%s = %d, want %d", k, v[0], v[1])
		}
	}
	if m.DateMin != "2019-06-01" || m.DateMax != "2025-06-01" {
		t.Errorf("date range %s to %s", m.DateMin, m.DateMax)
	}
	if len(m.Statuses) != 2 || m.Statuses[0].Key != "CLOSED" || m.Statuses[0].N != 5 {
		t.Errorf("statuses = %+v", m.Statuses)
	}
}

func TestDuplicatesWithoutPermitNumbersAreKept(t *testing.T) {
	// Two rows with no permit number are not provably the same permit.
	ix := Build([]Record{rec("", "Excavation", "CLOSED", "E A ST", 100, 2020), rec("", "Excavation", "CLOSED", "E A ST", 100, 2020)})
	if ix.Meta.UniquePermits != 2 || ix.Meta.DuplicateRows != 0 {
		t.Errorf("%+v", ix.Meta)
	}
}

func TestBuildDoesNotAliasInputRecords(t *testing.T) {
	// Every Block record must be its own copy (a classic loop-variable bug).
	recs := []Record{rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019), rec("2", "Barricade", "CLOSED", "E A ST", 100, 2020), rec("3", "Combo", "CLOSED", "E A ST", 100, 2021)}
	ix := Build(recs)
	d, _ := ix.Block("E A ST|100", Filter{})
	seen := map[string]bool{}
	for _, r := range d.Records {
		seen[r.Permit] = true
	}
	if len(seen) != 3 {
		t.Errorf("records collapsed: %+v", d.Records)
	}
	recs[0].Permit = "MUTATED"
	d, _ = ix.Block("E A ST|100", Filter{})
	for _, r := range d.Records {
		if r.Permit == "MUTATED" {
			t.Error("index shares memory with the caller's slice")
		}
	}
}

func TestBlocksGroupingAndOrdering(t *testing.T) {
	var recs []Record
	add := func(street string, block, n int) {
		for i := 0; i < n; i++ {
			recs = append(recs, rec(fmt.Sprintf("%s-%d-%d", street, block, i), "Excavation", "CLOSED", street, block, 2020))
		}
	}
	add("E B ST", 200, 3)
	add("E A ST", 100, 3) // ties with E B ST|200; key order breaks the tie
	add("E A ST", 200, 1) // same block number, different street: separate block
	add("W A ST", 100, 5) // same street name, other direction: separate block
	bs := Build(recs).Blocks(Filter{})
	var got []string
	for _, b := range bs {
		got = append(got, fmt.Sprintf("%s:%d", b.Key, b.Count))
	}
	want := "W A ST|100:5 E A ST|100:3 E B ST|200:3 E A ST|200:1"
	if strings.Join(got, " ") != want {
		t.Errorf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
	// Ordering must be stable across calls (map iteration is random)
	for i := 0; i < 20; i++ {
		again := Build(recs).Blocks(Filter{})
		for j := range again {
			if again[j].Key != bs[j].Key {
				t.Fatalf("unstable order on run %d", i)
			}
		}
	}
}
