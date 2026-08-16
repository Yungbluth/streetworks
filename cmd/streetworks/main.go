package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/permits"
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
