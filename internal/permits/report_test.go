package permits

import (
	"encoding/json"
	"fmt"
	"os"
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

func summarize(recs []Record) Meta {
	kept, st := Dedupe(recs, nil)
	_, unplaced := AssignPlaces(kept)
	return Summarize(kept, st, unplaced)
}

func TestSummary(t *testing.T) {
	bad := rec("B", "Excavation", "CLOSED", "E A ST", 100, 0)
	bad.DateFlag = "DATE_ISSUED 2049-09-03 is implausible"
	text := rec("T", "Excavation", "CLOSED", "E A ST", 100, 2020)
	text.StreetSource = "LOCATION"
	nogeo := rec("G", "Barricade", "CLOSED", "E A ST", 200, 2021)
	nogeo.HasGeo = false
	noblock := rec("N", "Combo", "CLOSED", "", -1, 2021)
	m := summarize([]Record{
		rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019),
		rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019), // repeat row
		rec("1", "Excavation", "FINAL", "E A ST", 100, 2019),  // and again, with another status
		rec("2", "Excavation", "VOID", "E A ST", 100, 2025),
		bad, text, nogeo, noblock,
	})
	checks := map[string][2]int{
		"Rows": {m.Rows, 8}, "UniquePermits": {m.UniquePermits, 6}, "DuplicateRows": {m.DuplicateRows, 2},
		"StatusDiffered": {m.StatusDiffered, 1}, "NoWork": {m.NoWork, 1}, "NoDate": {m.NoDate, 1},
		"BadDates": {m.BadDates, 1}, "StreetFromText": {m.StreetFromText, 1},
		"NoBlock": {m.NoBlock, 1}, "Unplaced": {m.Unplaced, 1},
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

func TestReportContents(t *testing.T) {
	recs := []Record{rec("1", "Excavation", "CLOSED", "E A ST", 100, 2019), rec("2", "Barricade", "VOID", "E A ST", 100, 2020)}
	recs[0].Type, recs[1].Type, recs[0].SubType = "EXCAVATION", "BARRICADE", "CURB"
	recs[0].Layer, recs[1].Layer = "permits_row_general", "permits_row_general"
	rep := summarize(recs).Report()
	for _, want := range []string{
		"Unique permits:       2", "2019-06-01 to 2020-06-01", "Void/withdrawn/etc.:  1 (50.0%)",
		"Permit status", "CLOSED", "VOID", "Source layers", "permits_row_general", "Permits per year", "2019",
		"CURB", "Not on the map:       0",
	} {
		if !strings.Contains(rep, want) {
			t.Errorf("report missing %q:\n%s", want, rep)
		}
	}
	if strings.Contains(rep, "Unknown permit types") {
		t.Errorf("no unknown types, so no section:\n%s", rep)
	}
}

func TestLayerStats(t *testing.T) {
	mk := func(layer, permit string) Record {
		r := rec(permit, "Excavation", "CLOSED", "E A ST", 100, 2020)
		r.Layer = layer
		return r
	}
	m := summarize([]Record{mk("old", "P1"), mk("old", "P2"), mk("new", "P1"), mk("new", "P3")})
	want := []LayerStat{{"old", 2, 2, 0}, {"new", 2, 1, 1}} // no priorities: the first copy read stays
	if len(m.Layers) != 2 || m.Layers[0] != want[0] || m.Layers[1] != want[1] {
		t.Errorf("layers = %+v, want %+v", m.Layers, want)
	}
}

func TestPerYearCounts(t *testing.T) {
	got := summarize([]Record{
		rec("A", "Excavation", "CLOSED", "E A ST", 100, 2026), rec("B", "Excavation", "CLOSED", "E A ST", 100, 1996),
		rec("C", "Excavation", "CLOSED", "E A ST", 100, 2026), rec("C", "Excavation", "CLOSED", "E A ST", 100, 2026), // repeat
		rec("D", "Excavation", "CLOSED", "E A ST", 100, 0), // undated: not in any year
		rec("E", "Excavation", "CLOSED", "E A ST", 100, 2010),
	}).PerYear
	want := []KV{{"1996", 1}, {"2010", 1}, {"2026", 2}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("per year = %+v, want %+v", got, want)
	}
}

func TestRealSampleSummary(t *testing.T) {
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
		geom := fmt.Sprintf(`{"type":"Point","coordinates":[%f,32.2]}`, -110.9-float64(i)/100)
		recs = append(recs, FromFeature(arcgis.Feature{Properties: p, Geometry: json.RawMessage(geom)}, "permits_inactive"))
	}
	m := summarize(recs)
	if m.UniquePermits != 10 || m.DuplicateRows != 0 || m.NoWork != 0 || m.NoDate != 0 || m.BadDates != 0 || m.Unplaced != 0 {
		t.Errorf("basic counts: %+v", m)
	}
	if m.NoBlock != 2 { // the two intersections
		t.Errorf("NoBlock = %d, want 2", m.NoBlock)
	}
	if m.StreetFromText != 1 { // "10602 E CERULEAN WAY" read from LOCATION
		t.Errorf("StreetFromText = %d, want 1", m.StreetFromText)
	}
	if m.DateMin != "2005-07-07" || m.DateMax != "2006-10-02" {
		t.Errorf("range=%s..%s", m.DateMin, m.DateMax)
	}
	if len(m.Categories) != 1 || m.Categories[0] != (KV{"Excavation", 10}) || len(m.UnknownTypes) != 0 {
		t.Errorf("categories = %+v, unknown = %+v", m.Categories, m.UnknownTypes)
	}
}

func TestPlaceBounds(t *testing.T) {
	var places []Place
	for i := 0; i < 500; i++ {
		places = append(places, Place{Lat: 32.1 + float64(i%50)/200, Lng: -111.0 + float64(i%40)/100})
	}
	b := PlaceBounds(append(places, Place{Lat: 64.0, Lng: -150.0})) // one bad geocode far from everything
	if b[2] > 33 || b[1] < -112 {
		t.Errorf("one stray point stretched the bounds: %v", b)
	}
	if b[0] < 32.0 || b[0] > b[2] || b[1] > b[3] {
		t.Errorf("bounds = %v", b)
	}
	if PlaceBounds(nil) != [4]float64{} {
		t.Error("no places should give zero bounds")
	}
}
