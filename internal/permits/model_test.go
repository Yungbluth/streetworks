package permits

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
)

func props(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func ms(y int, m time.Month, d int) float64 {
	return float64(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).UnixMilli())
}

func TestProURL(t *testing.T) {
	cases := map[string]string{
		"https://www.tucsonaz.gov/pro/pdsd/addresspermits/3401/E/GAS RD": "https://www.tucsonaz.gov/pro/pdsd/addresspermits/3401/E/GAS RD",
		"http://example.com/x": "http://example.com/x",
		"javascript:alert(1)":  "",
		"JAVASCRIPT:alert(1)":  "",
		"data:text/html,<b>":   "",
		"ftp://example.com/x":  "",
		"//evil.example/x":     "",
		"/relative/path":       "",
		"":                     "",
	}
	for in, want := range cases {
		if got := FromFeature(arcgis.Feature{Properties: props("PRO_URL", in)}, "t").ProURL; got != want {
			t.Errorf("PRO_URL %q -> %q, want %q", in, got, want)
		}
	}
}

func TestDateFields(t *testing.T) {
	cases := []struct {
		name      string
		p         map[string]any
		wantBasis string
		wantDate  string
	}{
		{"issued wins", props("DATE_ISSUED", ms(2020, 3, 5), "DATE_APPROVE", ms(2019, 1, 1), "DATE_ENTERED", ms(2018, 1, 1)), "DATE_ISSUED", "2020-03-05"},
		{"approve next", props("DATE_APPROVE", ms(2019, 1, 1), "DATE_START", ms(2018, 1, 1)), "DATE_APPROVE", "2019-01-01"},
		{"start next", props("DATE_START", ms(2018, 7, 9), "DATE_ENTERED", ms(2017, 1, 1)), "DATE_START", "2018-07-09"},
		{"entered last", props("DATE_ENTERED", ms(2017, 12, 31)), "DATE_ENTERED", "2017-12-31"},
		{"null skipped", props("DATE_ISSUED", nil, "DATE_ENTERED", ms(2017, 1, 2)), "DATE_ENTERED", "2017-01-02"},
		{"string ignored", props("DATE_ISSUED", "2020-01-01", "DATE_ENTERED", ms(2017, 1, 2)), "DATE_ENTERED", "2017-01-02"},
		{"none", props("DATE_FINAL", ms(2020, 1, 1)), "", ""}, // DATE_FINAL is not a "when"
	}
	for _, c := range cases {
		r := FromFeature(arcgis.Feature{Properties: c.p}, "t")
		if r.DateBasis != c.wantBasis || r.Date != c.wantDate {
			t.Errorf("%s: basis=%q date=%q, want %q %q", c.name, r.DateBasis, r.Date, c.wantBasis, c.wantDate)
		}
	}
}

func TestStreetFromText(t *testing.T) {
	// Structured fields win even when LOCATION holds a different address
	r := FromFeature(arcgis.Feature{Properties: props("STREET_NO", "100", "STREET_DIR", "E", "STREET_NAME", "A ST", "LOCATION", "999 W B ST")}, "t")
	if r.Street != "E A ST" || r.Block != 100 || r.StreetSource != "" {
		t.Errorf("%+v", r)
	}
	// ADDRESS is tried before LOCATION
	r = FromFeature(arcgis.Feature{Properties: props("ADDRESS", "300 N C AV", "LOCATION", "999 W B ST")}, "t")
	if r.Street != "N C AVE" || r.Block != 300 || r.StreetSource != "ADDRESS" || r.RawStreet != "300 N C AV" {
		t.Errorf("%+v", r)
	}
	r = FromFeature(arcgis.Feature{Properties: props("ADDRESS", "UNKNOWN", "LOCATION", "999 W B ST")}, "t")
	if r.Street != "W B ST" || r.StreetSource != "LOCATION" {
		t.Errorf("%+v", r)
	}
}

func TestAddressFromText(t *testing.T) {
	ok := map[string][3]string{
		"10602 E CERULEAN WAY":  {"10602", "E", "CERULEAN WAY"},
		"6200 S. HEMISPHERE PL": {"6200", "S", "HEMISPHERE PL"},
		"  1054  e  irwin pl  ": {"1054", "E", "IRWIN PL"},
		"250 S WILLIAMS BL ":    {"250", "S", "WILLIAMS BL"},
		"1 N 1ST AVE":           {"1", "N", "1ST AVE"},
	}
	for in, want := range ok {
		n, d, name, good := addressFromText(in)
		if !good || n != want[0] || d != want[1] || name != want[2] {
			t.Errorf("addressFromText(%q) = %q %q %q %v", in, n, d, name, good)
		}
	}
	for _, in := range []string{
		"", "4700 S ", "4700 S", "1100N", "7825E", "7550 S ?", "GOLF LINKS RD &  CATHY DR ",
		"HEMISPHERE PL & HEMISPHERE LP", "2300 E A ST & B ST", "2300 E A/B ST", "E 2300", "NO NUMBER HERE",
		"12 E 34", // no letters in the name
	} {
		if _, _, _, good := addressFromText(in); good {
			t.Errorf("addressFromText(%q) should be rejected", in)
		}
	}
}

func TestFinalDate(t *testing.T) {
	r := FromFeature(arcgis.Feature{Properties: props("DATE_FINAL", ms(2021, 4, 2), "DATE_ISSUED", ms(2021, 1, 1))}, "t")
	if r.Final != "2021-04-02" || r.Date != "2021-01-01" {
		t.Errorf("%+v", r)
	}
}

func TestNoWork(t *testing.T) {
	noWork := []string{"VOID", "Void", "void", "  VOID ", "WITHDRWN", "Withdrawn", "WITHDRAWN", "Canceled", "CANCELLED", "DENIED", "REVOKED",
		"In Review", "Needs Resubmittal", "Submitted", "Submitted - Online", "In Revision", "Awaiting Customer Attention", "Fees Due", "IN REVIEW"}
	work := []string{"CLOSED", "FINAL", "final", "Complete", "EXPIRED", "Expired", "ISSUED", "Issued", "VOIDED", "AVOID", "WITHDRAWAL", "",
		// work may have happened under these
		"Inspections", "Inspections Complete", "Approved", "On Hold", "Fees Paid", "Stop Work Order", "Renewal/Extension Fees Due"}
	for _, s := range noWork {
		if r := FromFeature(arcgis.Feature{Properties: props("PERMIT_STATUS", s)}, "t"); !r.NoWork() {
			t.Errorf("status %q should be a no-work status", s)
		}
	}
	for _, s := range work {
		if r := FromFeature(arcgis.Feature{Properties: props("PERMIT_STATUS", s)}, "t"); r.NoWork() {
			t.Errorf("status %q should not be a no-work status", s)
		}
	}
}

func TestCentroid(t *testing.T) {
	raw := func(s string) json.RawMessage { return json.RawMessage(s) }
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	type want struct {
		lat, lng float64
		ok       bool
	}
	cases := []struct {
		name string
		g    json.RawMessage
		w    want
	}{
		{"point", raw(`{"type":"Point","coordinates":[-110.97,32.22]}`), want{32.22, -110.97, true}},
		{"point with z", raw(`{"type":"Point","coordinates":[-110.97,32.22,700]}`), want{32.22, -110.97, true}},
		{"square", raw(`{"type":"Polygon","coordinates":[[[-110.98,32.22],[-110.97,32.22],[-110.97,32.23],[-110.98,32.23],[-110.98,32.22]]]}`), want{32.225, -110.975, true}},
		{"square, ring not closed", raw(`{"type":"Polygon","coordinates":[[[-110.98,32.22],[-110.97,32.22],[-110.97,32.23],[-110.98,32.23]]]}`), want{32.225, -110.975, true}},
		{"square, clockwise", raw(`{"type":"Polygon","coordinates":[[[-110.98,32.22],[-110.98,32.23],[-110.97,32.23],[-110.97,32.22],[-110.98,32.22]]]}`), want{32.225, -110.975, true}},
		{"triangle centroid is the vertex mean", raw(`{"type":"Polygon","coordinates":[[[0.3,10],[0.6,10],[0.3,10.3],[0.3,10]]]}`), want{10.1, 0.4, true}},
		{"hole ignored", raw(`{"type":"Polygon","coordinates":[[[-110.98,32.22],[-110.97,32.22],[-110.97,32.23],[-110.98,32.23],[-110.98,32.22]],[[-110.976,32.224],[-110.974,32.224],[-110.974,32.226],[-110.976,32.226],[-110.976,32.224]]]}`), want{32.225, -110.975, true}},
		{"multipolygon uses first polygon", raw(`{"type":"MultiPolygon","coordinates":[[[[-110.98,32.22],[-110.97,32.22],[-110.97,32.23],[-110.98,32.23],[-110.98,32.22]]],[[[-100,40],[-99,40],[-99,41],[-100,41],[-100,40]]]]}`), want{32.225, -110.975, true}},
		{"collinear ring falls back to vertex mean", raw(`{"type":"Polygon","coordinates":[[[-110.9,32.2],[-110.8,32.2],[-110.7,32.2]]]}`), want{32.2, -110.8, true}},
		{"line", raw(`{"type":"LineString","coordinates":[[-110.9,32.2],[-110.7,32.4]]}`), want{32.3, -110.8, true}},
		{"null", raw(`null`), want{}},
		{"empty", nil, want{}},
		{"truncated json", raw(`{"type":"Poi`), want{}},
		{"empty polygon", raw(`{"type":"Polygon","coordinates":[]}`), want{}},
		{"empty ring", raw(`{"type":"Polygon","coordinates":[[]]}`), want{}},
		{"point with one coordinate", raw(`{"type":"Point","coordinates":[1]}`), want{}},
		{"unsupported type", raw(`{"type":"GeometryCollection","geometries":[]}`), want{}},
		{"wrong coordinate nesting", raw(`{"type":"Polygon","coordinates":[1,2,3]}`), want{}},
		{"latitude out of range", raw(`{"type":"Point","coordinates":[-110,95]}`), want{}},
		{"longitude out of range", raw(`{"type":"Point","coordinates":[-190,32]}`), want{}},
		{"state-plane feet mistaken for degrees", raw(`{"type":"Point","coordinates":[1100000,480000]}`), want{}},
		{"null island is a failed geocode", raw(`{"type":"Point","coordinates":[0,0]}`), want{}},
		{"empty vertex must not panic (fuzz regression; lat 100 is out of range)", raw(`{"type":"Polygon","coordinates":[[[0,100,0,100],[0,100,0,100],[]]]}`), want{}},
		{"short vertices among good ones", raw(`{"type":"Polygon","coordinates":[[[-110.98,32.22],[-110.97,32.22],[5],[-110.97,32.23],[],[-110.98,32.23]]]}`), want{32.225, -110.975, true}},
		{"overflowing coordinates cannot yield NaN", raw(`{"type":"Polygon","coordinates":[[[1e308,1e308],[-1e308,1e308],[-1e308,-1e308],[1e308,-1e308]]]}`), want{}},
	}
	for _, c := range cases {
		lat, lng, ok := Centroid(c.g)
		if ok != c.w.ok || (ok && (!near(lat, c.w.lat) || !near(lng, c.w.lng))) {
			t.Errorf("%s: got %v,%v ok=%v; want %+v", c.name, lat, lng, ok, c.w)
		}
	}
}

func ftoa(f float64) string { b, _ := json.Marshal(f); return string(b) }
