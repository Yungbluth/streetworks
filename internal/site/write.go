// Package site writes the static files the map page loads
package site

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/permits"
)

const (
	version = 1
	// Per-year counts roughly double from 2004 to 2005
	sparseBefore = 2005
	// detailCell is the size of a detail square in degrees, about 1.1 km by 0.9 km in Tucson
	detailCell = 0.01
)

type Overlay struct {
	Name, Label string
	Features    []arcgis.Feature
}

// Input is everything a build has worked out
type Input struct {
	Records  []permits.Record // deduped
	Places   []permits.Place  // from AssignPlaces(Records)
	Meta     permits.Meta
	Sources  []arcgis.SourceMeta
	Overlays []Overlay
	Basemap  Basemap
	Built    time.Time
}

type Summary struct {
	Permits        int   // permits on the map
	FirstLoadBytes int64 // index, places and permits, as written
	FirstLoadGzip  int64 // the same, compressed the way a server would send them
}

// fixed is a coordinate in degrees times 1e5, about a metre of precision
func fixed(deg float64) int { return int(math.Round(deg * 1e5)) }

func CellKey(lat, lng float64) string {
	floorDiv := func(n int) int { // floor(n / 1000), rounding toward minus infinity
		q := n / 1000
		if n%1000 != 0 && n < 0 {
			q--
		}
		return q
	}
	return strconv.Itoa(floorDiv(fixed(lat))) + "_" + strconv.Itoa(floorDiv(fixed(lng)))
}

var safeName = regexp.MustCompile(`^[a-z0-9_]+$`)

// Write replaces dir with the site's data files
func Write(dir string, in Input) (Summary, error) {
	tmp := dir + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return Summary{}, err
	}
	sum, err := writeAll(tmp, in)
	if err != nil {
		os.RemoveAll(tmp)
		return Summary{}, err
	}
	old := dir + ".old"
	os.RemoveAll(old)
	if err := os.Rename(dir, old); err != nil && !os.IsNotExist(err) {
		os.RemoveAll(tmp)
		return Summary{}, err
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.Rename(old, dir)
		return Summary{}, err
	}
	return sum, os.RemoveAll(old)
}

type writer struct {
	dir       string
	firstLoad []byte
}

func (w *writer) json(name string, v any, firstLoad bool) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // "GOLF LINKS RD & CATHY DR" stays readable
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	b := buf.Bytes()
	if firstLoad {
		w.firstLoad = append(w.firstLoad, b...)
	}
	path := filepath.Join(w.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeAll(dir string, in Input) (Summary, error) {
	w := &writer{dir: dir}

	cats, catID := categoryTable(in.Records)
	places, mapped := placesFile(in.Places, in.Records)
	pm := permitsFile{
		Place: make([]int, len(mapped)), Year: make([]int, len(mapped)), Cat: make([]int, len(mapped)),
		NoWork: make([]int, len(mapped)), Lat: make([]int, len(mapped)), Lng: make([]int, len(mapped)),
	}
	details := map[string]map[string]detail{}
	for i, m := range mapped {
		r := in.Records[m.record]
		pm.Place[i], pm.Cat[i] = m.place, catID[r.Category]
		pm.Lat[i], pm.Lng[i] = fixed(r.Lat), fixed(r.Lng)
		if !r.When.IsZero() {
			pm.Year[i] = r.When.Year()
		}
		if r.NoWork() {
			pm.NoWork[i] = 1
		}
		key := CellKey(r.Lat, r.Lng)
		if details[key] == nil {
			details[key] = map[string]detail{}
		}
		details[key][strconv.Itoa(i)] = detailOf(r)
	}

	idx := indexFile{
		Version: version, Built: in.Built.UTC().Format(time.RFC3339), Sources: []source{},
		Categories: cats, DateRange: []string{in.Meta.DateMin, in.Meta.DateMax}, SparseBefore: sparseBefore,
		Bounds: permits.PlaceBounds(in.Places), Quality: in.Meta, Basemap: in.Basemap,
		Overlays: []overlayRef{}, DetailCell: detailCell,
	}
	for _, s := range in.Sources {
		idx.Sources = append(idx.Sources, source{s.Name, s.URL, s.FetchedAt.UTC().Format(time.RFC3339), s.Count})
	}
	for _, o := range in.Overlays {
		if !safeName.MatchString(o.Name) {
			return Summary{}, fmt.Errorf("overlay name %q: use lowercase letters, digits and _", o.Name)
		}
		file := "overlays/" + o.Name + ".json"
		if err := w.json(file, geoJSON(o.Features), false); err != nil {
			return Summary{}, err
		}
		idx.Overlays = append(idx.Overlays, overlayRef{o.Name, o.Label, file, len(o.Features)})
	}

	for _, f := range []struct {
		name string
		v    any
	}{{"index.json", idx}, {"places.json", places}, {"permits.json", pm}} {
		if err := w.json(f.name, f.v, true); err != nil {
			return Summary{}, err
		}
	}
	for key, cell := range details {
		if err := w.json("details/"+key+".json", cell, false); err != nil {
			return Summary{}, err
		}
	}

	var gz bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	zw.Write(w.firstLoad)
	zw.Close()
	return Summary{Permits: len(mapped), FirstLoadBytes: int64(len(w.firstLoad)), FirstLoadGzip: int64(gz.Len())}, nil
}

type indexFile struct {
	Version      int          `json:"version"`
	Built        string       `json:"built"`
	Sources      []source     `json:"sources"`
	Categories   []string     `json:"categories"`
	DateRange    []string     `json:"dateRange"`
	SparseBefore int          `json:"sparseBefore"`
	Bounds       [4]float64   `json:"bounds"`
	Quality      permits.Meta `json:"quality"`
	Basemap      Basemap      `json:"basemap"`
	Overlays     []overlayRef `json:"overlays"`
	DetailCell   float64      `json:"detailCell"`
}

type source struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Fetched string `json:"fetched"`
	Rows    int    `json:"rows"`
}

type overlayRef struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	File  string `json:"file"`
	Rows  int    `json:"rows"`
}

func categoryTable(recs []permits.Record) ([]string, map[string]int) {
	seen := map[string]bool{}
	for _, r := range recs {
		seen[r.Category] = true
	}
	cats := []string{}
	for c := range seen {
		if c != "Other" {
			cats = append(cats, c)
		}
	}
	sort.Strings(cats)
	if seen["Other"] {
		cats = append(cats, "Other")
	}
	ids := map[string]int{}
	for i, c := range cats {
		ids[c] = i
	}
	return cats, ids
}

type placesJSON struct {
	Streets []string          `json:"streets"`
	Kind    []int             `json:"kind"`
	Street  []int             `json:"street"`
	Cross   []int             `json:"cross"`
	Block   []int             `json:"block"`
	Lat     []int             `json:"lat"`
	Lng     []int             `json:"lng"`
	Label   map[string]string `json:"label"`
}

type permitsFile struct {
	Place  []int `json:"place"`
	Year   []int `json:"year"`
	Cat    []int `json:"cat"`
	NoWork []int `json:"noWork"`
	Lat    []int `json:"lat"`
	Lng    []int `json:"lng"`
}

type mappedPermit struct{ record, place int }

// placesFile turns places into parallel arrays and lists the permits on the map in place order
func placesFile(places []permits.Place, recs []permits.Record) (placesJSON, []mappedPermit) {
	n := len(places)
	out := placesJSON{
		Streets: []string{}, Kind: make([]int, n), Street: make([]int, n), Cross: make([]int, n),
		Block: make([]int, n), Lat: make([]int, n), Lng: make([]int, n), Label: map[string]string{},
	}
	streetID := map[string]int{}
	street := func(name string) int {
		if name == "" {
			return -1
		}
		id, ok := streetID[name]
		if !ok {
			id = len(out.Streets)
			streetID[name] = id
			out.Streets = append(out.Streets, name)
		}
		return id
	}
	var mapped []mappedPermit
	for i, p := range places {
		out.Kind[i], out.Street[i], out.Cross[i], out.Block[i] = int(p.Kind), street(p.Street), street(p.Cross), p.Block
		out.Lat[i], out.Lng[i] = fixed(p.Lat), fixed(p.Lng)
		if p.Kind == permits.KindPoint {
			out.Label[strconv.Itoa(i)] = p.Label
		}
		for _, r := range p.Permits {
			mapped = append(mapped, mappedPermit{record: r, place: i})
		}
	}
	return out, mapped
}

type detail struct {
	Permit      string   `json:"permit"`
	Type        string   `json:"type"`
	SubType     string   `json:"subType,omitempty"`
	Status      string   `json:"status"`
	NoWork      bool     `json:"noWork"`
	Date        string   `json:"date"`
	DateBasis   string   `json:"dateBasis"`
	Final       string   `json:"final,omitempty"`
	Description string   `json:"description"`
	Applicant   string   `json:"applicant,omitempty"`
	Utility     string   `json:"utility,omitempty"`
	Filed       string   `json:"filed"`
	Link        string   `json:"link,omitempty"`
	Located     string   `json:"located,omitempty"` // how the city placed the point, e.g. "Mapguide Location"
	Flags       []string `json:"flags,omitempty"`
}

func detailOf(r permits.Record) detail {
	d := detail{
		Permit: r.Permit, Type: r.Type, SubType: r.SubType, Status: r.Status, NoWork: r.NoWork(),
		Date: r.Date, DateBasis: r.DateBasis, Final: r.Final, Description: r.Description,
		Applicant: r.Applicant, Utility: r.Utility, Filed: r.RawStreet, Link: r.ProURL, Located: r.GeocodeNote,
	}
	if d.Filed == "" {
		d.Filed = strings.Join(strings.Fields(r.Location), " ")
	}
	if r.StreetSource != "" {
		d.Flags = append(d.Flags, "street read from location text")
	}
	if r.DateFlag != "" {
		d.Flags = append(d.Flags, r.DateFlag)
	}
	return d
}

type feature struct {
	Type       string          `json:"type"`
	Properties map[string]any  `json:"properties"`
	Geometry   json.RawMessage `json:"geometry"`
}

func geoJSON(fs []arcgis.Feature) any {
	out := make([]feature, len(fs))
	for i, f := range fs {
		geom := f.Geometry
		if len(geom) == 0 {
			geom = json.RawMessage("null")
		}
		out[i] = feature{"Feature", f.Properties, geom}
	}
	return struct {
		Type     string    `json:"type"`
		Features []feature `json:"features"`
	}{"FeatureCollection", out}
}
