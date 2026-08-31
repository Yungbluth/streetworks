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
	StatusDiffered int         `json:"status_differed"` // duplicates whose copies disagreed on status
	NoWork         int         `json:"no_work"`
	NoBlock        int         `json:"no_block"`
	Unplaced       int         `json:"unplaced"` // no coordinates, so not on the map
	NoDate         int         `json:"no_date"`
	BadDates       int         `json:"bad_dates"`
	StreetFromText int         `json:"street_from_text"`
	DateMin        string      `json:"date_min"`
	DateMax        string      `json:"date_max"`
	DateBasis      []KV        `json:"date_basis"`
	Statuses       []KV        `json:"statuses"`
	Layers         []LayerStat `json:"layers"`
	PerYear        []KV        `json:"per_year"`
	Categories     []KV        `json:"categories"`
	PermitTypes    []KV        `json:"permit_types"`
	SubTypes       []KV        `json:"sub_types"`
	UnknownTypes   []KV        `json:"unknown_types"` // types the category table doesn't know yet
}

// LayerStat shows how much each source layer contributed
type LayerStat struct {
	Layer string `json:"layer"`
	Rows  int    `json:"rows"`
	Kept  int    `json:"kept"`
	Dupes int    `json:"dupes"`
}

func Summarize(recs []Record, d DedupeStats, unplaced int) Meta {
	m := Meta{
		Rows: d.Rows, UniquePermits: len(recs), DuplicateRows: d.Dropped,
		StatusDiffered: d.StatusDiffered, Unplaced: unplaced, Layers: d.PerLayer,
	}
	if m.Layers == nil {
		m.Layers = []LayerStat{}
	}
	basis, cats, types, subs, statuses, unknown := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	perYear := map[int]int{}
	var lo, hi time.Time
	for _, r := range recs {
		cats[r.Category]++
		statuses[r.Status]++
		types[r.Type]++
		if _, known := CategoryFor(r.Type); !known {
			unknown[r.Type]++
		}
		if r.SubType != "" {
			subs[r.SubType]++
		}
		if r.DateFlag != "" {
			m.BadDates++
		}
		if r.StreetSource != "" {
			m.StreetFromText++
		}
		if r.NoWork() {
			m.NoWork++
		}
		if r.Block < 0 || r.Street == "" {
			m.NoBlock++
		}
		if r.When.IsZero() {
			m.NoDate++
			continue
		}
		basis[r.DateBasis]++
		perYear[r.When.Year()]++
		if lo.IsZero() || r.When.Before(lo) {
			lo = r.When
		}
		if r.When.After(hi) {
			hi = r.When
		}
	}
	if !lo.IsZero() {
		m.DateMin, m.DateMax = lo.Format("2006-01-02"), hi.Format("2006-01-02")
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

	m.DateBasis, m.Statuses, m.Categories = top(basis, 0), top(statuses, 0), top(cats, 0)
	m.PermitTypes, m.SubTypes, m.UnknownTypes = top(types, 25), top(subs, 25), top(unknown, 0)
	return m
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

// PlaceBounds returns [south, west, north, east] around where most places are
func PlaceBounds(places []Place) [4]float64 {
	if len(places) == 0 {
		return [4]float64{}
	}
	lats, lngs := make([]float64, len(places)), make([]float64, len(places))
	for i, p := range places {
		lats[i], lngs[i] = p.Lat, p.Lng
	}
	sort.Float64s(lats)
	sort.Float64s(lngs)
	at := func(xs []float64, q float64) float64 { return xs[int(q*float64(len(xs)-1))] }
	return [4]float64{at(lats, 0.01), at(lngs, 0.01), at(lats, 0.99), at(lngs, 0.99)}
}

func (m Meta) Report() string {
	var b strings.Builder
	pct := func(n int) string {
		if m.UniquePermits == 0 {
			return "0"
		}
		return fmt.Sprintf("%d (%.1f%%)", n, 100*float64(n)/float64(m.UniquePermits))
	}
	fmt.Fprintf(&b, "Rows read:            %d\n", m.Rows)
	fmt.Fprintf(&b, "Unique permits:       %d (%d repeat rows dropped, %d of them with a different status)\n", m.UniquePermits, m.DuplicateRows, m.StatusDiffered)
	fmt.Fprintf(&b, "Date range:           %s to %s\n", m.DateMin, m.DateMax)
	fmt.Fprintf(&b, "Void/withdrawn/etc.:  %s (hidden by default)\n", pct(m.NoWork))
	fmt.Fprintf(&b, "No usable date:       %s\n", pct(m.NoDate))
	fmt.Fprintf(&b, "Implausible dates:    %s (skipped; next date field used)\n", pct(m.BadDates))
	fmt.Fprintf(&b, "Street from text:     %s (read from ADDRESS/LOCATION)\n", pct(m.StreetFromText))
	fmt.Fprintf(&b, "No street block:      %s (placed at an intersection or a point)\n", pct(m.NoBlock))
	fmt.Fprintf(&b, "Not on the map:       %s (no coordinates)\n", pct(m.Unplaced))
	kv := func(title string, xs []KV) {
		fmt.Fprintf(&b, "%s:\n", title)
		for _, x := range xs {
			fmt.Fprintf(&b, "  %6d  %s\n", x.N, x.Key)
		}
	}
	if len(m.Layers) > 0 {
		fmt.Fprintf(&b, "Source layers (rows read / kept / dropped as copies of a permit kept elsewhere):\n")
		for _, l := range m.Layers {
			fmt.Fprintf(&b, "  %7d / %7d / %7d  %s\n", l.Rows, l.Kept, l.Dupes, l.Layer)
		}
	}
	kv("Permits per year (by the date used)", m.PerYear)
	kv("Date field used", m.DateBasis)
	kv("Permit status", m.Statuses)
	kv("Category", m.Categories)
	kv("PERMIT_TYPE values (top 25)", m.PermitTypes)
	kv("SUB_TYPE values (top 25)", m.SubTypes)
	if len(m.UnknownTypes) > 0 {
		kv("Unknown permit types (shown as Other; add them to the category table)", m.UnknownTypes)
	}
	return b.String()
}
