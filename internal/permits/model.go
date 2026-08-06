package permits

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
)

// Record is one right-of-way permit, normalized from the city's layer.
type Record struct {
	Layer        string    `json:"layer"`
	Permit       string    `json:"permit"`
	Type         string    `json:"type"`
	SubType      string    `json:"sub_type,omitempty"`
	Category     string    `json:"category"`
	Status       string    `json:"status"`
	Description  string    `json:"description"`
	Applicant    string    `json:"applicant,omitempty"`
	Utility      string    `json:"utility,omitempty"`
	Location     string    `json:"location,omitempty"`
	RawStreet    string    `json:"raw_street"`
	Street       string    `json:"street"`
	Block        int       `json:"block"`
	StreetSource string    `json:"street_source,omitempty"`
	DateFlag     string    `json:"date_flag,omitempty"`
	NoWorkStatus bool      `json:"no_work,omitempty"` // set when records are served, from the status
	GeocodeNote  string    `json:"geocode_note,omitempty"`
	ProURL       string    `json:"pro_url,omitempty"`
	Date         string    `json:"date,omitempty"`
	DateBasis    string    `json:"date_basis,omitempty"`
	Final        string    `json:"final,omitempty"`
	Lat          float64   `json:"lat,omitempty"`
	Lng          float64   `json:"lng,omitempty"`
	HasGeo       bool      `json:"-"`
	When         time.Time `json:"-"`
}

var addrRE = regexp.MustCompile(`^(\d+)\s+([NSEW])\.?\s+(\S.*)$`)

func addressFromText(s string) (no, dir, name string, ok bool) {
	m := addrRE.FindStringSubmatch(strings.ToUpper(strings.Join(strings.Fields(s), " ")))
	if m == nil || strings.ContainsAny(m[3], "&/") || !strings.ContainsAny(m[3], "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return
	}
	return m[1], m[2], m[3], true
}

// noWorkStatuses are permit states where no street work happened
var noWorkStatuses = map[string]bool{
	"VOID": true, "WITHDRWN": true, "WITHDRAWN": true, "CANCELED": true, "CANCELLED": true, "DENIED": true, "REVOKED": true,
	"IN REVIEW": true, "NEEDS RESUBMITTAL": true, "SUBMITTED": true, "SUBMITTED - ONLINE": true, "IN REVISION": true,
	"AWAITING CUSTOMER ATTENTION": true, "FEES DUE": true,
}

// NoWork reports whether the permit never led to street work, or has not been issued yet
func (r *Record) NoWork() bool { return noWorkStatuses[strings.ToUpper(strings.TrimSpace(r.Status))] }

func str(p map[string]any, k string) string {
	v, ok := p[k]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64: // JSON numbers; avoid "1e+06" for large street numbers
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func date(p map[string]any, k string) (time.Time, bool) {
	f, ok := p[k].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(f)).UTC(), true
}

func FromFeature(f arcgis.Feature, layer string) Record {
	p := f.Properties
	r := Record{
		Layer: layer, Permit: str(p, "PERMIT"), Type: str(p, "PERMIT_TYPE"), SubType: str(p, "SUB_TYPE"),
		Status: str(p, "PERMIT_STATUS"), Description: str(p, "DESCRIPTION"), Applicant: str(p, "APPLICANT"),
		Utility: str(p, "UTILITY"), Location: str(p, "LOCATION"), GeocodeNote: str(p, "GEOCODE_NOTE"),
		RawStreet: strings.Join(strings.Fields(str(p, "STREET_NO")+" "+str(p, "STREET_DIR")+" "+str(p, "STREET_NAME")), " "),
	}
	r.Category = Category(r.Type, r.SubType, str(p, "SUBTYPE_DESC"))
	if r.Category == "Other" {
		r.Category = Category(r.Description)
	}
	if u := str(p, "PRO_URL"); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		r.ProURL = u
	}
	// Different records fill different date fields; take the first plausible one and remember which
	maxDate := time.Now().AddDate(1, 0, 0)
	for _, k := range []string{"DATE_ISSUED", "DATE_APPROVE", "DATE_START", "DATE_ENTERED"} {
		t, ok := date(p, k)
		if !ok {
			continue
		}
		if t.Year() < 1980 || t.After(maxDate) {
			if r.DateFlag == "" {
				r.DateFlag = k + " " + t.Format("2006-01-02") + " is implausible"
			}
			continue
		}
		r.When, r.DateBasis, r.Date = t, k, t.Format("2006-01-02")
		break
	}
	if t, ok := date(p, "DATE_FINAL"); ok {
		r.Final = t.Format("2006-01-02")
	}
	no, dir, name := str(p, "STREET_NO"), str(p, "STREET_DIR"), str(p, "STREET_NAME")
	if name == "" { // many older rows only have the address in free text
		for _, k := range []string{"ADDRESS", "LOCATION"} {
			if n, d, nm, ok := addressFromText(str(p, k)); ok {
				no, dir, name, r.StreetSource = n, d, nm, k
				r.RawStreet = strings.Join(strings.Fields(str(p, k)), " ")
				break
			}
		}
	}
	r.Street = Street(dir, name)
	r.Block = BlockOf(no)
	if lat, lng, ok := Centroid(f.Geometry); ok {
		r.Lat, r.Lng, r.HasGeo = lat, lng, true
	}
	return r
}

func LoadFiles(paths ...string) ([]Record, error) {
	var out []Record
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		dec := json.NewDecoder(f)
		for {
			var s arcgis.Stored
			if err := dec.Decode(&s); err == io.EOF {
				break
			} else if err != nil {
				f.Close()
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			out = append(out, FromFeature(s.Feature, s.Layer))
		}
		f.Close()
	}
	return out, nil
}

func Centroid(raw json.RawMessage) (lat, lng float64, ok bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	var g struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if json.Unmarshal(raw, &g) != nil {
		return
	}
	switch g.Type {
	case "Point":
		var c []float64
		if json.Unmarshal(g.Coordinates, &c) == nil && len(c) >= 2 {
			lat, lng, ok = c[1], c[0], true
		}
	case "Polygon":
		var rings [][][]float64
		if json.Unmarshal(g.Coordinates, &rings) == nil && len(rings) > 0 {
			lat, lng, ok = ringCentroid(rings[0])
		}
	case "MultiPolygon":
		var polys [][][][]float64
		if json.Unmarshal(g.Coordinates, &polys) == nil && len(polys) > 0 && len(polys[0]) > 0 {
			lat, lng, ok = ringCentroid(polys[0][0])
		}
	case "LineString":
		var pts [][]float64
		if json.Unmarshal(g.Coordinates, &pts) == nil {
			lat, lng, ok = meanOf(pts)
		}
	}
	if ok {
		valid := !math.IsNaN(lat) && !math.IsNaN(lng) && math.Abs(lat) <= 90 && math.Abs(lng) <= 180 && !(lat == 0 && lng == 0)
		if !valid {
			return 0, 0, false
		}
	}
	return
}

func meanOf(pts [][]float64) (lat, lng float64, ok bool) {
	n := 0
	for _, p := range pts {
		if len(p) >= 2 {
			lng += p[0]
			lat += p[1]
			n++
		}
	}
	if n == 0 {
		return 0, 0, false
	}
	return lat / float64(n), lng / float64(n), true
}

func ringCentroid(raw [][]float64) (lat, lng float64, ok bool) {
	// Malformed vertices (fewer than two numbers) are dropped, not trusted
	r := make([][]float64, 0, len(raw))
	for _, v := range raw {
		if len(v) >= 2 {
			r = append(r, v)
		}
	}
	if len(r) < 3 {
		return meanOf(r)
	}
	ox, oy := r[0][0], r[0][1]
	var a, cx, cy float64
	for i := range r {
		j := (i + 1) % len(r)
		x1, y1, x2, y2 := r[i][0]-ox, r[i][1]-oy, r[j][0]-ox, r[j][1]-oy
		cr := x1*y2 - x2*y1
		a += cr
		cx += (x1 + x2) * cr
		cy += (y1 + y2) * cr
	}
	if math.Abs(a) < 1e-18 {
		return meanOf(r)
	}
	return oy + cy/(3*a), ox + cx/(3*a), true
}
