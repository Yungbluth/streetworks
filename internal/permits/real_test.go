package permits

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
)

// rows below are copied from the City of Tucson TDOT_PERMITS_RW_INACTIVE layer
func realFeat(p map[string]any) arcgis.Feature {
	return arcgis.Feature{Properties: p, Geometry: json.RawMessage(`null`)}
}

func TestRealRows(t *testing.T) {
	base := func(over map[string]any) map[string]any {
		p := map[string]any{"PERMIT_TYPE": "EXCAVATION", "PERMIT_STATUS": "CLOSED", "DATE_ISSUED": 1158883200000.0}
		for k, v := range over {
			p[k] = v
		}
		return p
	}
	cases := []struct {
		name        string
		in          map[string]any
		street      string
		block       int
		source, cat string
	}{
		{"BL suffix", base(map[string]any{"PERMIT": "T06EX01005", "SUB_TYPE": "SIDE    ", "SUBTYPE_DESC": "SIDEWALK INSTALLATION", "STREET_NO": "749", "STREET_DIR": "E", "STREET_NAME": "SPEEDWAY BL"}), "E SPEEDWAY BLVD", 700, "", "Excavation"},
		{"AV suffix", base(map[string]any{"STREET_NO": "1421", "STREET_DIR": "S", "STREET_NAME": "PARK AV"}), "S PARK AVE", 1400, "", "Excavation"},
		{"LP suffix", base(map[string]any{"STREET_NO": "1490", "STREET_DIR": "S", "STREET_NAME": "EASTSIDE LP"}), "S EASTSIDE LOOP", 1400, "", "Excavation"},
		{"ordinal street", base(map[string]any{"STREET_NO": "428", "STREET_DIR": "S", "STREET_NAME": "4TH AV"}), "S 4TH AVE", 400, "", "Excavation"},
		{"address only in LOCATION", base(map[string]any{"LOCATION": "10602 E CERULEAN WAY", "ADDRESS": nil}), "E CERULEAN WAY", 10600, "LOCATION", "Excavation"},
		{"address with trailing space", base(map[string]any{"LOCATION": "1054 E IRWIN PL "}), "E IRWIN PL", 1000, "LOCATION", "Excavation"},
		{"intersection stays unplaced", base(map[string]any{"LOCATION": "GOLF LINKS RD &  CATHY DR "}), "", -1, "", "Excavation"},
		{"grid reference stays unplaced", base(map[string]any{"LOCATION": "4700 S "}), "", -1, "", "Excavation"},
	}
	for _, c := range cases {
		r := FromFeature(realFeat(c.in), "t")
		if r.Street != c.street || r.Block != c.block || r.StreetSource != c.source || r.Category != c.cat {
			t.Errorf("%s: got street=%q block=%d source=%q cat=%q; want %q %d %q %q", c.name, r.Street, r.Block, r.StreetSource, r.Category, c.street, c.block, c.source, c.cat)
		}
	}
}

func TestPaddedSubtypeAndStatus(t *testing.T) {
	r := FromFeature(realFeat(map[string]any{"PERMIT_TYPE": "EXCAVATION", "SUB_TYPE": "CURB    ", "PERMIT_STATUS": "CLOSED", "DATE_ISSUED": 1120694400000.0}), "t")
	if r.SubType != "CURB" || r.Date != "2005-07-07" || r.DateBasis != "DATE_ISSUED" || r.NoWork() {
		t.Errorf("%+v", r)
	}
}

func TestImplausibleDateFallsBack(t *testing.T) {
	r := FromFeature(realFeat(map[string]any{"DATE_ISSUED": 2514240000000.0, "DATE_ENTERED": 1158883200000.0}), "t")
	want := time.UnixMilli(1158883200000).UTC().Format("2006-01-02")
	if r.Date != want || r.DateBasis != "DATE_ENTERED" || r.DateFlag == "" {
		t.Errorf("date=%q basis=%q flag=%q", r.Date, r.DateBasis, r.DateFlag)
	}
	if r := FromFeature(realFeat(map[string]any{"DATE_ISSUED": 2514240000000.0}), "t"); !r.When.IsZero() || r.DateFlag == "" {
		t.Errorf("only-bad-date row should have no date: %+v", r)
	}
}
