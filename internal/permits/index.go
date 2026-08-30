package permits

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type KV struct {
	Key string `json:"key"`
	N   int    `json:"n"`
}

type Meta struct {
	Rows           int         `json:"rows"`
	UniquePermits  int         `json:"unique_permits"`
	DuplicateRows  int         `json:"duplicate_rows"`
	NoWork         int         `json:"no_work"`
	NoBlock        int         `json:"no_block"`
	NoGeo          int         `json:"no_geo"`
	NoDate         int         `json:"no_date"`
	BadDates       int         `json:"bad_dates"`
	StreetFromText int         `json:"street_from_text"`
	Blocks         int         `json:"blocks"`
	DateMin        string      `json:"date_min"`
	DateMax        string      `json:"date_max"`
	DateBasis      []KV        `json:"date_basis"`
	Statuses       []KV        `json:"statuses"`
	Bounds         [4]float64  `json:"bounds"`
	Layers         []LayerStat `json:"layers"`
	PerYear        []KV        `json:"per_year"`
	Categories     []KV        `json:"categories"`
	PermitTypes    []KV        `json:"permit_types"`
	SubTypes       []KV        `json:"sub_types"`
}

type LayerStat struct {
	Layer string `json:"layer"`
	Rows  int    `json:"rows"`
	Kept  int    `json:"kept"`
	Dupes int    `json:"dupes"`
}

type Block struct {
	Key, Street string
	Num         int
	Recs        []*Record
	Lat, Lng    float64
	HasGeo      bool
}

type Index struct {
	blocks map[string]*Block
	loose  []*Record // permits with coordinates but no street block (often intersections)
	Meta   Meta
}

type Filter struct {
	From, To int
	Cats     map[string]bool // nil: every category; empty: none
	NoWork   bool            // include permits that never led to work (void, withdrawn, cancelled, denied)
	Bounds   *Bounds         // if set, Blocks only returns blocks inside it
}

type Bounds struct{ South, West, North, East float64 }

func (b Bounds) Contains(lat, lng float64) bool {
	return lat >= b.South && lat <= b.North && lng >= b.West && lng <= b.East
}

func (f Filter) match(r *Record) bool {
	if !f.NoWork && r.NoWork() {
		return false
	}
	if f.Cats != nil && !f.Cats[r.Category] {
		return false
	}
	if r.When.IsZero() {
		return f.From == 0 && f.To == 0
	}
	y := r.When.Year()
	return (f.From == 0 || y >= f.From) && (f.To == 0 || y <= f.To)
}

type BlockSummary struct {
	Key    string  `json:"key"`
	Street string  `json:"street"`
	Block  int     `json:"block"`
	Count  int     `json:"count"`
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	Last   string  `json:"last,omitempty"`
}

type BlockDetail struct {
	Key     string   `json:"key"`
	Street  string   `json:"street"`
	Block   int      `json:"block"`
	Count   int      `json:"count"`
	Records []Record `json:"records"`
}

func top(m map[string]int, n int) []KV {
	out := make([]KV, 0, len(m))
	for k, v := range m {
		out = append(out, KV{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Key < out[j].Key
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func Build(recs []Record) *Index {
	ix := &Index{blocks: map[string]*Block{}}
	m := &ix.Meta
	m.Rows = len(recs)
	seen := map[string]bool{}
	basis, cats, types, subs, stats := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	var lo, hi time.Time
	layerStats, perYear := map[string]*LayerStat{}, map[int]int{}
	var layerOrder []string
	for i := range recs {
		r := recs[i]
		ls := layerStats[r.Layer]
		if ls == nil {
			ls = &LayerStat{Layer: r.Layer}
			layerStats[r.Layer] = ls
			layerOrder = append(layerOrder, r.Layer)
		}
		ls.Rows++
		if r.Permit != "" {
			if seen[r.Permit] {
				m.DuplicateRows++
				ls.Dupes++
				continue
			}
			seen[r.Permit] = true
		}
		ls.Kept++
		m.UniquePermits++
		cats[r.Category]++
		stats[r.Status]++
		if r.DateFlag != "" {
			m.BadDates++
		}
		if r.StreetSource != "" {
			m.StreetFromText++
		}
		types[r.Type]++
		if r.SubType != "" {
			subs[r.SubType]++
		}
		if r.NoWork() {
			m.NoWork++
		}
		if !r.HasGeo {
			m.NoGeo++
		}
		if r.When.IsZero() {
			m.NoDate++
		} else {
			basis[r.DateBasis]++
			perYear[r.When.Year()]++
			if lo.IsZero() || r.When.Before(lo) {
				lo = r.When
			}
			if r.When.After(hi) {
				hi = r.When
			}
		}
		if r.Block < 0 || r.Street == "" {
			m.NoBlock++
			if r.HasGeo {
				ix.loose = append(ix.loose, &r)
			}
			continue
		}
		key := fmt.Sprintf("%s|%d", r.Street, r.Block)
		b := ix.blocks[key]
		if b == nil {
			b = &Block{Key: key, Street: r.Street, Num: r.Block}
			ix.blocks[key] = b
		}
		b.Recs = append(b.Recs, &r)
	}
	for _, b := range ix.blocks {
		n := 0
		for _, r := range b.Recs {
			if r.HasGeo {
				b.Lat += r.Lat
				b.Lng += r.Lng
				n++
			}
		}
		if n > 0 {
			b.Lat, b.Lng, b.HasGeo = b.Lat/float64(n), b.Lng/float64(n), true
		}
	}
	m.Layers = make([]LayerStat, 0, len(layerOrder))
	for _, name := range layerOrder {
		m.Layers = append(m.Layers, *layerStats[name])
	}
	years := make([]int, 0, len(perYear))
	for y := range perYear {
		years = append(years, y)
	}
	sort.Ints(years)
	m.PerYear = make([]KV, 0, len(years))
	for _, y := range years {
		m.PerYear = append(m.PerYear, KV{Key: fmt.Sprint(y), N: perYear[y]})
	}
	m.Blocks = len(ix.blocks)
	m.Bounds = ix.percentileBounds(0.01, 0.99)
	if !lo.IsZero() {
		m.DateMin, m.DateMax = lo.Format("2006-01-02"), hi.Format("2006-01-02")
	}
	m.DateBasis, m.Categories, m.PermitTypes, m.SubTypes, m.Statuses = top(basis, 0), top(cats, 0), top(types, 25), top(subs, 25), top(stats, 0)
	return ix
}

// Blocks lists mappable blocks with at least one record matching the filter
func (ix *Index) Blocks(f Filter) []BlockSummary {
	var out []BlockSummary
	for _, b := range ix.blocks {
		if !b.HasGeo || (f.Bounds != nil && !f.Bounds.Contains(b.Lat, b.Lng)) {
			continue
		}
		n := 0
		var last time.Time
		for _, r := range b.Recs {
			if f.match(r) {
				n++
				if r.When.After(last) {
					last = r.When
				}
			}
		}
		if n == 0 {
			continue
		}
		s := BlockSummary{Key: b.Key, Street: b.Street, Block: b.Num, Count: n, Lat: b.Lat, Lng: b.Lng}
		if !last.IsZero() {
			s.Last = last.Format("2006-01-02")
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func (ix *Index) Block(key string, f Filter) (BlockDetail, bool) {
	b := ix.blocks[key]
	if b == nil {
		return BlockDetail{}, false
	}
	d := BlockDetail{Key: b.Key, Street: b.Street, Block: b.Num, Records: []Record{}}
	for _, r := range b.Recs {
		if f.match(r) {
			rr := *r
			rr.NoWorkStatus = r.NoWork()
			d.Records = append(d.Records, rr)
		}
	}
	sort.SliceStable(d.Records, func(i, j int) bool { return d.Records[i].Date > d.Records[j].Date })
	d.Count = len(d.Records)
	return d, true
}

func (ix *Index) percentileBounds(lo, hi float64) [4]float64 {
	var lats, lngs []float64
	for _, b := range ix.blocks {
		if b.HasGeo {
			lats, lngs = append(lats, b.Lat), append(lngs, b.Lng)
		}
	}
	if len(lats) == 0 {
		return [4]float64{}
	}
	sort.Float64s(lats)
	sort.Float64s(lngs)
	at := func(xs []float64, q float64) float64 { return xs[int(q*float64(len(xs)-1))] }
	return [4]float64{at(lats, lo), at(lngs, lo), at(lats, hi), at(lngs, hi)}
}

// Report is a plain-text data-quality summary for the terminal.
func (m Meta) Report() string {
	var b strings.Builder
	pct := func(n int) string {
		if m.UniquePermits == 0 {
			return "0%"
		}
		return fmt.Sprintf("%d (%.1f%%)", n, 100*float64(n)/float64(m.UniquePermits))
	}
	fmt.Fprintf(&b, "Rows read:            %d\n", m.Rows)
	fmt.Fprintf(&b, "Unique permits:       %d (%d repeat rows dropped)\n", m.UniquePermits, m.DuplicateRows)
	fmt.Fprintf(&b, "Date range:           %s to %s\n", m.DateMin, m.DateMax)
	fmt.Fprintf(&b, "Void/withdrawn/etc.:  %s (hidden by default)\n", pct(m.NoWork))
	fmt.Fprintf(&b, "No usable date:       %s\n", pct(m.NoDate))
	fmt.Fprintf(&b, "Implausible dates:   %s (skipped; next date field used)\n", pct(m.BadDates))
	fmt.Fprintf(&b, "Street from text:     %s (read from ADDRESS/LOCATION)\n", pct(m.StreetFromText))
	fmt.Fprintf(&b, "No geometry:          %s\n", pct(m.NoGeo))
	fmt.Fprintf(&b, "No street/block:      %s (cannot be mapped to a block)\n", pct(m.NoBlock))
	fmt.Fprintf(&b, "Street blocks:        %d\n", m.Blocks)
	kv := func(title string, xs []KV) {
		fmt.Fprintf(&b, "%s:\n", title)
		for _, x := range xs {
			fmt.Fprintf(&b, "  %6d  %s\n", x.N, x.Key)
		}
	}
	if len(m.Layers) > 0 {
		fmt.Fprintf(&b, "Source layers (rows read / kept / dropped as repeats of a permit number already seen):\n")
		for _, l := range m.Layers {
			fmt.Fprintf(&b, "  %7d / %7d / %7d  %s\n", l.Rows, l.Kept, l.Dupes, l.Layer)
		}
	}
	kv("Permits per year (by the date used)", m.PerYear)
	kv("Date field used", m.DateBasis)
	kv("Permit status", m.Statuses)
	kv("Category (derived)", m.Categories)
	kv("PERMIT_TYPE values", m.PermitTypes)
	kv("SUB_TYPE values (top 25)", m.SubTypes)
	return b.String()
}
