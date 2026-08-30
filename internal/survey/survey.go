package survey

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/fetch"
)

const DefaultService = "https://gis.tucsonaz.gov/arcgis/rest/services/PublicMaps/PermitsCode/MapServer"

var (
	// The fields the permit parser reads; a layer missing more than one of them isn't worth adding
	wantFields = []string{"PERMIT", "PERMIT_TYPE", "PERMIT_STATUS", "STREET_NO", "STREET_NAME", "DATE_ISSUED"}
	dateFields = []string{"DATE_ISSUED", "DATE_APPROVE", "DATE_START", "DATE_ENTERED", "CREATED_DATE", "DATE_ADDED"}
)

type row struct {
	id      int
	name    string
	url     string
	rows    int
	newest  time.Time
	fits    int
	missing []string
	have    bool
	err     error
}

func Run(ctx context.Context, w io.Writer, c *arcgis.Client, service string, have []fetch.Layer) error {
	fmt.Fprintf(w, "Examining the right-of-way permit layers in %s\n(each layer is asked for its row count, newest date and field names; this takes a minute)\n\n", service)
	rows, err := examine(ctx, c, service, have)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "No right-of-way permit layers found in this service.")
		return nil
	}

	var baseline time.Time
	fmt.Fprintf(w, "%5s  %9s  %-10s  %-5s  %s\n", "ID", "Rows", "Newest", "Fits", "Layer")
	for _, r := range rows {
		if r.have && r.newest.After(baseline) {
			baseline = r.newest
		}
		if r.err != nil {
			fmt.Fprintf(w, "%5d  %9s  %-10s  %-5s  %s   (could not read: %v)\n", r.id, "?", "?", "?", r.name, r.err)
			continue
		}
		newest := "none"
		switch {
		case !r.newest.IsZero():
			newest = r.newest.Format("2006-01-02")
		case r.rows > 0:
			newest = "unknown"
		}
		note := ""
		if r.have {
			note = "   (already downloaded)"
		}
		if len(r.missing) > 0 {
			note += "   (no " + strings.Join(r.missing, ", ") + ")"
		}
		fmt.Fprintf(w, "%5d  %9d  %-10s  %d/%-3d  %s%s\n", r.id, r.rows, newest, r.fits, len(wantFields), r.name, note)
	}
	fmt.Fprintf(w, "\nFits = how many of the fields this tool reads (%s) the layer has.\n", strings.Join(wantFields, ", "))
	if !baseline.IsZero() {
		fmt.Fprintf(w, "The layers you already download end %s.\n", baseline.Format("2006-01-02"))
	}

	var recs []string
	used := map[string]bool{}
	for _, r := range rows {
		if r.have || r.err != nil || r.rows == 0 || r.fits < len(wantFields)-1 || !r.newest.After(baseline) || !isStreetWork(r.name) {
			continue
		}
		name := suggestName(r.name)
		if used[name] {
			name = fmt.Sprintf("%s_%d", name, r.id)
		}
		used[name] = true
		recs = append(recs, fmt.Sprintf("  -extra %s=%s", name, r.url))
	}
	if len(recs) == 0 {
		fmt.Fprintln(w, "\nNo other layer fits and has newer permits.")
		return nil
	}
	fmt.Fprintf(w, "\nLayers that fit and have newer permits. Add any of these to fetch (repeat the flag for each):\n%s\n", strings.Join(recs, "\n"))
	return nil
}

func examine(ctx context.Context, c *arcgis.Client, service string, have []fetch.Layer) ([]row, error) {
	refs, err := c.ServiceLayers(ctx, service)
	if err != nil {
		return nil, err
	}
	known := downloaded(have)
	limit := time.Now().AddDate(1, 0, 0)
	var rows []row
	for _, ref := range refs {
		if !isRowPermitLayer(ref.Name) {
			continue
		}
		u := fmt.Sprintf("%s/%d", strings.TrimRight(service, "/"), ref.ID)
		r := row{id: ref.ID, name: ref.Name, url: u, have: known[normURL(u)]}
		info, err := c.Layer(ctx, u)
		if err == nil {
			r.rows, err = c.Count(ctx, u)
		}
		if err != nil {
			r.err = err
			rows = append(rows, r)
			continue
		}
		for _, f := range wantFields {
			if hasField(info.Fields, f) {
				r.fits++
			} else {
				r.missing = append(r.missing, f)
			}
		}
		if r.rows > 0 {
			r.newest = newestDate(ctx, c, u, info.Fields, limit)
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].newest.Equal(rows[j].newest) {
			return rows[i].newest.After(rows[j].newest)
		}
		return rows[i].id < rows[j].id
	})
	return rows, nil
}

func newestDate(ctx context.Context, c *arcgis.Client, layerURL string, fields []arcgis.Field, limit time.Time) time.Time {
	var newest time.Time
	for _, f := range fields {
		if f.Type != "esriFieldTypeDate" || !contains(dateFields, f.Name) {
			continue
		}
		where := fmt.Sprintf("%s <= timestamp '%s 00:00:00'", f.Name, limit.Format("2006-01-02"))
		_, hi, err := c.DateRangeWhere(ctx, layerURL, f.Name, where)
		if err != nil {
			// Some servers reject the filter.
			if _, hi, err = c.DateRange(ctx, layerURL, f.Name); err == nil && hi.After(limit) {
				hi = time.Time{}
			}
		}
		if err == nil && hi.After(newest) {
			newest = hi
		}
	}
	return newest
}

func downloaded(layers []fetch.Layer) map[string]bool {
	m := map[string]bool{}
	for _, l := range layers {
		m[normURL(l.URL)] = true
		for _, f := range l.Fallbacks {
			m[normURL(f)] = true
		}
	}
	return m
}

func normURL(u string) string { return strings.ToLower(strings.TrimRight(u, "/")) }

func hasField(fields []arcgis.Field, name string) bool {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func isRowPermitLayer(name string) bool {
	n := strings.ToUpper(name)
	return strings.Contains(n, "PERMIT") && (strings.Contains(n, "RW") || strings.Contains(n, "ROW") || strings.Contains(n, "TDOT"))
}

func isStreetWork(name string) bool {
	n := strings.ToUpper(name)
	return !strings.Contains(n, "EVENT") && !strings.Contains(n, "BANNER")
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// suggestName turns a city layer name into a local one that fetch will parse as permits
func suggestName(layer string) string {
	s := strings.Trim(nonWord.ReplaceAllString(strings.ToLower(layer), "_"), "_")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "tdot_"), "dtm_")
	if !strings.HasPrefix(s, "permits") {
		s = "permits_" + s
	}
	if len(s) > 48 {
		s = strings.TrimRight(s[:48], "_")
	}
	return s
}
