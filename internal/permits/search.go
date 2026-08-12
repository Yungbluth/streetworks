package permits

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// maxCrossingGap is how far apart two streets' nearest blocks can be and still count as crossing
const maxCrossingGap = 350.0

func (ix *Index) searchIntersection(a, b string, limit int) []Place {
	qa, okA := parseStreetQuery(a)
	qb, okB := parseStreetQuery(b)
	out := []Place{}
	if !okA || !okB {
		return out
	}
	type pair struct {
		ea, eb *streetEntry
		ba, bb *Block
		d      float64
	}
	var pairs []pair
	for _, ea := range ix.matchStreets(qa, 4) {
		for _, eb := range ix.matchStreets(qb, 4) {
			if ea == eb {
				continue
			}
			best := pair{ea: ea, eb: eb, d: math.Inf(1)}
			for _, x := range ea.blocks {
				for _, y := range eb.blocks {
					if d := DistanceMeters(x.Lat, x.Lng, y.Lat, y.Lng); d < best.d {
						best.ba, best.bb, best.d = x, y, d
					}
				}
			}
			if best.ba != nil {
				pairs = append(pairs, best)
			}
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].d < pairs[j].d })
	// When the streets really do cross somewhere, pairings that do not
	if len(pairs) > 0 && pairs[0].d <= maxCrossingGap {
		keep := pairs[:0]
		for _, p := range pairs {
			if p.d <= maxCrossingGap {
				keep = append(keep, p)
			}
		}
		pairs = keep
	}
	for _, p := range pairs {
		if len(out) == limit {
			break
		}
		out = append(out, Place{Kind: "intersection", Street: p.ea.name, Cross: p.eb.name, Block: -1, Wanted: -1,
			Exact: p.d <= maxCrossingGap, Lat: (p.ba.Lat + p.bb.Lat) / 2, Lng: (p.ba.Lng + p.bb.Lng) / 2,
			Permits: p.ea.permits + p.eb.permits, Gap: math.Round(p.d)})
	}
	return out
}

func DistanceMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371008.8
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(h)))
}

const MaxRadius = 5000.0

type NearbyRecord struct {
	Record
	Distance float64 `json:"distance_m"`
	BlockKey string  `json:"block_key,omitempty"` // empty for permits that only have coordinates
}

type Nearby struct {
	Lat, Lng   float64        `json:"-"`
	Radius     float64        `json:"radius_m"`
	Count      int            `json:"count"`
	Blocks     int            `json:"blocks"`
	Unplaced   int            `json:"unplaced"` // permits found by coordinates that have no street block (often intersections)
	Categories []KV           `json:"categories"`
	PerYear    []KV           `json:"per_year"`
	Records    []NearbyRecord `json:"records"`
	Truncated  bool           `json:"truncated"`
}

func (ix *Index) Nearby(lat, lng, radius float64, f Filter, max int) Nearby {
	radius = math.Min(math.Max(radius, 1), MaxRadius)
	res := Nearby{Lat: lat, Lng: lng, Radius: radius, Categories: []KV{}, PerYear: []KV{}, Records: []NearbyRecord{}}
	// A cheap bounding box before the exact distance: degrees of latitude are ~111 km.
	dLat := radius/111000 + 0.002
	dLng := radius/(111000*math.Max(0.1, math.Cos(lat*math.Pi/180))) + 0.002
	inBox := func(la, ln float64) bool { return math.Abs(la-lat) <= dLat && math.Abs(ln-lng) <= dLng }
	cats, years := map[string]int{}, map[string]int{}
	blocks := map[string]bool{}
	var all []NearbyRecord
	take := func(r *Record, key string) {
		if !r.HasGeo || !inBox(r.Lat, r.Lng) || !f.match(r) {
			return
		}
		d := DistanceMeters(lat, lng, r.Lat, r.Lng)
		if d > radius {
			return
		}
		all = append(all, NearbyRecord{Record: *r, Distance: math.Round(d), BlockKey: key})
		cats[r.Category]++
		if !r.When.IsZero() {
			years[strconv.Itoa(r.When.Year())]++
		}
		if key != "" {
			blocks[key] = true
		} else {
			res.Unplaced++
		}
	}
	for _, b := range ix.blocks {
		if b.HasGeo && !inBox(b.Lat, b.Lng) && !anyInBox(b.Recs, inBox) {
			continue
		}
		for _, r := range b.Recs {
			take(r, b.Key)
		}
	}
	for _, r := range ix.loose {
		take(r, "")
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].When.Equal(all[j].When) {
			return all[i].When.After(all[j].When)
		}
		return all[i].Distance < all[j].Distance
	})
	res.Count, res.Blocks = len(all), len(blocks)
	if max > 0 && len(all) > max {
		all, res.Truncated = all[:max], true
	}
	for i := range all {
		all[i].NoWorkStatus = all[i].NoWork()
	}
	if all == nil {
		all = []NearbyRecord{} // never null in JSON: the page iterates it
	}
	res.Records = all
	res.Categories = top(cats, 0)
	ys := top(years, 0)
	sort.Slice(ys, func(i, j int) bool { return ys[i].Key < ys[j].Key })
	res.PerYear = ys
	return res
}

func anyInBox(recs []*Record, in func(la, ln float64) bool) bool {
	for _, r := range recs {
		if r.HasGeo && in(r.Lat, r.Lng) {
			return true
		}
	}
	return false
}

// StreetProfile is every recorded block on one street, in block order
type StreetProfile struct {
	Street string         `json:"street"`
	Count  int            `json:"count"`
	Blocks []BlockSummary `json:"blocks"`
	Bounds [4]float64     `json:"bounds"`
}

func (ix *Index) Street(name string, f Filter) (StreetProfile, bool) {
	_, core, _ := splitStreet(name)
	for _, e := range ix.streets[core] {
		if e.name != name {
			continue
		}
		p := StreetProfile{Street: e.name, Blocks: []BlockSummary{}, Bounds: e.bounds}
		for _, b := range e.blocks {
			s := BlockSummary{Key: b.Key, Street: b.Street, Block: b.Num, Lat: b.Lat, Lng: b.Lng}
			var last time.Time
			for _, r := range b.Recs {
				if f.match(r) {
					s.Count++
					if r.When.After(last) {
						last = r.When
					}
				}
			}
			if !last.IsZero() {
				s.Last = last.Format("2006-01-02")
			}
			p.Count += s.Count
			p.Blocks = append(p.Blocks, s)
		}
		return p, true
	}
	return StreetProfile{}, false
}
