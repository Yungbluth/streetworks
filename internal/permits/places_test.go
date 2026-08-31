package permits

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/yungbluth/streetworks/internal/arcgis"
)

func TestParseIntersection(t *testing.T) {
	ok := map[string][2]string{
		"GOLF LINKS RD &  CATHY DR ":    {"CATHY DR", "GOLF LINKS RD"},
		"cathy dr & golf links road":    {"CATHY DR", "GOLF LINKS RD"},
		"SPEEDWAY BL & KOLB RD":         {"KOLB RD", "SPEEDWAY BLVD"},
		"KOLB RD and SPEEDWAY":          {"KOLB RD", "SPEEDWAY"},
		"E SPEEDWAY BL & N KOLB RD":     {"KOLB RD", "SPEEDWAY BLVD"}, // directions dropped so spellings meet
		"CONGRESS RD / CENTRAL AV":      {"CENTRAL AVE", "CONGRESS RD"},
		"GOLF LINKS &GOLF LINKS CI":     {"GOLF LINKS", "GOLF LINKS CIR"},
		"6th St and 4th Ave":            {"4TH AVE", "6TH ST"},
		"HEMISPHERE PL & HEMISPHERE LP": {"HEMISPHERE LOOP", "HEMISPHERE PL"},
	}
	for in, want := range ok {
		a, b, good := ParseIntersection(in)
		if !good || [2]string{a, b} != want {
			t.Errorf("ParseIntersection(%q) = %q, %q, %v; want %v", in, a, b, good, want)
		}
	}
	for _, in := range []string{
		"4700 S", "1100N", "7550 S ?", "", "   ", "A & B & C", "SPEEDWAY BL & KOLB RD & 3RD ST",
		"& KOLB", "KOLB &", "1054 E IRWIN PL", "1200 E SPEEDWAY & KOLB", "KOLB & KOLB", "123 & 456",
	} {
		if a, b, good := ParseIntersection(in); good {
			t.Errorf("ParseIntersection(%q) = %q, %q; want not ok", in, a, b)
		}
	}
}

func at(r Record, lat, lng float64) Record {
	r.Lat, r.Lng, r.HasGeo = lat, lng, true
	return r
}

func TestAssignPlaces(t *testing.T) {
	recs := []Record{
		at(Record{Permit: "B1", Street: "E SPEEDWAY BLVD", Block: 1200}, 32.236, -110.954),
		at(Record{Permit: "X1", Block: -1, Location: "GOLF LINKS RD & CATHY DR"}, 32.20, -110.90),
		at(Record{Permit: "X2", Block: -1, Location: "cathy dr & golf links road"}, 32.20, -110.90),
		at(Record{Permit: "P1", Block: -1, Location: "4700 S "}, 32.18, -110.95),
		{Permit: "N1", Block: -1, Location: "KOLB & GRANT"}, // no coordinates
	}
	places, unplaced := AssignPlaces(recs)
	if unplaced != 1 || len(places) != 3 {
		t.Fatalf("places=%+v unplaced=%d", places, unplaced)
	}
	block, cross, point := places[0], places[1], places[2]
	if block.Kind != KindBlock || block.Street != "E SPEEDWAY BLVD" || block.Block != 1200 || len(block.Permits) != 1 || block.Permits[0] != 0 {
		t.Errorf("block: %+v", block)
	}
	if cross.Kind != KindIntersection || cross.Street != "CATHY DR" || cross.Cross != "GOLF LINKS RD" || cross.Block != -1 || len(cross.Permits) != 2 {
		t.Errorf("intersection: %+v", cross)
	}
	if point.Kind != KindPoint || point.Label != "4700 S" || point.Permits[0] != 3 {
		t.Errorf("point: %+v", point)
	}
}

func TestSamePoint(t *testing.T) {
	places, _ := AssignPlaces([]Record{
		at(Record{Block: -1, Location: "4700 S"}, 32.18000, -110.95000),
		at(Record{Block: -1, Location: " 4700 s "}, 32.18002, -110.95003), // a few metres away
		at(Record{Block: -1, Location: "4700 S"}, 32.30, -110.80),         // same text, elsewhere
		at(Record{Block: -1}, 32.25, -110.85),                             // no text at all
	})
	if len(places) != 3 || len(places[0].Permits)+len(places[1].Permits)+len(places[2].Permits) != 4 {
		t.Fatalf("%+v", places)
	}
	for _, p := range places {
		if p.Label == "" {
			t.Errorf("every point needs a label: %+v", p)
		}
	}
}

func TestPlacePosition(t *testing.T) {
	places, _ := AssignPlaces([]Record{
		at(Record{Street: "E A ST", Block: 100}, 32.0, -111.0),
		at(Record{Street: "E A ST", Block: 100}, 32.2, -110.8),
	})
	if math.Abs(places[0].Lat-32.1) > 1e-9 || math.Abs(places[0].Lng+110.9) > 1e-9 {
		t.Errorf("%+v", places[0])
	}
}

func TestRealPlaces(t *testing.T) {
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
		geom := json.RawMessage(`{"type":"Point","coordinates":[` + ftoa(-110.9-float64(i)/100) + `,32.2]}`)
		recs = append(recs, FromFeature(arcgis.Feature{Properties: p, Geometry: geom}, "permits_inactive"))
	}
	places, unplaced := AssignPlaces(recs)
	kinds := map[PlaceKind]int{}
	for _, p := range places {
		kinds[p.Kind]++
	}
	// ten rows: eight have a street block, two only name a crossing
	if unplaced != 0 || kinds[KindBlock] != 8 || kinds[KindIntersection] != 2 || kinds[KindPoint] != 0 {
		t.Errorf("kinds=%v unplaced=%d", kinds, unplaced)
	}
}
