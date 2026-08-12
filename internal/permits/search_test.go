package permits

import (
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
