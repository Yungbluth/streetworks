package permits

import (
	"testing"
)

func TestStreetTable(t *testing.T) {
	cases := []struct{ dir, name, want string }{
		{"E", "SPEEDWAY BL", "E SPEEDWAY BLVD"},
		{"S", "PARK AV", "S PARK AVE"},
		{"S", "EASTSIDE LP", "S EASTSIDE LOOP"},
		{"S", "4TH AV", "S 4TH AVE"},
		{"E", "2ND ST", "E 2ND ST"},
		{"W", "NAVAJO RD", "W NAVAJO RD"},
		{"E", "GAS RD", "E GAS RD"},
		// the same street spelled differently must collapse to one key
		{"E", "SPEEDWAY BLVD", "E SPEEDWAY BLVD"},
		{"e", "speedway boulevard", "E SPEEDWAY BLVD"},
		{"East", "Speedway Boulevard", "E SPEEDWAY BLVD"},
		{"", "EAST SPEEDWAY BL", "E SPEEDWAY BLVD"},
		{"E.", "SPEEDWAY BLVD.", "E SPEEDWAY BLVD"},
		{" E ", "  SPEEDWAY   BL  ", "E SPEEDWAY BLVD"},
		{"E", "E SPEEDWAY BL", "E SPEEDWAY BLVD"},
		{"E", "E. Speedway Bl.", "E SPEEDWAY BLVD"},
		{"W", "SPEEDWAY BL", "W SPEEDWAY BLVD"},
		{"W", "E SPEEDWAY BL", "W E SPEEDWAY BLVD"},
		// ordinals
		{"S", "22 ND ST", "S 22ND ST"},
		{"S", "22nd Street", "S 22ND ST"},
		{"N", "1 ST AVE", "N 1ST AVE"},
		{"N", "3 RD AVE", "N 3RD AVE"},
		{"E", "5 TH ST", "E 5TH ST"},
		// a street named like a direction or suffix is not mangled
		{"", "WEST ST", "WEST ST"},
		{"", "NORTH AVE", "NORTH AVE"},
		{"", "NORTH ST", "NORTH ST"},
		// two-letter directions
		{"NE", "MAIN ST", "NE MAIN ST"},
		{"", "SW MAIN ST", "SW MAIN ST"},
		// suffix variants
		{"", "CERULEAN WAY", "CERULEAN WAY"},
		{"", "CERULEAN WY", "CERULEAN WAY"},
		{"", "GOLF LINKS CI", "GOLF LINKS CIR"},
		{"", "CAMINO REAL PKWY", "CAMINO REAL PKWY"},
		{"", "OLD SPANISH TRAIL", "OLD SPANISH TRL"},
		// punctuation and spacing noise
		{"", "O'MALLEY RD", "O MALLEY RD"},
		{"", "ST. MARY'S RD", "ST MARY S RD"},
		{"", "VIA-ALBA  DR", "VIA ALBA DR"},
		// empty and degenerate input
		{"E", "", ""},
		{"", "", ""},
		{"", "   ", ""},
		{"", "...", ""},
		{"", "E", "E"},
		{"E", "E", "E E"},
		{"X", "MAIN ST", "MAIN ST"}, // unknown direction ignored
	}
	for _, c := range cases {
		if got := Street(c.dir, c.name); got != c.want {
			t.Errorf("Street(%q, %q) = %q, want %q", c.dir, c.name, got, c.want)
		}
	}
}

func TestStreetVariants(t *testing.T) {
	groups := [][][2]string{
		{{"E", "SPEEDWAY BL"}, {"E", "SPEEDWAY BLVD"}, {"", "East Speedway Boulevard"}, {"E.", "speedway bl."}},
		{{"S", "EASTSIDE LP"}, {"S", "EASTSIDE LOOP"}, {"South", "Eastside Loop"}},
		{{"S", "22 ND ST"}, {"S", "22ND ST"}, {"S", "22nd Street"}},
	}
	for _, g := range groups {
		want := Street(g[0][0], g[0][1])
		for _, v := range g[1:] {
			if got := Street(v[0], v[1]); got != want {
				t.Errorf("Street(%q,%q) = %q; want %q (same as %q)", v[0], v[1], got, want, g[0][1])
			}
		}
	}
}

func TestBlockOfTable(t *testing.T) {
	cases := map[string]int{
		"2345": 2300, "2300": 2300, "2399": 2300, "2400": 2400,
		"749": 700, "99": 0, "100": 100, "10602": 10600,
		" 2300 E": 2300, "2300-2310": 2300, "2300A": 2300, "0099": 0,
		"": -1, "   ": -1, "ABC": -1, "-5": -1, "0": -1, "000": -1, ".5": -1,
		"99999999999999999999": -1, // overflows int: must not panic or wrap
	}
	for in, want := range cases {
		if got := BlockOf(in); got != want {
			t.Errorf("BlockOf(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestCategories(t *testing.T) {
	cases := []struct {
		parts []string
		want  string
	}{
		{[]string{"EXCAVATION", "STRT", "STREET EXCAVATION"}, "Excavation"},
		{[]string{"Excavation Permit"}, "Excavation"},
		{[]string{"BARRICADE"}, "Barricade"},
		{[]string{"TRAFFIC OBSTRUCTION - BARRICADE"}, "Barricade"},
		{[]string{"COMBO"}, "Combo"},
		{[]string{"EXCAVATION/BARRICADE COMBO"}, "Combo"}, // combo beats its parts
		{[]string{"CIVIC EVENT"}, "Civic event"},
		{[]string{"Special Event"}, "Civic event"},
		{[]string{"Right-of-Way Small Wireless", "TR-SWF"}, "Small wireless"},
		{[]string{"TR-SWF"}, "Small wireless"},
		{[]string{"Right-of-Way (ROW) - Utility", "TR-UTL"}, "Utility"},
		{[]string{"Fiber Permit", "TR-FBR"}, "Utility"},
		{[]string{"Right-of-Way (ROW) - Annual", "TR-ANN"}, "Annual"},
		{[]string{"Right-of-Way (ROW)", "TR-ROW"}, "Right-of-way"},
		{[]string{"Private Improvement Agreement (PIA)", "TR-PIA"}, "Other"},
		{[]string{"EXCAVATION", "STRT", "STREET EXCAVATION FOR UTILITY"}, "Excavation"}, // legacy words win
		{[]string{"Right-of-Way (ROW) - Utility", "TR-UTL", "excavation permit"}, "Excavation"},
		{[]string{"VALET"}, "Other"},
		{[]string{""}, "Other"},
		{nil, "Other"},
	}
	for _, c := range cases {
		if got := Category(c.parts...); got != c.want {
			t.Errorf("Category(%q) = %q, want %q", c.parts, got, c.want)
		}
	}
}
