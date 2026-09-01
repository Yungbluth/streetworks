package site

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/permits"
)

var update = flag.Bool("update", false, "rewrite testdata/golden from the current output")

func TestCellKey(t *testing.T) {
	for _, c := range []struct {
		lat, lng float64
		want     string
	}{
		{32.2215, -110.97, "3222_-11097"},
		{32.2215, -110.96999, "3222_-11097"},
		{32.23, -110.9701, "3223_-11098"},
		{32.0, -111.0, "3200_-11100"},
		{-0.001, 0.001, "-1_0"},
	} {
		if got := CellKey(c.lat, c.lng); got != c.want {
			t.Errorf("CellKey(%v, %v) = %q, want %q", c.lat, c.lng, got, c.want)
		}
	}
}

func sampleInput(t *testing.T) Input {
	t.Helper()
	raw, err := os.ReadFile("../permits/testdata/real_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	var recs []permits.Record
	for i, p := range rows {
		geom := fmt.Sprintf(`{"type":"Point","coordinates":[%.5f,%.5f]}`, -110.9-float64(i)*0.004, 32.2+float64(i%3)*0.006)
		recs = append(recs, permits.FromFeature(arcgis.Feature{Properties: p, Geometry: json.RawMessage(geom)}, "permits_inactive"))
	}
	recs = append(recs, permits.Record{Permit: "NOGEO", Category: "Other", Status: "CLOSED", Block: -1}) // stays off the map
	kept, st := permits.Dedupe(recs, nil)
	places, unplaced := permits.AssignPlaces(kept)
	built := time.Date(2026, 8, 29, 18, 0, 0, 0, time.UTC)
	return Input{
		Records: kept, Places: places, Meta: permits.Summarize(kept, st, unplaced),
		Sources: []arcgis.SourceMeta{{Name: "permits_inactive", URL: "https://example.test/75", FetchedAt: built, Count: 10}},
		Overlays: []Overlay{{Name: "prop409_major", Label: "Prop 409 major streets", Features: []arcgis.Feature{
			{Properties: map[string]any{"NAME": "Broadway"}, Geometry: json.RawMessage(`{"type":"LineString","coordinates":[[-110.9,32.2],[-110.8,32.2]]}`)},
		}}},
		Basemap: DefaultBasemap(),
		Built:   built,
	}
}
