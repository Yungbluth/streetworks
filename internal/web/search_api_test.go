package web

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/yungbluth/streetworks/internal/permits"
)

func searchServer(t *testing.T) (*testing.T, func(path string) (int, string)) {
	t.Helper()
	a := rec("A1", "Excavation", "CLOSED", "E SPEEDWAY BLVD", 1200, 2024)
	a.Lat, a.Lng = 32.232, -110.958
	b := rec("B1", "Barricade", "CLOSED", "E SPEEDWAY BLVD", 1300, 2019)
	b.Lat, b.Lng = 32.232, -110.957
	c := rec("C1", "Combo", "CLOSED", "N CAMPBELL AVE", 1000, 2022)
	c.Lat, c.Lng = 32.2325, -110.9435
	srv, _ := newTestServer(t, nil, []permits.Record{a, b, c})
	return t, func(path string) (int, string) {
		resp, body := fetch(t, srv, "GET", path)
		return resp.StatusCode, body
	}
}

func TestSearchEndpoint(t *testing.T) {
	_, get := searchServer(t)
	code, body := get("/api/search?q=" + url.QueryEscape("1234 E Speedway Blvd"))
	var res struct {
		Places []permits.Place `json:"places"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &res) != nil || len(res.Places) == 0 {
		t.Fatalf("%d %s", code, body)
	}
	if p := res.Places[0]; p.Kind != "address" || p.Street != "E SPEEDWAY BLVD" || p.Block != 1200 || !p.Exact {
		t.Errorf("%+v", p)
	}
	for _, q := range []string{"", "zzzz", "%00", strings.Repeat("x", 5000)} {
		code, body := get("/api/search?q=" + url.QueryEscape(q))
		if code != 200 || !strings.Contains(body, `"places":[]`) {
			t.Errorf("q=%.20q: %d %s", q, code, body)
		}
	}
	if _, body := get("/api/search?q=speedway%20%26%20campbell"); !strings.Contains(body, `"kind":"intersection"`) {
		t.Errorf("intersection: %s", body)
	}
	if _, body := get("/api/search?q=speedway"); !strings.Contains(body, `"kind":"street"`) || !strings.Contains(body, `"bounds":[`) {
		t.Errorf("street: %s", body)
	}
}

func TestNearbyEndpoint(t *testing.T) {
	_, get := searchServer(t)
	code, body := get("/api/nearby?lat=32.232&lng=-110.958&r=200")
	var n struct {
		Count   int `json:"count"`
		Records []struct {
			Permit   string  `json:"permit"`
			Distance float64 `json:"distance_m"`
			BlockKey string  `json:"block_key"`
		} `json:"records"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &n) != nil {
		t.Fatalf("%d %s", code, body)
	}
	if n.Count != 2 || len(n.Records) != 2 || n.Records[0].Permit != "A1" || n.Records[0].BlockKey != "E SPEEDWAY BLVD|1200" {
		t.Errorf("%+v", n)
	}
	if _, body := get("/api/nearby?lat=32.232&lng=-110.958&r=200&cats=Barricade"); !strings.Contains(body, `"count":1`) {
		t.Errorf("category filter: %s", body)
	}
	if _, body := get("/api/nearby?lat=32.232&lng=-110.958&r=200&cats="); !strings.Contains(body, `"count":0`) || !strings.Contains(body, `"records":[]`) {
		t.Errorf("no categories: %s", body)
	}
	for _, q := range []string{"", "lat=32&lng=-110", "lat=x&lng=-110&r=100", "lat=91&lng=-110&r=100", "lat=32&lng=-181&r=100",
		"lat=32&lng=-110&r=0", "lat=32&lng=-110&r=-5", "lat=32&lng=-110&r=999999", "lat=NaN&lng=-110&r=100", "lat=32&lng=Inf&r=100"} {
		if code, _ := get("/api/nearby?" + q); code != 400 {
			t.Errorf("%q: %d, want 400", q, code)
		}
	}
}

func TestStreetEndpoint(t *testing.T) {
	_, get := searchServer(t)
	code, body := get("/api/street?name=" + url.QueryEscape("E SPEEDWAY BLVD"))
	var p permits.StreetProfile
	if code != 200 || json.Unmarshal([]byte(body), &p) != nil || p.Count != 2 || len(p.Blocks) != 2 || p.Blocks[0].Block != 1200 {
		t.Fatalf("%d %s", code, body)
	}
	if _, body := get("/api/street?name=" + url.QueryEscape("E SPEEDWAY BLVD") + "&from=2023"); !strings.Contains(body, `"count":1`) {
		t.Errorf("filter: %s", body)
	}
	for _, n := range []string{"", "speedway", "NOPE ST"} {
		if code, _ := get("/api/street?name=" + url.QueryEscape(n)); code != 404 {
			t.Errorf("%q: %d", n, code)
		}
	}
}
