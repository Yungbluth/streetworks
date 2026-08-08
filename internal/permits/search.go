package permits

import (
	"regexp"
	"strings"
)

// search works entirely from the permit data, so it needs no outside geocoder and no key

// Place is something the map can go to.
type Place struct {
	Kind    string      `json:"kind"`             // "address", "intersection" or "street"
	Street  string      `json:"street"`           // canonical, e.g. "E SPEEDWAY BLVD"
	Cross   string      `json:"cross,omitempty"`  // the second street of an intersection
	Block   int         `json:"block"`            // hundred-block of an address; -1 otherwise
	Wanted  int         `json:"wanted"`           // hundred-block the person asked for; -1 if none
	Exact   bool        `json:"exact"`            // false when the nearest recorded block stood in
	Lat     float64     `json:"lat"`              //
	Lng     float64     `json:"lng"`              //
	Bounds  *[4]float64 `json:"bounds,omitempty"` // south, west, north, east of a whole street
	Permits int         `json:"permits"`          // permits recorded on the street (for ranking and display)
	Gap     float64     `json:"gap_m,omitempty"`  // intersection only: how far apart the two streets' nearest blocks are
}

type streetEntry struct {
	name, dir, core, suffix string
	blocks                  []*Block // sorted by Num, all with coordinates
	permits                 int
	bounds                  [4]float64
}

var (
	canonDirs     = map[string]bool{"N": true, "S": true, "E": true, "W": true, "NE": true, "NW": true, "SE": true, "SW": true}
	canonSuffixes = func() map[string]bool {
		m := map[string]bool{}
		for _, v := range suffixes {
			m[v] = true
		}
		return m
	}()
	leadingNumber = regexp.MustCompile(`^\s*(\d{1,6})(?:\s*[-\x{2013}]\s*\d{1,6})?\s+(.*)$`)
	crossSplit    = regexp.MustCompile(`(?i)\s*(?:&|@|/|\band\b|\bat\b)\s*`)
)

// splitStreet breaks a canonical street name into direction, core name and suffix
func splitStreet(canon string) (dir, core, suffix string) {
	t := strings.Fields(canon)
	if len(t) > 1 && canonDirs[t[0]] {
		dir, t = t[0], t[1:]
	}
	if len(t) > 1 && canonSuffixes[t[len(t)-1]] {
		suffix, t = t[len(t)-1], t[:len(t)-1]
	}
	return dir, strings.Join(t, " "), suffix
}
