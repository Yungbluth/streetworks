package permits

import (
	"encoding/json"
	"os"

	"testing"
)

// testdata/street_cases.json holds [dir, name, want] triples
func TestStreetTable(t *testing.T) {
	raw, err := os.ReadFile("testdata/street_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases [][3]string
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 40 {
		t.Fatalf("only %d cases loaded", len(cases))
	}
	for _, c := range cases {
		if got := Street(c[0], c[1]); got != c[2] {
			t.Errorf("Street(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
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
	for typ, want := range map[string]string{
		"BARRICADE":                           "Barricade",
		"EXCAVATION":                          "Excavation",
		"COMBO":                               "Combo",
		"Right-of-Way (ROW) - Utility":        "Utility",
		"Fiber Permit":                        "Utility",
		"Right-of-Way (ROW)":                  "Right-of-way",
		"Right-of-Way (ROW) - Annual":         "Annual",
		"Right-of-Way Small Wireless":         "Small wireless",
		"CIVIC EVENT":                         "Civic event",
		"Private Improvement Agreement (PIA)": "Other",
		"  excavation ":                       "Excavation", // case and outer spaces don't matter
		"right-of-way (row) - utility":        "Utility",
	} {
		got, known := CategoryFor(typ)
		if got != want || !known {
			t.Errorf("CategoryFor(%q) = %q, %v; want %q, true", typ, got, known, want)
		}
	}
	for _, typ := range []string{"Valet", "", "EXCAVATION PERMIT", "Right-of-Way"} {
		if got, known := CategoryFor(typ); got != "Other" || known {
			t.Errorf("CategoryFor(%q) = %q, %v; want Other, false", typ, got, known)
		}
	}
}
