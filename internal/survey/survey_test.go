package survey

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/arcgistest"
	"github.com/yungbluth/streetworks/internal/fetch"
)

func testClient() *arcgis.Client {
	c := arcgis.New()
	c.Retries, c.Backoff = 1, time.Millisecond
	return c
}

func ms(y int, m time.Month, d int) float64 {
	return float64(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).UnixMilli())
}

var allFields = []arcgis.Field{arcgistest.OIDField(),
	{Name: "PERMIT", Type: "esriFieldTypeString"}, {Name: "PERMIT_TYPE", Type: "esriFieldTypeString"}, {Name: "PERMIT_STATUS", Type: "esriFieldTypeString"},
	{Name: "STREET_NO", Type: "esriFieldTypeString"}, {Name: "STREET_NAME", Type: "esriFieldTypeString"}, {Name: "DATE_ISSUED", Type: "esriFieldTypeDate"}}

func dated(dates ...float64) []arcgis.Feature {
	var fs []arcgis.Feature
	for i, d := range dates {
		fs = append(fs, arcgistest.Feat(map[string]any{"OBJECTID": float64(i + 1), "DATE_ISSUED": d}, ""))
	}
	return fs
}

// city is shaped like the real permit service
func city(t *testing.T, rejectWhere bool) *arcgistest.Server {
	t.Helper()
	srv := arcgistest.New(
		arcgistest.Layer{ID: 75, Name: "TDOT_PERMITS_RW_INACTIVE", Fields: allFields, Features: dated(ms(2010, 1, 1), ms(2025, 2, 12), ms(2049, 9, 3)), RejectWhere: rejectWhere},
		arcgistest.Layer{ID: 32, Name: "TDOT_PERMITS_RW_ACTIVE", Fields: allFields, Features: dated(ms(2026, 6, 1), ms(2026, 7, 1)), RejectWhere: rejectWhere},
		arcgistest.Layer{ID: 133, Name: "DTM_PERMITS_ROW_ACTIVE", Fields: allFields, Features: dated(ms(2026, 7, 20))},
		arcgistest.Layer{ID: 33, Name: "TDOT_PERMITS_PIA_ACTIVE", Fields: allFields[:2], Features: dated(ms(2026, 7, 25))},
		arcgistest.Layer{ID: 62, Name: "DTM_PERMITS_ROW_CIP_JOC", Fields: allFields},
		arcgistest.Layer{ID: 90, Name: "TDOT_PERMITS_OLDER", Fields: allFields, Features: dated(ms(2019, 5, 5))},
		arcgistest.Layer{ID: 91, Name: "TDOT_PERMITS_BROKEN", Fields: allFields, Features: dated(ms(2026, 7, 1)), QueryStatus: 500},
		arcgistest.Layer{ID: 137, Name: "DTM_PERMITS_ROW_SpecialEvents", Fields: allFields, Features: dated(ms(2026, 7, 1))},
		arcgistest.Layer{ID: 12, Name: "PDSD_BUILDING_PERMITS", Fields: allFields, Features: dated(ms(2026, 7, 25))},
		arcgistest.Layer{ID: 13, Name: "Prop409", Fields: allFields},
	)
	t.Cleanup(srv.Close)
	return srv
}

func run(t *testing.T, srv *arcgistest.Server, have ...fetch.Layer) string {
	t.Helper()
	var out bytes.Buffer
	if err := Run(context.Background(), &out, testClient(), srv.ServiceURL(), have); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	return out.String()
}

func tableLine(out, id string) string {
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > 0 && f[0] == id {
			return l
		}
	}
	return ""
}

func recommendations(t *testing.T, out string) string {
	t.Helper()
	i := strings.Index(out, "Layers that fit")
	if i < 0 {
		t.Fatalf("no recommendations:\n%s", out)
	}
	return out[i:]
}

func TestTable(t *testing.T) {
	srv := city(t, false)
	out := run(t, srv, fetch.Layer{Name: "permits_inactive", URL: srv.LayerURL(75)})

	for id, want := range map[string][]string{
		"75":  {"2025-02-12", "already downloaded", "6/6"}, // the 2049 typo is ignored
		"32":  {"2026-07-01", "6/6"},
		"33":  {"1/6", "(no PERMIT_TYPE, PERMIT_STATUS, STREET_NO, STREET_NAME, DATE_ISSUED)"},
		"62":  {"none"},
		"91":  {"could not read"},
		"137": {"2026-07-01"},
	} {
		for _, w := range want {
			if l := tableLine(out, id); !strings.Contains(l, w) {
				t.Errorf("layer %s: %q should contain %q", id, l, w)
			}
		}
	}
	for _, id := range []string{"12", "13"} {
		if tableLine(out, id) != "" {
			t.Errorf("layer %s isn't a right-of-way permit layer", id)
		}
	}
	if strings.Index(out, "  133 ") > strings.Index(out, "   75 ") {
		t.Errorf("newest layers should come first:\n%s", out)
	}
}

func TestRecommend(t *testing.T) {
	srv := city(t, false)
	rec := recommendations(t, run(t, srv, fetch.Layer{Name: "permits_inactive", URL: srv.LayerURL(75)}))
	for _, want := range []string{"-extra permits_rw_active=" + srv.LayerURL(32), "-extra permits_row_active=" + srv.LayerURL(133)} {
		if !strings.Contains(rec, want) {
			t.Errorf("should recommend %q:\n%s", want, rec)
		}
	}
	for _, id := range []int{75, 33, 62, 90, 91, 137} {
		if strings.Contains(rec, srv.LayerURL(id)+"\n") {
			t.Errorf("should not recommend layer %d:\n%s", id, rec)
		}
	}
}

func TestErrors(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	if err := Run(context.Background(), &bytes.Buffer{}, testClient(), dead.URL, nil); err == nil {
		t.Error("an unreachable service should be an error")
	}
	empty := arcgistest.New(arcgistest.Layer{ID: 1, Name: "litecode"})
	defer empty.Close()
	if out := run(t, empty); !strings.Contains(out, "No right-of-way permit layers") {
		t.Error(out)
	}
}

func TestSuggestName(t *testing.T) {
	for in, want := range map[string]string{
		"TDOT_PERMITS_RW_ACTIVE":                        "permits_rw_active",
		"DTM_PERMITS_ROW_ACTIVE":                        "permits_row_active",
		"Permits RW (Active)":                           "permits_rw_active",
		"RW_ACTIVE":                                     "permits_rw_active",
		"tdot_row-permits":                              "permits_row_permits",
		"Inactive General ROW Permits (2020 - Present)": "permits_inactive_general_row_permits_2020_presen",
	} {
		if got := suggestName(in); got != want {
			t.Errorf("suggestName(%q) = %q, want %q", in, got, want)
		}
	}
}
