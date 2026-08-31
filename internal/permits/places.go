package permits

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PlaceKind says how a permit's place was worked out.
type PlaceKind int

const (
	KindBlock        PlaceKind = iota // a hundred-block of one street
	KindIntersection                  // two streets named in the location text
	KindPoint                         // just a spot on the map, labelled with whatever text came with it
)

// Place is somewhere on the map that one or more permits share.
type Place struct {
	Kind     PlaceKind
	Street   string // the block's street, or the first street of an intersection
	Cross    string // the second street of an intersection
	Block    int    // -1 unless Kind is KindBlock
	Label    string // the location text of a point
	Lat, Lng float64
	Permits  []int // indexes into the records given to AssignPlaces
}

func (p Place) key() string {
	switch p.Kind {
	case KindBlock:
		return p.Street + "|" + strconv.Itoa(p.Block)
	case KindIntersection:
		return p.Street + " & " + p.Cross
	}
	return p.Label
}

// noLabel stands in for a point permit whose location text is blank
const noLabel = "No location given"

func ParseIntersection(text string) (a, b string, ok bool) {
	s := " " + strings.ToUpper(text) + " "
	s = strings.ReplaceAll(s, " AND ", "&")
	s = strings.ReplaceAll(s, "/", "&")
	parts := strings.Split(s, "&")
	if len(parts) != 2 {
		return "", "", false
	}
	for i, part := range parts {
		street := Street("", part)
		if d, rest, found := strings.Cut(street, " "); found && directions[d] != "" {
			street = rest
		}
		first, _, _ := strings.Cut(street, " ")
		if _, err := strconv.Atoi(first); err == nil || street == "" {
			return "", "", false // empty, or a house number like "1200 E SPEEDWAY"
		}
		parts[i] = street
	}
	a, b = parts[0], parts[1]
	if a == b {
		return "", "", false
	}
	if b < a {
		a, b = b, a
	}
	return a, b, true
}

// AssignPlaces puts every record that has coordinates on a block, an intersection or a point
func AssignPlaces(recs []Record) (places []Place, unplaced int) {
	byKey := map[string]int{}
	for i, r := range recs {
		if !r.HasGeo {
			unplaced++
			continue
		}
		p := Place{Kind: KindPoint, Block: -1}
		var key string
		switch a, b, ok := ParseIntersection(r.Location); {
		case r.Street != "" && r.Block >= 0:
			p.Kind, p.Street, p.Block = KindBlock, r.Street, r.Block
			key = p.key()
		case ok:
			p.Kind, p.Street, p.Cross = KindIntersection, a, b
			key = p.key()
		default:
			p.Label = strings.Join(strings.Fields(strings.ToUpper(r.Location)), " ")
			if p.Label == "" {
				p.Label = noLabel
			}
			// the same text can turn up all over town ("4700 S" is a grid line)
			key = fmt.Sprintf("%s|%.4f|%.4f", p.Label, r.Lat, r.Lng)
		}
		at, seen := byKey[key]
		if !seen {
			at = len(places)
			byKey[key] = at
			places = append(places, p)
		}
		places[at].Permits = append(places[at].Permits, i)
	}

	for i := range places {
		p := &places[i]
		var lat, lng float64
		for _, idx := range p.Permits {
			lat += recs[idx].Lat
			lng += recs[idx].Lng
		}
		p.Lat, p.Lng = lat/float64(len(p.Permits)), lng/float64(len(p.Permits))
	}

	sort.SliceStable(places, func(i, j int) bool {
		a, b := places[i], places[j]
		switch {
		case a.Kind != b.Kind:
			return a.Kind < b.Kind
		case a.Street != b.Street:
			return a.Street < b.Street
		case a.Block != b.Block:
			return a.Block < b.Block
		case a.Cross != b.Cross:
			return a.Cross < b.Cross
		case a.Label != b.Label:
			return a.Label < b.Label
		case a.Lat != b.Lat:
			return a.Lat < b.Lat
		}
		return a.Lng < b.Lng
	})
	return places, unplaced
}
