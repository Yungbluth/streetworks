package permits

import (
	"math"
	"regexp"
	"sort"
	"strconv"
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

func (ix *Index) buildStreets() {
	by := map[string]*streetEntry{}
	for _, b := range ix.blocks {
		if !b.HasGeo {
			continue
		}
		e := by[b.Street]
		if e == nil {
			e = &streetEntry{name: b.Street}
			e.dir, e.core, e.suffix = splitStreet(b.Street)
			by[b.Street] = e
		}
		e.blocks = append(e.blocks, b)
		e.permits += len(b.Recs)
	}
	ix.streets = map[string][]*streetEntry{}
	for _, e := range by {
		sort.Slice(e.blocks, func(i, j int) bool { return e.blocks[i].Num < e.blocks[j].Num })
		e.bounds = [4]float64{90, 180, -90, -180}
		for _, b := range e.blocks {
			e.bounds[0], e.bounds[1] = math.Min(e.bounds[0], b.Lat), math.Min(e.bounds[1], b.Lng)
			e.bounds[2], e.bounds[3] = math.Max(e.bounds[2], b.Lat), math.Max(e.bounds[3], b.Lng)
		}
		ix.streets[e.core] = append(ix.streets[e.core], e)
	}
	for _, list := range ix.streets {
		sort.Slice(list, func(i, j int) bool {
			if list[i].permits != list[j].permits {
				return list[i].permits > list[j].permits
			}
			return list[i].name < list[j].name
		})
	}
	ix.cores = make([]string, 0, len(ix.streets))
	for c := range ix.streets {
		ix.cores = append(ix.cores, c)
	}
	sort.Strings(ix.cores)
}

type streetQuery struct{ dir, core, suffix string }

func parseStreetQuery(s string) (streetQuery, bool) {
	canon := Street("", s)
	if canon == "" {
		return streetQuery{}, false
	}
	d, c, x := splitStreet(canon)
	return streetQuery{d, c, x}, c != ""
}

func (ix *Index) matchStreets(q streetQuery, limit int) []*streetEntry {
	var exact, prefix, contains []*streetEntry
	if list, ok := ix.streets[q.core]; ok {
		exact = append(exact, list...)
	}
	if len(q.core) >= 2 {
		i := sort.SearchStrings(ix.cores, q.core)
		for ; i < len(ix.cores) && strings.HasPrefix(ix.cores[i], q.core); i++ {
			if ix.cores[i] != q.core {
				prefix = append(prefix, ix.streets[ix.cores[i]]...)
			}
		}
		if len(exact)+len(prefix) == 0 && len(q.core) >= 3 {
			for _, c := range ix.cores {
				if strings.Contains(c, q.core) {
					contains = append(contains, ix.streets[c]...)
				}
			}
		}
	}
	byPermits := func(l []*streetEntry) {
		sort.SliceStable(l, func(i, j int) bool { return l[i].permits > l[j].permits })
	}
	byPermits(prefix)
	byPermits(contains)
	all := append(append(exact, prefix...), contains...)
	narrow := func(keep func(*streetEntry) bool) {
		var out []*streetEntry
		for _, e := range all {
			if keep(e) {
				out = append(out, e)
			}
		}
		if len(out) > 0 {
			all = out
		}
	}
	if q.dir != "" {
		narrow(func(e *streetEntry) bool { return e.dir == q.dir })
	}
	if q.suffix != "" {
		narrow(func(e *streetEntry) bool { return e.suffix == q.suffix })
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// nearestBlock returns the street's block closest in number to want
func (e *streetEntry) nearestBlock(want int) *Block {
	i := sort.Search(len(e.blocks), func(i int) bool { return e.blocks[i].Num >= want })
	switch {
	case i == len(e.blocks):
		return e.blocks[i-1]
	case i == 0 || e.blocks[i].Num == want:
		return e.blocks[i]
	case want-e.blocks[i-1].Num <= e.blocks[i].Num-want:
		return e.blocks[i-1]
	}
	return e.blocks[i]
}

// Search turns what someone typed into places on the map
func (ix *Index) Search(q string, limit int) []Place {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	q = strings.TrimSpace(q)
	if len(q) > 120 {
		q = q[:120]
	}
	out := []Place{}
	if q == "" || ix.streets == nil {
		return out
	}
	if parts := crossSplit.Split(q, -1); len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		return ix.searchIntersection(parts[0], parts[1], limit)
	}
	want := -1
	if m := leadingNumber.FindStringSubmatch(q); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			want, q = n/100*100, m[2]
		}
	}
	sq, ok := parseStreetQuery(q)
	if !ok {
		return out
	}
	for _, e := range ix.matchStreets(sq, limit) {
		if want < 0 {
			out = append(out, Place{Kind: "street", Street: e.name, Block: -1, Wanted: -1, Exact: true,
				Lat: (e.bounds[0] + e.bounds[2]) / 2, Lng: (e.bounds[1] + e.bounds[3]) / 2, Bounds: &[4]float64{e.bounds[0], e.bounds[1], e.bounds[2], e.bounds[3]}, Permits: e.permits})
			continue
		}
		b := e.nearestBlock(want)
		out = append(out, Place{Kind: "address", Street: e.name, Block: b.Num, Wanted: want, Exact: b.Num == want,
			Lat: b.Lat, Lng: b.Lng, Permits: e.permits})
	}
	if want >= 0 {
		// Exact blocks first, then the closest stand-ins, keeping street order otherwise
		sort.SliceStable(out, func(i, j int) bool {
			di, dj := abs(out[i].Block-want), abs(out[j].Block-want)
			return di < dj
		})
	}
	return out
}
