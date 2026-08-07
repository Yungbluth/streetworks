package permits

import (
	"sort"
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
	blocks  map[string]*Block
	loose   []*Record                 // permits with coordinates but no street block (often intersections)
	streets map[string][]*streetEntry // by core street name, for Search
	cores   []string                  // sorted keys of streets
	Meta    Meta
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
