package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/fetch"
	"github.com/yungbluth/streetworks/internal/permits"
	"github.com/yungbluth/streetworks/internal/site"
)

// overlayLabels are the names the page shows for reference layers
var overlayLabels = map[string]string{
	"prop409_major":       "Prop 409 major streets",
	"prop409_residential": "Prop 409 residential streets",
}

func buildCmd(ctx context.Context, w io.Writer, args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	raw := fs.String("raw", "data/raw", "where fetch saved the layers")
	out := fs.String("out", "site/data", "where to write the site's data")
	guard := fs.String("guard", "", "URL of the published index.json; refuse to write if the permit count drops more than 10%")
	var bo site.BasemapOptions
	fs.StringVar(&bo.CartoKey, "carto-key", "", "CARTO basemap key (carto.com/basemaps/apikey)")
	fs.StringVar(&bo.LightURL, "tiles", "", "custom tile URL template, e.g. https://host/{z}/{x}/{y}.png")
	fs.StringVar(&bo.DarkURL, "tiles-dark", "", "tile URL template for dark mode (default: -tiles)")
	fs.StringVar(&bo.Attribution, "attribution", "", "plain-text credit for custom tiles")
	if err := fs.Parse(args); err != nil {
		return err
	}
	basemap, err := site.NewBasemap(bo)
	if err != nil {
		return err
	}

	files, _ := filepath.Glob(filepath.Join(*raw, "*.jsonl"))
	if len(files) == 0 {
		return fmt.Errorf("no layers in %s; run streetworks fetch first", *raw)
	}
	sort.Strings(files)

	priority := map[string]int{}
	for _, l := range fetch.Defaults() {
		priority[l.Name] = l.Priority
	}
	in := site.Input{Basemap: basemap, Built: time.Now().UTC()}
	var recs []permits.Record
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if meta, err := readSourceMeta(strings.TrimSuffix(path, ".jsonl") + ".meta.json"); err == nil {
			in.Sources = append(in.Sources, meta)
		} else {
			fmt.Fprintf(w, "note: %s has no .meta.json, so the page won't say when it was fetched\n", name)
		}
		if !(fetch.Layer{Name: name}).IsPermits() {
			o, err := readOverlay(path, name)
			if err != nil {
				return err
			}
			in.Overlays = append(in.Overlays, o)
			continue
		}
		if _, known := priority[name]; !known {
			priority[name] = 2 // an -extra permit layer counts as current
		}
		layer, err := permits.LoadFiles(path)
		if err != nil {
			return err
		}
		recs = append(recs, layer...)
	}

	kept, stats := permits.Dedupe(recs, priority)
	places, unplaced := permits.AssignPlaces(kept)
	in.Records, in.Places, in.Meta = kept, places, permits.Summarize(kept, stats, unplaced)
	fmt.Fprint(w, in.Meta.Report())

	if err := site.CheckGuard(ctx, &http.Client{Timeout: 30 * time.Second}, *guard, in.Meta.UniquePermits); err != nil {
		return err
	}
	sum, err := site.Write(*out, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%d permits on the map at %d places (%d without coordinates left off)\n", sum.Permits, len(places), unplaced)
	fmt.Fprintf(w, "first load: %.2f MB, %.2f MB compressed\n", float64(sum.FirstLoadBytes)/1e6, float64(sum.FirstLoadGzip)/1e6)
	fmt.Fprintf(w, "basemap: %s\n", basemap.Describe())
	fmt.Fprintf(w, "wrote %s\n", *out)
	return nil
}

func readSourceMeta(path string) (arcgis.SourceMeta, error) {
	var m arcgis.SourceMeta
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

func readOverlay(path, name string) (site.Overlay, error) {
	o := site.Overlay{Name: name, Label: overlayLabels[name]}
	if o.Label == "" {
		o.Label = name
	}
	f, err := os.Open(path)
	if err != nil {
		return o, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for dec.More() {
		var s arcgis.Stored
		if err := dec.Decode(&s); err != nil {
			return o, fmt.Errorf("%s: %w", path, err)
		}
		o.Features = append(o.Features, s.Feature)
	}
	return o, nil
}
