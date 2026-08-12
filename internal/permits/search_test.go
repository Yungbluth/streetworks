package permits

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// a small city laid out like Tucson's grid
const (
	cLat, cLng = 32.22, -110.97
	step       = 0.001
)

func ewStreet(dir, name string, lat float64, blocks ...int) []Record {
	var out []Record
	for _, n := range blocks {
		lng := cLng + float64(n)/100*step
		if dir == "W" {
			lng = cLng - float64(n)/100*step
		}
		out = append(out, placed(fmt.Sprintf("%s-%s-%d", dir, name, n), strings.TrimSpace(dir+" "+name), n, lat, lng, 2020))
	}
	return out
}

func nsStreet(dir, name string, lng float64, blocks ...int) []Record {
	var out []Record
	for _, n := range blocks {
		lat := cLat + float64(n)/100*step
		if dir == "S" {
			lat = cLat - float64(n)/100*step
		}
		out = append(out, placed(fmt.Sprintf("%s-%s-%d", dir, name, n), strings.TrimSpace(dir+" "+name), n, lat, lng, 2021))
	}
	return out
}

func placed(permit, street string, block int, lat, lng float64, year int) Record {
	r := rec(permit, "Excavation", "CLOSED", street, block, year)
	r.Lat, r.Lng = lat, lng
	return r
}

func many(rs []Record, n int) []Record { // n permits per block, for ranking by size
	var out []Record
	for _, r := range rs {
		for i := 0; i < n; i++ {
			c := r
			c.Permit = fmt.Sprintf("%s#%d", r.Permit, i)
			out = append(out, c)
		}
	}
	return out
}

var speedwayLat = cLat + 10*step
var campbellLng = cLng + 18*step

func testCity() *Index {
	var recs []Record
	recs = append(recs, many(ewStreet("E", "SPEEDWAY BLVD", speedwayLat, 100, 900, 1200, 1300, 1800, 1900, 2500), 3)...)
	recs = append(recs, ewStreet("W", "SPEEDWAY BLVD", speedwayLat, 100, 400)...)
	recs = append(recs, nsStreet("N", "CAMPBELL AVE", campbellLng, 300, 900, 1000, 1100, 1700)...)
	recs = append(recs, nsStreet("N", "4TH AVE", cLng+4*step, 500, 600)...)
	recs = append(recs, ewStreet("E", "6TH ST", cLat+6*step, 300, 400)...)
	recs = append(recs, ewStreet("E", "SPEEDWAY ACCESS RD", cLat+40*step, 100)...) // far away, shares a prefix
	recs = append(recs, nsStreet("N", "SPEED AVE", cLng+50*step, 100)...)          // a shorter name sharing a prefix
	recs = append(recs, ewStreet("", "WEST ST", cLat-20*step, 300)...)             // a street named West
	return Build(recs)
}

func labels(ps []Place) string {
	var out []string
	for _, p := range ps {
		switch p.Kind {
		case "street":
			out = append(out, "street:"+p.Street)
		case "address":
			out = append(out, fmt.Sprintf("address:%d %s exact=%v", p.Block, p.Street, p.Exact))
		case "intersection":
			out = append(out, fmt.Sprintf("x:%s & %s exact=%v", p.Street, p.Cross, p.Exact))
		}
	}
	return strings.Join(out, " | ")
}

func TestSearchAddresses(t *testing.T) {
	ix := testCity()
	cases := []struct{ q, wantFirst string }{
		{"1234 E Speedway Blvd", "address:1200 E SPEEDWAY BLVD exact=true"},
		{"1234 east speedway boulevard", "address:1200 E SPEEDWAY BLVD exact=true"},
		{"1234 E. SPEEDWAY BL.", "address:1200 E SPEEDWAY BLVD exact=true"},
		{"1234 speedway", "address:1200 E SPEEDWAY BLVD exact=true"}, // no direction: the street with that block
		{"1299 E Speedway", "address:1200 E SPEEDWAY BLVD exact=true"},
		{"1300 e speedway", "address:1300 E SPEEDWAY BLVD exact=true"},
		{"1600 E Speedway", "address:1800 E SPEEDWAY BLVD exact=false"}, // nearest recorded: 1300 is 3 away, 1800 is 2
		{"5000 E Speedway", "address:2500 E SPEEDWAY BLVD exact=false"}, // past the end of the data
		{"40 E Speedway", "address:100 E SPEEDWAY BLVD exact=false"},
		{"400 W Speedway", "address:400 W SPEEDWAY BLVD exact=true"},
		{"400 Speedway", "address:400 W SPEEDWAY BLVD exact=true"}, // only W has a 400 block
		{"1000 N Campbell Ave", "address:1000 N CAMPBELL AVE exact=true"},
		{"1000 N Campbell Avenue", "address:1000 N CAMPBELL AVE exact=true"},
		{"512 N 4th Ave", "address:500 N 4TH AVE exact=true"},
		{"512 N 4 th Av", "address:500 N 4TH AVE exact=true"},
		{"512 north fourth", ""}, // spelled-out numbers are not understood; must not crash or guess
		{"1234-1240 E Speedway", "address:1200 E SPEEDWAY BLVD exact=true"}, // a number range
		{"300 West St", "address:300 WEST ST exact=true"},
	}
	for _, c := range cases {
		got := ix.Search(c.q, 8)
		first := ""
		if len(got) > 0 {
			first = labels(got[:1])
		}
		if first != c.wantFirst {
			t.Errorf("Search(%q) first = %q, want %q\n  all: %s", c.q, first, c.wantFirst, labels(got))
		}
	}
	// "1234 speedway" offers both directions, the exact block first
	if got := labels(ix.Search("1234 speedway", 8)); !strings.Contains(got, "W SPEEDWAY BLVD") {
		t.Errorf("both directions should be offered: %s", got)
	}
	if p := ix.Search("1600 E Speedway", 1)[0]; p.Wanted != 1600 || p.Block != 1800 {
		t.Errorf("%+v", p)
	}
}

func TestSearchStreets(t *testing.T) {
	ix := testCity()
	cases := []struct{ q, want string }{
		{"Speedway Blvd", "street:E SPEEDWAY BLVD | street:W SPEEDWAY BLVD"},   // busiest first, suffix narrows
		{"e speedway", "street:E SPEEDWAY BLVD | street:E SPEEDWAY ACCESS RD"}, // direction narrows
		{"W Speedway", "street:W SPEEDWAY BLVD"},
		{"N Campbell", "street:N CAMPBELL AVE"},
		{"campbell avenue", "street:N CAMPBELL AVE"},
		{"S Campbell", "street:N CAMPBELL AVE"}, // no S Campbell: direction ignored rather than nothing
		{"4th ave", "street:N 4TH AVE"},
		{"west st", "street:WEST ST"},
		{"camp", "street:N CAMPBELL AVE"},  // typing in progress
		{"pbell", "street:N CAMPBELL AVE"}, // contained, when nothing starts with it
	}
	for _, c := range cases {
		if got := labels(ix.Search(c.q, 8)); got != c.want {
			t.Errorf("Search(%q)\n  got  %s\n  want %s", c.q, got, c.want)
		}
	}
	if got := labels(ix.Search("speed", 8)); !strings.HasPrefix(got, "street:N SPEED AVE") || !strings.Contains(got, "E SPEEDWAY BLVD") {
		t.Errorf("speed: %s", got)
	}
	p := ix.Search("E Speedway Blvd", 1)[0]
	if p.Bounds == nil || p.Bounds[1] > cLng+1*step+1e-9 || p.Bounds[3] < cLng+25*step-1e-9 || p.Permits != 21 {
		t.Errorf("bounds/permits: %+v %v", p, p.Bounds)
	}
}

func TestSearchIntersections(t *testing.T) {
	ix := testCity()
	for _, q := range []string{"Speedway & Campbell", "speedway and campbell", "Campbell / Speedway", "E Speedway Blvd at N Campbell Ave", "speedway@campbell"} {
		got := ix.Search(q, 8)
		if len(got) == 0 {
			t.Errorf("%q: nothing", q)
			continue
		}
		p := got[0]
		names := p.Street + "+" + p.Cross
		if p.Kind != "intersection" || !strings.Contains(names, "E SPEEDWAY BLVD") || !strings.Contains(names, "N CAMPBELL AVE") || !p.Exact {
			t.Errorf("%q: %s", q, labels(got))
		}
		// the point lies between the nearest blocks: close to the real crossing
		if d := DistanceMeters(p.Lat, p.Lng, speedwayLat, campbellLng); d > 150 {
			t.Errorf("%q: point is %.0f m from the crossing", q, d)
		}
	}
	got := ix.Search("4th Ave & West St", 8)
	if len(got) == 0 || got[0].Exact || got[0].Gap < 500 {
		t.Errorf("non-crossing streets: %+v", got)
	}
	for _, q := range []string{"& Campbell", "Speedway &", "nowhere & campbell", " & "} {
		if got := ix.Search(q, 8); len(got) != 0 && got[0].Kind == "intersection" && strings.Contains(q, "nowhere") {
			t.Errorf("%q should not find an intersection: %s", q, labels(got))
		}
	}
}

func TestSearchInputSafety(t *testing.T) {
	ix := testCity()
	for _, q := range []string{"", "   ", "12345", "&", "!!!", "\x00\x01", strings.Repeat("speedway ", 200), "1234", "99999999999999 speedway", "a"} {
		got := ix.Search(q, 8)
		if got == nil {
			t.Errorf("%q: nil (must be an empty list for JSON)", q)
		}
	}
	if got := ix.Search("zzzz", 8); len(got) != 0 {
		t.Errorf("unknown street: %s", labels(got))
	}
	if n := len(ix.Search("s", 100)); n > 20 {
		t.Errorf("limit not capped: %d", n)
	}
	if got := Build(nil).Search("speedway", 8); got == nil || len(got) != 0 {
		t.Errorf("empty index: %v", got)
	}
	b, _ := json.Marshal(ix.Search("zzzz", 8))
	if string(b) != "[]" {
		t.Errorf("JSON = %s", b)
	}
}
