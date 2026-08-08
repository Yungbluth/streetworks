package permits

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
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

func TestBlockLocationIsMeanOfRecordPoints(t *testing.T) {
	a, b := rec("1", "Excavation", "CLOSED", "E A ST", 100, 2020), rec("2", "Excavation", "CLOSED", "E A ST", 100, 2020)
	a.Lat, a.Lng, b.Lat, b.Lng = 32.0, -111.0, 32.2, -110.8
	noGeo := rec("3", "Excavation", "CLOSED", "E A ST", 100, 2020)
	noGeo.HasGeo, noGeo.Lat, noGeo.Lng = false, 0, 0 // must not drag the mean toward 0,0
	bs := Build([]Record{a, b, noGeo}).Blocks(Filter{})
	if len(bs) != 1 || bs[0].Count != 3 || fmt.Sprintf("%.4f,%.4f", bs[0].Lat, bs[0].Lng) != "32.1000,-110.9000" {
		t.Errorf("%+v", bs)
	}
}

func TestBlocksWithoutAnyGeometryAreNotMappedButStillReadable(t *testing.T) {
	r := rec("1", "Excavation", "CLOSED", "E A ST", 100, 2020)
	r.HasGeo = false
	ix := Build([]Record{r})
	if len(ix.Blocks(Filter{})) != 0 {
		t.Error("a block with no coordinates must not be listed for the map")
	}
	if d, ok := ix.Block("E A ST|100", Filter{}); !ok || d.Count != 1 {
		t.Errorf("detail still available: %+v %v", d, ok)
	}
}

func TestBlockDetail(t *testing.T) {
	ix := Build([]Record{
		rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019),
		rec("2", "Excavation", "CLOSED", "E A ST", 100, 2023),
		rec("3", "Excavation", "CLOSED", "E A ST", 100, 0),
		rec("4", "Excavation", "CLOSED", "E A ST", 100, 2021),
	})
	d, ok := ix.Block("E A ST|100", Filter{})
	if !ok || d.Street != "E A ST" || d.Block != 100 || d.Count != 4 {
		t.Fatalf("%+v %v", d, ok)
	}
	var dates []string
	for _, r := range d.Records {
		dates = append(dates, r.Date)
	}
	if strings.Join(dates, ",") != "2023-06-01,2021-06-01,2019-06-01," {
		t.Errorf("not newest-first with undated last: %v", dates)
	}
	if _, ok := ix.Block("nope|0", Filter{}); ok {
		t.Error("unknown key")
	}
	if _, ok := ix.Block("", Filter{}); ok {
		t.Error("empty key")
	}
	// An empty result set must marshal as [], not null (the UI calls .map on it)
	d, _ = ix.Block("E A ST|100", Filter{From: 2100})
	b, _ := json.Marshal(d)
	if !strings.Contains(string(b), `"records":[]`) || d.Count != 0 {
		t.Errorf("%s", b)
	}
}

func TestEmptyIndex(t *testing.T) {
	ix := Build(nil)
	if len(ix.Blocks(Filter{})) != 0 || ix.Meta.Rows != 0 || ix.Meta.DateMin != "" {
		t.Errorf("%+v", ix.Meta)
	}
	rep := ix.Meta.Report() // must not divide by zero
	if !strings.Contains(rep, "Rows read:") || strings.Contains(rep, "NaN") || strings.Contains(rep, "Inf") {
		t.Errorf("report: %s", rep)
	}
}

func TestReportContents(t *testing.T) {
	recs := []Record{rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019), rec("2", "Barricade", "VOID", "E A ST", 100, 2020)}
	recs[0].Type, recs[1].Type, recs[0].SubType = "EXCAVATION", "BARRICADE", "CURB"
	rep := Build(recs).Meta.Report()
	for _, want := range []string{"Unique permits:       2", "2019-06-01 to 2020-06-01", "Void/withdrawn/etc.:  1 (50.0%)", "Permit status", "CLOSED", "VOID", "PERMIT_TYPE values", "EXCAVATION", "SUB_TYPE values", "CURB", "Date field used", "DATE_ISSUED"} {
		if !strings.Contains(rep, want) {
			t.Errorf("report missing %q:\n%s", want, rep)
		}
	}
}

func TestRealSampleEndToEnd(t *testing.T) {
	raw, err := os.ReadFile("testdata/real_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	var recs []Record
	for i, p := range rows {
		// the sample was fetched without geometry; place each row at a distinct point
		geom := fmt.Sprintf(`{"type":"Point","coordinates":[%f,32.2]}`, -110.9-float64(i)/100)
		recs = append(recs, FromFeature(arcgis.Feature{Properties: p, Geometry: json.RawMessage(geom)}, "permits_inactive"))
	}
	m := Build(recs).Meta
	if m.UniquePermits != 10 || m.DuplicateRows != 0 || m.NoWork != 0 || m.NoDate != 0 || m.BadDates != 0 {
		t.Errorf("basic counts: %+v", m)
	}
	if m.NoBlock != 2 { // the two intersections ("GOLF LINKS RD & CATHY DR", "HEMISPHERE PL & HEMISPHERE LP")
		t.Errorf("NoBlock = %d, want 2", m.NoBlock)
	}
	if m.StreetFromText != 1 { // "10602 E CERULEAN WAY" read from LOCATION
		t.Errorf("StreetFromText = %d, want 1", m.StreetFromText)
	}
	if m.Blocks != 8 || m.DateMin != "2005-07-07" || m.DateMax != "2006-10-02" {
		t.Errorf("blocks=%d range=%s..%s", m.Blocks, m.DateMin, m.DateMax)
	}
	if len(m.Categories) != 1 || m.Categories[0].Key != "Excavation" || m.Categories[0].N != 10 {
		t.Errorf("categories = %+v", m.Categories)
	}
	if len(m.Statuses) != 1 || m.Statuses[0].Key != "CLOSED" {
		t.Errorf("statuses = %+v", m.Statuses)
	}
	ix := Build(recs)
	want := []string{"E GAS RD|3400", "W NAVAJO RD|100", "S PARK AVE|1400", "E SPEEDWAY BLVD|700", "S WILLIAMS BLVD|200", "S EASTSIDE LOOP|1400", "E CERULEAN WAY|10600", "E REES LOOP|10200"}
	for _, k := range want {
		d, ok := ix.Block(k, Filter{})
		if !ok || d.Count != 1 {
			t.Errorf("block %q: %+v %v", k, d, ok)
		}
	}
	if d, _ := ix.Block("S WILLIAMS BLVD|200", Filter{}); d.Records[0].SubType != "CURB" || d.Records[0].ProURL == "" {
		t.Errorf("%+v", d.Records[0])
	}
}

func TestBuildAtRealScale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test")
	}
	// The inactive layer has 92,323 rows
	recs := make([]Record, 0, 95000)
	for i := 0; i < 92323; i++ {
		recs = append(recs, rec(fmt.Sprintf("P%d", i), []string{"Excavation", "Barricade", "Combo"}[i%3], "CLOSED", fmt.Sprintf("E ST%d", i%900), (i%40)*100, 1996+i%30))
	}
	start := time.Now()
	ix := Build(recs)
	if bs := ix.Blocks(Filter{From: 2000, To: 2020}); len(bs) == 0 {
		t.Fatal("no blocks")
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Errorf("Build+Blocks on 92k rows took %v", el)
	}
}

func TestBoundsFilterAndMetaBounds(t *testing.T) {
	at := func(permit string, lat, lng float64, block int) Record {
		r := rec(permit, "Excavation", "CLOSED", "E A ST", block, 2020)
		r.Lat, r.Lng = lat, lng
		return r
	}
	ix := Build([]Record{at("1", 32.20, -110.90, 100), at("2", 32.25, -110.95, 200), at("3", 32.30, -111.00, 300), at("4", 40.0, -100.0, 400)})
	keys := func(f Filter) string {
		var out []string
		for _, b := range ix.Blocks(f) {
			out = append(out, b.Key)
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	cases := []struct {
		name string
		b    *Bounds
		want string
	}{
		{"no bounds", nil, "E A ST|100,E A ST|200,E A ST|300,E A ST|400"},
		{"around Tucson", &Bounds{32.1, -111.1, 32.4, -110.8}, "E A ST|100,E A ST|200,E A ST|300"},
		{"edges are inclusive", &Bounds{32.20, -110.90, 32.20, -110.90}, "E A ST|100"},
		{"just outside", &Bounds{32.2001, -110.9, 32.4, -110.8}, ""},
		{"empty region", &Bounds{0, 0, 1, 1}, ""},
		{"only the far outlier", &Bounds{39, -101, 41, -99}, "E A ST|400"},
	}
	for _, c := range cases {
		if got := keys(Filter{Bounds: c.b}); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	// Meta.Bounds uses percentiles, which for 4 points stays inside the data
	b := ix.Meta.Bounds
	if b[0] < 32.0 || b[2] > 40.0 || b[0] > b[2] || b[1] > b[3] {
		t.Errorf("meta bounds = %v", b)
	}
	if Build(nil).Meta.Bounds != [4]float64{} {
		t.Error("no data should give zero bounds")
	}
}

func TestMetaBoundsIgnoreStrayGeocodes(t *testing.T) {
	var recs []Record
	for i := 0; i < 500; i++ {
		r := rec(fmt.Sprintf("P%d", i), "Excavation", "CLOSED", "E A ST", 100+i*100, 2020)
		r.Lat, r.Lng = 32.1+float64(i%50)/200, -111.0+float64(i%40)/100
		recs = append(recs, r)
	}
	stray := rec("STRAY", "Excavation", "CLOSED", "E Z ST", 100, 2020)
	stray.Lat, stray.Lng = 64.0, -150.0 // a bad geocode far from everything
	b := Build(append(recs, stray)).Meta.Bounds
	if b[2] > 33 || b[1] < -112 {
		t.Errorf("one stray point stretched the bounds: %v", b)
	}
}

func TestBlockDetailFlagsNoWorkRecords(t *testing.T) {
	var recs []Record
	for i, st := range []string{"CLOSED", "VOID", "WITHDRWN", "DENIED", "Canceled", "FINAL", "EXPIRED"} {
		recs = append(recs, rec(fmt.Sprintf("P%d", i), "Excavation", st, "E A ST", 100, 2020))
	}
	d, _ := Build(recs).Block("E A ST|100", Filter{NoWork: true})
	flags := map[string]bool{}
	for _, r := range d.Records {
		flags[r.Status] = r.NoWorkStatus
	}
	want := map[string]bool{"CLOSED": false, "VOID": true, "WITHDRWN": true, "DENIED": true, "Canceled": true, "FINAL": false, "EXPIRED": false}
	for st, w := range want {
		if flags[st] != w {
			t.Errorf("status %q: no_work = %v, want %v", st, flags[st], w)
		}
	}
	if d, _ := Build(recs).Block("E A ST|100", Filter{}); d.Count != 3 {
		t.Errorf("default filter kept %d records, want 3 (CLOSED, FINAL, EXPIRED)", d.Count)
	}
	b, _ := json.Marshal(d.Records[0])
	if strings.Contains(string(b), `"no_work":false`) {
		t.Errorf("false flag should be omitted: %s", b)
	}
}

func TestLayerStatsExposeOverlapBetweenLayers(t *testing.T) {
	mk := func(layer, permit string, year int) Record {
		r := rec(permit, "Excavation", "CLOSED", "E A ST", 100, year)
		r.Layer = layer
		return r
	}
	recs := []Record{
		mk("old", "P1", 2019), mk("old", "P2", 2020), mk("old", "P2", 2020), // P2 repeated inside one layer
		mk("new", "P1", 2019), mk("new", "P2", 2020), mk("new", "P3", 2026), mk("new", "P4", 2026), // P1, P2 already seen in "old"
	}
	m := Build(recs).Meta
	want := []LayerStat{{"old", 3, 2, 1}, {"new", 4, 2, 2}}
	if len(m.Layers) != 2 || m.Layers[0] != want[0] || m.Layers[1] != want[1] {
		t.Errorf("layers = %+v, want %+v", m.Layers, want)
	}
	if m.Rows != 7 || m.UniquePermits != 4 || m.DuplicateRows != 3 {
		t.Errorf("totals: %+v", m)
	}
	var rows, kept, dupes int
	for _, l := range m.Layers {
		rows, kept, dupes = rows+l.Rows, kept+l.Kept, dupes+l.Dupes
		if l.Rows != l.Kept+l.Dupes {
			t.Errorf("%s: rows != kept + dupes", l.Layer)
		}
	}
	if rows != m.Rows || kept != m.UniquePermits || dupes != m.DuplicateRows {
		t.Errorf("per-layer sums %d/%d/%d do not match totals", rows, kept, dupes)
	}
}

func TestPerYearCounts(t *testing.T) {
	recs := []Record{
		rec("A", "Excavation", "CLOSED", "E A ST", 100, 2026), rec("B", "Excavation", "CLOSED", "E A ST", 100, 1996),
		rec("C", "Excavation", "CLOSED", "E A ST", 100, 2026), rec("C", "Excavation", "CLOSED", "E A ST", 100, 2026), // repeat
		rec("D", "Excavation", "CLOSED", "E A ST", 100, 0), // undated: not in any year
		rec("E", "Excavation", "CLOSED", "E A ST", 100, 2010),
	}
	got := Build(recs).Meta.PerYear
	want := []KV{{"1996", 1}, {"2010", 1}, {"2026", 2}}
	if len(got) != len(want) {
		t.Fatalf("per year = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("per year[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDiagnosticsAreNeverNullInJSONAndShowInTheReport(t *testing.T) {
	b, _ := json.Marshal(Build(nil).Meta)
	if strings.Contains(string(b), "null") {
		t.Errorf("empty meta has null: %s", b)
	}
	r := rec("A", "Excavation", "CLOSED", "E A ST", 100, 2024)
	r.Layer = "permits_row_general"
	rep := Build([]Record{r}).Meta.Report()
	for _, want := range []string{"Source layers", "permits_row_general", "Permits per year", "2024"} {
		if !strings.Contains(rep, want) {
			t.Errorf("report missing %q:\n%s", want, rep)
		}
	}
}

func TestEmptyCategorySetMatchesNothing(t *testing.T) {
	ix := Build([]Record{rec("1", "Excavation", "CLOSED", "E A ST", 100, 2020)})
	if n := len(ix.Blocks(Filter{Cats: map[string]bool{}})); n != 0 {
		t.Errorf("empty set: %d blocks", n)
	}
	if n := len(ix.Blocks(Filter{})); n != 1 {
		t.Errorf("nil set (all categories): %d blocks", n)
	}
}
