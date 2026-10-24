package site

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/permits"
	"github.com/yungbluth/streetworks/internal/store"
)

var update = flag.Bool("update", false, "rewrite testdata/golden from the current output")

func sampleInput(t *testing.T) Input {
	t.Helper()
	raw, err := os.ReadFile("../permits/testdata/real_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	var recs []permits.Record
	for i, p := range rows {
		geom := fmt.Sprintf(`{"type":"Point","coordinates":[%.5f,%.5f]}`, -110.9-float64(i)*0.004, 32.2+float64(i%3)*0.006)
		recs = append(recs, permits.FromFeature(arcgis.Feature{Properties: p, Geometry: json.RawMessage(geom)}, "permits_inactive"))
	}
	recs = append(recs, permits.Record{Permit: "NOGEO", Category: "Other", Status: "CLOSED", Block: -1}) // stays off the map
	kept, st := permits.Dedupe(recs, nil)
	places, unplaced := permits.AssignPlaces(kept)
	built := time.Date(2026, 8, 29, 18, 0, 0, 0, time.UTC)
	return Input{
		Records: kept, Places: places, Meta: permits.Summarize(kept, st, unplaced),
		Sources: []arcgis.SourceMeta{{Name: "permits_inactive", URL: "https://example.test/75", FetchedAt: built, Count: 10}},
		Overlays: []Overlay{{Name: "prop409_major", Label: "Prop 409 major streets", Features: []arcgis.Feature{
			{Properties: map[string]any{"NAME": "Broadway"}, Geometry: json.RawMessage(`{"type":"LineString","coordinates":[[-110.9,32.2],[-110.8,32.2]]}`)},
		}}},
		Basemap: DefaultBasemap(),
		Built:   built,
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestWriteGolden(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if _, err := Write(dir, filepath.Join(t.TempDir(), "streetworks.db"), sampleInput(t)); err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "golden")
	if *update {
		os.RemoveAll(golden)
		copyDir(t, dir, golden)
		return
	}
	var got, want []string
	list := func(root string, out *[]string) {
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(root, p)
				*out = append(*out, rel)
			}
			return err
		})
	}
	list(dir, &got)
	list(golden, &want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("files = %v\nwant    %v (run go test ./internal/site -update to accept)", got, want)
	}
	for _, f := range want {
		a, _ := os.ReadFile(filepath.Join(dir, f))
		b, _ := os.ReadFile(filepath.Join(golden, f))
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs from golden:\n%s", f, a)
		}
	}
}

type gotPlaces struct {
	Streets                    []string
	Kind, Street, Cross, Block []int
	Lat, Lng                   []int
	Label                      map[string]string
}

type gotPermits struct {
	Place, Year, Cat, NoWork, Lat, Lng []int
}

func TestWriteShape(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	in := sampleInput(t)
	sum, err := Write(dir, filepath.Join(t.TempDir(), "streetworks.db"), in)
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Version      int
		SparseBefore int
		Categories   []string
		DateRange    []string
		Bounds       []float64
		Overlays     []struct {
			Name, Label, File string
			Rows              int
		}
		Quality permits.Meta
		Sources []struct {
			Name, URL, Fetched string
			Rows               int
		}
	}
	readJSON(t, filepath.Join(dir, "index.json"), &idx)
	if idx.Version != 2 || idx.SparseBefore != 2005 || len(idx.Bounds) != 4 {
		t.Errorf("index: %+v", idx)
	}
	if len(idx.DateRange) != 2 || idx.DateRange[0] != in.Meta.DateMin || idx.Quality.UniquePermits != 11 || idx.Quality.Unplaced != 1 {
		t.Errorf("index: %+v", idx)
	}
	if len(idx.Sources) != 1 || idx.Sources[0].Rows != 10 || idx.Sources[0].Fetched != "2026-08-29T18:00:00Z" {
		t.Errorf("sources: %+v", idx.Sources)
	}
	if len(idx.Overlays) != 1 || idx.Overlays[0].File != "overlays/prop409_major.json" || idx.Overlays[0].Rows != 1 {
		t.Errorf("overlays: %+v", idx.Overlays)
	}
	var overlay struct {
		Type     string
		Features []struct{ Type string }
	}
	readJSON(t, filepath.Join(dir, idx.Overlays[0].File), &overlay)
	if overlay.Type != "FeatureCollection" || len(overlay.Features) != 1 || overlay.Features[0].Type != "Feature" {
		t.Errorf("overlay: %+v", overlay)
	}

	var pl gotPlaces
	readJSON(t, filepath.Join(dir, "places.json"), &pl)
	n := len(pl.Kind)
	for name, l := range map[string]int{"street": len(pl.Street), "cross": len(pl.Cross), "block": len(pl.Block), "lat": len(pl.Lat), "lng": len(pl.Lng)} {
		if l != n {
			t.Errorf("places.%s has %d entries, kind has %d", name, l, n)
		}
	}
	if n != len(in.Places) {
		t.Errorf("%d places written, %d given", n, len(in.Places))
	}
	for i := range pl.Kind {
		if s := pl.Street[i]; s >= len(pl.Streets) || (pl.Kind[i] != 2 && s < 0) {
			t.Errorf("place %d: street %d", i, s)
		}
		if _, labelled := pl.Label[fmt.Sprint(i)]; labelled != (pl.Kind[i] == 2) {
			t.Errorf("place %d (kind %d): label present = %v", i, pl.Kind[i], labelled)
		}
	}

	var pm gotPermits
	readJSON(t, filepath.Join(dir, "permits.json"), &pm)
	m := len(pm.Place)
	for name, l := range map[string]int{"year": len(pm.Year), "cat": len(pm.Cat), "noWork": len(pm.NoWork), "lat": len(pm.Lat), "lng": len(pm.Lng)} {
		if l != m {
			t.Errorf("permits.%s has %d entries, place has %d", name, l, m)
		}
	}
	if m != 10 || sum.Permits != 10 {
		t.Errorf("permits written = %d, summary says %d; want the 10 with coordinates", m, sum.Permits)
	}
	for i, p := range pm.Place {
		if p < 0 || p >= n || (i > 0 && p < pm.Place[i-1]) {
			t.Errorf("permit %d: place %d (must exist, and permits are ordered by place)", i, p)
		}
		if c := pm.Cat[i]; c < 0 || c >= len(idx.Categories) {
			t.Errorf("permit %d: category %d", i, c)
		}
	}

	if sum.FirstLoadBytes <= 0 || sum.FirstLoadGzip <= 0 || sum.FirstLoadGzip >= sum.FirstLoadBytes {
		t.Errorf("summary sizes: %+v", sum)
	}
}

func TestWriteDatabase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	dbPath := filepath.Join(t.TempDir(), "streetworks.db")
	if _, err := Write(dir, dbPath, sampleInput(t)); err != nil {
		t.Fatal(err)
	}
	var idx struct{ Built string }
	readJSON(t, filepath.Join(dir, "index.json"), &idx)
	var pm gotPermits
	readJSON(t, filepath.Join(dir, "permits.json"), &pm)
	n := len(pm.Place)

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	meta, err := db.Meta(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.Permits != n || meta.Built != idx.Built || meta.Version != version {
		t.Errorf("meta = %+v; want %d permits, built %q, version %d", meta, n, idx.Built, version)
	}
	ids := make([]int, n)
	for i := range ids {
		ids[i] = i
	}
	got, err := db.Lookup(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("looked up %d of %d permits", len(got), n)
	}
	_, mapped := placesFile(sampleInput(t).Places, sampleInput(t).Records)
	if want := sampleInput(t).Records[mapped[0].record].Permit; got[0].Permit != want {
		t.Errorf("permit 0 = %q, want %q", got[0].Permit, want)
	}
	var filed bool
	for _, d := range got {
		if d.Filed == "10602 E CERULEAN WAY" {
			filed = true
			if len(d.Flags) != 1 || d.Flags[0] != "street read from location text" {
				t.Errorf("flags = %v", d.Flags)
			}
		}
	}
	if !filed {
		t.Error("the permit whose street came from LOCATION wasn't written")
	}
}

func TestWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if _, err := Write(dir, filepath.Join(t.TempDir(), "streetworks.db"), sampleInput(t)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "index.json"))

	bad := sampleInput(t)
	bad.Built = bad.Built.Add(time.Hour) // so a half-finished write would show up in index.json
	bad.Overlays[0].Features[0].Geometry = json.RawMessage(`{not json`)
	if _, err := Write(dir, filepath.Join(t.TempDir(), "streetworks.db"), bad); err == nil {
		t.Fatal("a broken overlay should fail the write")
	}
	after, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Errorf("previous data was touched (err %v)", err)
	}
	if _, err := os.Stat(dir + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temporary directory left behind: %v", err)
	}
}

func TestOverlayNamesMustBeSafeFileNames(t *testing.T) {
	in := sampleInput(t)
	in.Overlays[0].Name = "../escape"
	if _, err := Write(filepath.Join(t.TempDir(), "data"), filepath.Join(t.TempDir(), "streetworks.db"), in); err == nil {
		t.Error("an overlay name with a path in it was accepted")
	}
}

func copyDir(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
