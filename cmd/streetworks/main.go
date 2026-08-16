package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/permits"
	"github.com/yungbluth/streetworks/internal/web"
)

var permitsService = "https://gis.tucsonaz.gov/arcgis/rest/services/PublicMaps/PermitsCode/MapServer"

const (
	prop409Service = "https://gis.tucsonaz.gov/arcgis/rest/services/TDOT/Prop409Public_rev/MapServer"
	openDataBase   = "https://publicgis.tucsonaz.gov/open/rest/services/OpenData/OpenData_Transportation/MapServer"
)

type layer struct{ Name, URL string }

var nameRE = regexp.MustCompile(`^[a-z0-9_]+$`)

// newClient is a variable so tests can use fast retry timing
var newClient = func() *arcgis.Client { return arcgis.New() }

// Layers whose URLs came from the city's own service directory
var defaultLayers = []layer{
	{"permits_inactive", permitsService + "/75"},
	{"prop409_major", prop409Service + "/813"},
	{"prop409_residential", prop409Service + "/814"},
	// the city's current right-of-way permit layers
	{"permits_row_active", permitsService + "/133"},
	{"permits_row_utility", permitsService + "/139"},
	{"permits_row_general", permitsService + "/135"},
	{"permits_row_cip_joc", permitsService + "/62"},
	{"permits_row_development", permitsService + "/64"},
	{"permits_row_homedev", permitsService + "/134"},
	{"permits_row_residential", permitsService + "/136"},
}

var fallbacks = map[string][]string{
	"prop409_major":       {openDataBase + "/13"},
	"prop409_residential": {openDataBase + "/14"},
}

func altsFor(l layer) []string {
	for _, d := range defaultLayers {
		if d.Name == l.Name && d.URL == l.URL {
			return fallbacks[l.Name]
		}
	}
	return nil
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return u
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(s string) error { *m = append(*m, s); return nil }

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: streetworks discover|survey|ingest|serve [flags]")
		os.Exit(2)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		signal.Stop(sigs) // from here a second Ctrl+C ends the program immediately
		fmt.Fprintln(os.Stderr, "\nstopping... (press Ctrl+C again to force quit)")
		cancel()
	}()
	var err error
	switch os.Args[1] {
	case "discover":
		err = discover(ctx, os.Stdout, os.Args[2:])
	case "ingest":
		err = ingest(ctx, os.Stdout, os.Args[2:])
	case "survey":
		err = survey(ctx, os.Stdout, os.Args[2:])
	case "serve":
		err = serve(ctx, os.Stdin, os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		log.Fatal(err)
	}
}

func layersFrom(extra multi, only string) ([]layer, error) {
	ls := append([]layer(nil), defaultLayers...)
	for _, e := range extra {
		n, u, ok := strings.Cut(e, "=")
		n, u = strings.TrimSpace(n), strings.TrimSpace(u)
		if !ok || n == "" || u == "" {
			return nil, fmt.Errorf("-extra wants name=url, got %q", e)
		}
		if !nameRE.MatchString(n) {
			return nil, fmt.Errorf("layer name %q must be lowercase letters, digits and underscores", n)
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return nil, fmt.Errorf("layer %q: url must start with http:// or https://, got %q", n, u)
		}
		replaced := false
		for i := range ls {
			if ls[i].Name == n {
				ls[i].URL, replaced = u, true
			}
		}
		if !replaced {
			ls = append(ls, layer{n, u})
		}
	}
	if only == "" {
		return ls, nil
	}
	known := map[string]layer{}
	for _, l := range ls {
		known[l.Name] = l
	}
	var out []layer
	for _, n := range strings.Split(only, ",") {
		n = strings.TrimSpace(n)
		l, ok := known[n]
		if !ok {
			return nil, fmt.Errorf("-only: unknown layer %q", n)
		}
		out = append(out, l)
	}
	return out, nil
}

// discover answers the questions that decide whether the data is good enough
func discover(ctx context.Context, w io.Writer, args []string) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	service := fs.String("service", permitsService, "MapServer URL whose layers to list")
	only := fs.String("only", "", "comma-separated layer names to describe (default all)")
	var extra multi
	fs.Var(&extra, "extra", "name=url of another layer to describe (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c := newClient()

	fmt.Fprintln(w, "Layers in", *service)
	refs, err := c.ServiceLayers(ctx, *service)
	if err != nil {
		fmt.Fprintln(w, "  could not list:", err)
	}
	for _, r := range refs {
		mark := ""
		if n := strings.ToUpper(r.Name); strings.Contains(n, "PERMIT") || strings.Contains(n, "_RW") {
			mark = "   <-- permits-related; add with -extra name=" + *service + "/" + fmt.Sprint(r.ID)
		}
		fmt.Fprintf(w, "  %4d  %s%s\n", r.ID, r.Name, mark)
	}
	ls, err := layersFrom(extra, *only)
	if err != nil {
		return err
	}
	for _, l := range ls {
		fmt.Fprintf(w, "\n== %s\n   %s\n", l.Name, l.URL)
		info, err := c.Layer(ctx, l.URL)
		if err != nil {
			fmt.Fprintln(w, "   FAILED:", err)
			continue
		}
		n, err := c.Count(ctx, l.URL)
		if err != nil {
			fmt.Fprintln(w, "   count failed:", err)
		}
		fmt.Fprintf(w, "   %s, %s, %d rows, max %d per request\n", info.Name, info.GeometryType, n, info.MaxRecordCount)
		var names []string
		for _, f := range info.Fields {
			names = append(names, f.Name+":"+strings.TrimPrefix(f.Type, "esriFieldType"))
		}
		fmt.Fprintln(w, "   fields:", strings.Join(names, " "))
		for _, f := range info.Fields {
			if f.Type != "esriFieldTypeDate" {
				continue
			}
			lo, hi, err := c.DateRange(ctx, l.URL, f.Name)
			if err != nil || lo.IsZero() {
				fmt.Fprintf(w, "   %-14s (no values)\n", f.Name)
				continue
			}
			fmt.Fprintf(w, "   %-14s %s to %s\n", f.Name, lo.Format("2006-01-02"), hi.Format("2006-01-02"))
		}
	}
	return nil
}

func ingest(ctx context.Context, w io.Writer, args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	out := fs.String("out", "data", "output directory")
	only := fs.String("only", "", "comma-separated layer names (default all)")
	page := fs.Int("page", 2000, "records per request")
	var extra multi
	fs.Var(&extra, "extra", "name=url of another layer (repeatable)")
	service := fs.String("service", permitsService, "MapServer checked for other permit layers")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *page <= 0 {
		return fmt.Errorf("-page must be positive, got %d", *page)
	}
	ls, err := layersFrom(extra, *only)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	c := newClient()
	failed := 0
	for _, l := range ls {
		fmt.Fprintf(w, "%s ...\n", l.Name)
		n, used, err := pullLayer(ctx, w, c, l, altsFor(l), *out, *page)
		if err != nil {
			failed++
			fmt.Fprintf(w, "  FAILED after %d rows: %v\n", n, err)
			if arcgis.IsUnreachable(err) {
				fmt.Fprintf(w, "  This is a network problem, not a data problem: this computer could not connect to %s.\n"+
					"  Check that https://%s opens in a browser, and whether a VPN, firewall or antivirus is blocking it.\n", hostOf(used), hostOf(used))
			}
			continue
		}
		fmt.Fprintf(w, "  %d rows saved\n", n)
		if want, err := c.Count(ctx, used); err == nil && want != n {
			fmt.Fprintf(w, "  WARNING: saved %d rows but the server reports %d. The layer may have changed during the download, or some rows could not be fetched.\n", n, want)
		}
		if n == 0 {
			fmt.Fprintf(w, "  WARNING: this layer returned no rows. Run: streetworks discover -only %s\n", l.Name)
		}
	}
	latest := ""
	if recs, err := loadPermits(*out); err == nil && len(recs) > 0 {
		m := permits.Build(recs).Meta
		fmt.Fprintf(w, "\nData quality for permit layers\n------------------------------\n%s", m.Report())
		latest = m.DateMax
	}
	suggestSurvey(ctx, w, c, *service, ls, latest)
	if failed > 0 {
		return fmt.Errorf("%d layer(s) failed; the others were saved", failed)
	}
	return nil
}

var nonName = regexp.MustCompile(`[^a-z0-9]+`)

func suggestName(layerName string) string {
	s := strings.Trim(nonName.ReplaceAllString(strings.ToLower(layerName), "_"), "_")
	for _, p := range []string{"tdot_", "dtm_"} {
		s = strings.TrimPrefix(s, p)
	}
	if !strings.HasPrefix(s, "permits") {
		s = "permits_" + s
	}
	if len(s) > 48 {
		s = strings.TrimRight(s[:48], "_")
	}
	return s
}

// isStreetWorkLayer is false for layers about events and banners
func isStreetWorkLayer(name string) bool {
	n := strings.ToUpper(name)
	return !strings.Contains(n, "EVENT") && !strings.Contains(n, "BANNER")
}

// isRowPermitLayer picks out right-of-way permit layers by name
func isRowPermitLayer(name string) bool {
	n := strings.ToUpper(name)
	return strings.Contains(n, "PERMIT") && (strings.Contains(n, "RW") || strings.Contains(n, "ROW") || strings.Contains(n, "TDOT"))
}

func normURL(u string) string { return strings.ToLower(strings.TrimRight(u, "/")) }

// pulledURLs is every address the given layers may be downloaded from
func pulledURLs(pulled []layer) map[string]bool {
	have := map[string]bool{}
	for _, l := range pulled {
		have[normURL(l.URL)] = true
		for _, a := range altsFor(l) {
			have[normURL(a)] = true
		}
	}
	return have
}

// suggestSurvey tells the user, briefly, that other permit layers exist
func suggestSurvey(ctx context.Context, w io.Writer, c *arcgis.Client, service string, pulled []layer, latest string) {
	if t, err := time.Parse("2006-01-02", latest); err == nil && t.After(time.Now().AddDate(0, -3, 0)) {
		return // the newest permit is recent, so there is nothing to chase
	}
	refs, err := c.ServiceLayers(ctx, service)
	if err != nil {
		return // a suggestion is a courtesy; never fail the run over it
	}
	have, n := pulledURLs(pulled), 0
	for _, r := range refs {
		if isRowPermitLayer(r.Name) && !have[normURL(fmt.Sprintf("%s/%d", strings.TrimRight(service, "/"), r.ID))] {
			n++
		}
	}
	if n == 0 {
		return
	}
	ends := ""
	if latest != "" {
		ends = " (the data you have ends " + latest + ")"
	}
	fmt.Fprintf(w, "\nThe city's permit service has %d other right-of-way permit layer%s that %s not downloaded%s.\n"+
		"The inactive layer only holds closed permits, so newer ones are probably elsewhere. To see which\n"+
		"layers have newer data and fit this tool, run:\n\n    go run ./cmd/streetworks survey\n", n, map[bool]string{true: "", false: "s"}[n == 1], map[bool]string{true: "was", false: "were"}[n == 1], ends)
}

// Fields the permit parser reads.
var (
	surveyFields = []string{"PERMIT", "PERMIT_TYPE", "PERMIT_STATUS", "STREET_NO", "STREET_NAME", "DATE_ISSUED"}
	surveyDates  = []string{"DATE_ISSUED", "DATE_APPROVE", "DATE_START", "DATE_ENTERED", "CREATED_DATE", "DATE_ADDED"}
)

type surveyRow struct {
	ID      int
	Name    string
	URL     string
	Rows    int
	Newest  time.Time
	Field   string
	Fits    int
	Missing []string
	Pulled  bool
	Err     error
}

func hasFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func surveyLayers(ctx context.Context, c *arcgis.Client, service string, pulled []layer) ([]surveyRow, error) {
	refs, err := c.ServiceLayers(ctx, service)
	if err != nil {
		return nil, err
	}
	have := pulledURLs(pulled)
	limitT := time.Now().AddDate(1, 0, 0)
	limit := limitT.Format("2006-01-02")
	var rows []surveyRow
	for _, r := range refs {
		if !isRowPermitLayer(r.Name) {
			continue
		}
		u := fmt.Sprintf("%s/%d", strings.TrimRight(service, "/"), r.ID)
		row := surveyRow{ID: r.ID, Name: r.Name, URL: u, Pulled: have[normURL(u)]}
		info, err := c.Layer(ctx, u)
		if err == nil {
			row.Rows, err = c.Count(ctx, u)
		}
		if err != nil {
			row.Err = err
			rows = append(rows, row)
			continue
		}
		for _, f := range surveyFields {
			found := false
			for _, lf := range info.Fields {
				if strings.EqualFold(lf.Name, f) {
					found = true
					break
				}
			}
			if found {
				row.Fits++
			} else {
				row.Missing = append(row.Missing, f)
			}
		}
		if row.Rows > 0 {
			for _, f := range info.Fields {
				if f.Type != "esriFieldTypeDate" || !hasFold(surveyDates, f.Name) {
					continue
				}
				_, hi, err := c.DateRangeWhere(ctx, u, f.Name, fmt.Sprintf("%s <= timestamp '%s 00:00:00'", f.Name, limit))
				if err != nil {
					// the server did not accept the filter
					if _, hi, err = c.DateRange(ctx, u, f.Name); err == nil && hi.After(limitT) {
						hi = time.Time{}
					}
				}
				if err == nil && hi.After(row.Newest) {
					row.Newest, row.Field = hi, f.Name
				}
			}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].Newest.Equal(rows[j].Newest) {
			return rows[i].Newest.After(rows[j].Newest)
		}
		return rows[i].ID < rows[j].ID
	})
	return rows, nil
}

func survey(ctx context.Context, w io.Writer, args []string) error {
	fs := flag.NewFlagSet("survey", flag.ContinueOnError)
	service := fs.String("service", permitsService, "MapServer whose permit layers to examine")
	var extra multi
	fs.Var(&extra, "extra", "name=url of a layer you already download (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pulled, err := layersFrom(extra, "")
	if err != nil {
		return err
	}
	c := newClient()
	fmt.Fprintf(w, "Examining the right-of-way permit layers in %s\n(each layer is asked for its row count, newest date and field names; this takes a minute)\n\n", *service)
	rows, err := surveyLayers(ctx, c, *service, pulled)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "No right-of-way permit layers found in this service.")
		return nil
	}
	fmt.Fprintf(w, "%5s  %9s  %-10s  %-5s  %s\n", "ID", "Rows", "Newest", "Fits", "Layer")
	var baseline time.Time
	for _, r := range rows {
		if r.Pulled && r.Newest.After(baseline) {
			baseline = r.Newest
		}
		switch {
		case r.Err != nil:
			fmt.Fprintf(w, "%5d  %9s  %-10s  %-5s  %s   (could not read: %v)\n", r.ID, "?", "?", "?", r.Name, r.Err)
		default:
			newest := "none"
			if r.Rows > 0 {
				newest = "unknown"
			}
			if !r.Newest.IsZero() {
				newest = r.Newest.Format("2006-01-02")
			}
			note := ""
			if r.Pulled {
				note = "   (already downloaded)"
			}
			if len(r.Missing) > 0 {
				note += "   (no " + strings.Join(r.Missing, ", ") + ")"
			}
			fmt.Fprintf(w, "%5d  %9d  %-10s  %d/%-3d  %s%s\n", r.ID, r.Rows, newest, r.Fits, len(surveyFields), r.Name, note)
		}
	}
	fmt.Fprintln(w, "\nFits = how many of the fields this tool reads (PERMIT, PERMIT_TYPE, PERMIT_STATUS, STREET_NO, STREET_NAME, DATE_ISSUED) the layer has.")
	if !baseline.IsZero() {
		fmt.Fprintf(w, "The layers you already download end %s.\n", baseline.Format("2006-01-02"))
	}
	used := map[string]bool{}
	var recs []string
	for _, r := range rows {
		if r.Pulled || r.Err != nil || r.Rows == 0 || r.Fits < len(surveyFields)-1 || !r.Newest.After(baseline) || !isStreetWorkLayer(r.Name) {
			continue
		}
		name := suggestName(r.Name)
		if used[name] {
			name = fmt.Sprintf("%s_%d", name, r.ID)
		}
		used[name] = true
		recs = append(recs, fmt.Sprintf("  -extra %s=%s", name, r.URL))
	}
	if len(recs) == 0 {
		fmt.Fprintln(w, "\nNo other layer fits and has newer permits.")
		return nil
	}
	fmt.Fprintf(w, "\nLayers that fit and have newer permits. Add any of these to ingest (and repeat the flag for each):\n%s\n", strings.Join(recs, "\n"))
	return nil
}

func pullLayer(ctx context.Context, w io.Writer, c *arcgis.Client, l layer, alts []string, dir string, page int) (int, string, error) {
	urls := append([]string{l.URL}, alts...)
	for i, u := range urls {
		n, err := pull(ctx, c, layer{l.Name, u}, dir, page)
		if err == nil || !arcgis.IsUnreachable(err) || i == len(urls)-1 {
			return n, u, err
		}
		fmt.Fprintf(w, "  could not reach %s; trying %s\n", hostOf(u), hostOf(urls[i+1]))
	}
	return 0, l.URL, nil // unreachable: urls is never empty
}

// pull writes to a temp file and renames, so a failed run never leaves a half-written layer behind
func pull(ctx context.Context, c *arcgis.Client, l layer, dir string, page int) (int, error) {
	path := filepath.Join(dir, l.Name+".jsonl")
	f, err := os.Create(path + ".tmp")
	if err != nil {
		return 0, err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	n, err := c.Fetch(ctx, l.URL, page, func(fs []arcgis.Feature) error {
		for _, ft := range fs {
			if err := enc.Encode(arcgis.Stored{Layer: l.Name, Feature: ft}); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		err = w.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path + ".tmp")
		return n, err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return n, err
	}
	meta, _ := json.Marshal(arcgis.SourceMeta{Name: l.Name, URL: l.URL, FetchedAt: time.Now().UTC(), Count: n})
	return n, os.WriteFile(filepath.Join(dir, l.Name+".meta.json"), meta, 0o644)
}

func loadPermits(dir string) ([]permits.Record, error) {
	paths, _ := filepath.Glob(filepath.Join(dir, "permits*.jsonl"))
	if len(paths) == 0 {
		return nil, fmt.Errorf("no permits*.jsonl in %s; run ingest first", dir)
	}
	return permits.LoadFiles(paths...)
}

func loadIndex(dir string) (*permits.Index, error) {
	recs, err := loadPermits(dir)
	if err != nil {
		return nil, err
	}
	return permits.Build(recs), nil
}

func watchForQuit(r io.Reader, stop func()) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		switch strings.ToLower(strings.TrimSpace(sc.Text())) {
		case "q", "quit", "exit", "stop":
			stop()
			return
		}
	}
}

func runServer(ctx context.Context, ln net.Listener, h http.Handler, grace time.Duration) error {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		srv.Close() // grace period over: drop whatever is still connected
	}
	<-errc
	return nil
}

func serve(ctx context.Context, stdin io.Reader, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dir := fs.String("data", "data", "data directory written by ingest")
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	carto := fs.String("carto-key", os.Getenv("STREETWORKS_CARTO_KEY"), "free CARTO basemap key from carto.com/basemaps/apikey (or set STREETWORKS_CARTO_KEY)")
	tiles := fs.String("tiles", "", "custom basemap tile URL template, e.g. https://host/{z}/{x}/{y}.png")
	tilesDark := fs.String("tiles-dark", "", "tile URL template for dark mode (default: same as -tiles)")
	credit := fs.String("attribution", "", "credit line shown on the map for a custom -tiles source")
	if err := fs.Parse(args); err != nil {
		return err
	}
	bm, err := web.NewBasemap(web.BasemapOptions{CartoKey: *carto, LightURL: *tiles, DarkURL: *tilesDark, Attribution: *credit})
	if err != nil {
		return err
	}
	ix, err := loadIndex(*dir)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w\nIs another copy of streetworks still running? Stop it, or choose another address with -addr", *addr, err)
	}
	m := ix.Meta
	log.Printf("loaded %d unique permits (%d blocks, %s to %s)", m.UniquePermits, m.Blocks, m.DateMin, m.DateMax)
	log.Printf("basemap: %s", bm.Describe())
	log.Printf("listening on http://%s", ln.Addr())
	log.Printf("press Ctrl+C to stop (or type q and press Enter)")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go watchForQuit(stdin, cancel)
	err = runServer(ctx, ln, (&web.Server{Index: ix, Dir: *dir, Basemap: bm}).Handler(), 5*time.Second)
	log.Printf("stopped")
	return err
}
