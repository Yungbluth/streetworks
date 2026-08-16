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
	"strings"
	"syscall"

	"github.com/yungbluth/streetworks/internal/arcgis"
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
