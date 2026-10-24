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
	"github.com/yungbluth/streetworks/internal/store"
)

const (
	version = 2
	// Per-year counts roughly double from 2004 to 2005
	sparseBefore = 2005
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

var safeName = regexp.MustCompile(`^[a-z0-9_]+$`)

func Write(dir, dbPath string, in Input) (Summary, error) {
	tmp := dir + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return Summary{}, err
	}
	sum, db, err := writeAll(tmp, in)
	if err != nil {
		os.RemoveAll(tmp)
		return Summary{}, err
	}
	if err := store.Write(dbPath, db.meta, db.permits); err != nil {
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

type database struct {
	meta    store.Meta
	permits []store.Permit
}

func writeAll(dir string, in Input) (Summary, database, error) {
	w := &writer{dir: dir}

	cats, catID := categoryTable(in.Records)
	places, mapped := placesFile(in.Places, in.Records)
	pm := permitsFile{
		Place: make([]int, len(mapped)), Year: make([]int, len(mapped)), Cat: make([]int, len(mapped)),
		NoWork: make([]int, len(mapped)), Lat: make([]int, len(mapped)), Lng: make([]int, len(mapped)),
	}
	dbPermits := make([]store.Permit, len(mapped))
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
		dbPermits[i] = store.Permit{ID: i, Place: m.place, Year: pm.Year[i], Cat: pm.Cat[i], Detail: detailOf(r)}
	}

	idx := indexFile{
		Version: version, Built: in.Built.UTC().Format(time.RFC3339), Sources: []source{},
		Categories: cats, DateRange: []string{in.Meta.DateMin, in.Meta.DateMax}, SparseBefore: sparseBefore,
		Bounds: permits.PlaceBounds(in.Places), Quality: in.Meta, Basemap: in.Basemap,
		Overlays: []overlayRef{},
	}
	for _, s := range in.Sources {
		idx.Sources = append(idx.Sources, source{s.Name, s.URL, s.FetchedAt.UTC().Format(time.RFC3339), s.Count})
	}
	for _, o := range in.Overlays {
		if !safeName.MatchString(o.Name) {
			return Summary{}, database{}, fmt.Errorf("overlay name %q: use lowercase letters, digits and _", o.Name)
		}
		file := "overlays/" + o.Name + ".json"
		if err := w.json(file, geoJSON(o.Features), false); err != nil {
			return Summary{}, database{}, err
		}
		idx.Overlays = append(idx.Overlays, overlayRef{o.Name, o.Label, file, len(o.Features)})
	}

	for _, f := range []struct {
		name string
		v    any
	}{{"index.json", idx}, {"places.json", places}, {"permits.json", pm}} {
		if err := w.json(f.name, f.v, true); err != nil {
			return Summary{}, database{}, err
		}
	}
	var gz bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	zw.Write(w.firstLoad)
	zw.Close()
	sum := Summary{Permits: len(mapped), FirstLoadBytes: int64(len(w.firstLoad)), FirstLoadGzip: int64(gz.Len())}
	return sum, database{store.Meta{Built: idx.Built, Version: version, Permits: len(mapped)}, dbPermits}, nil
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

func detailOf(r permits.Record) store.Detail {
	d := store.Detail{
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
