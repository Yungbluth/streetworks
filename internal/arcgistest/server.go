package arcgistest

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
)

var whereRE = regexp.MustCompile(`(?i)^\s*(\w+)\s*<=\s*timestamp\s*'(\d{4}-\d{2}-\d{2}) 00:00:00'\s*$`)

const Base = "/arcgis/rest/services/Test/MapServer"

type Layer struct {
	ID             int
	Name           string
	Geometry       string
	Fields         []arcgis.Field
	Features       []arcgis.Feature
	MaxRecordCount int  // default 500
	IgnoreOffset   bool // simulate layers that do not support paging
	QueryStatus    int  // if set, /query answers with this HTTP status
	CountError     bool // returnCountOnly answers with an ArcGIS error body
	ReportedCount  int  // if set, returnCountOnly reports this instead of len(Features)
	RejectWhere    bool // statistics queries with any where other than 1=1 fail, like a server that rejects the filter
}

type Server struct {
	*httptest.Server
	mu  sync.Mutex
	log []url.Values
}

func Feat(props map[string]any, geom string) arcgis.Feature {
	f := arcgis.Feature{Properties: props}
	if geom != "" {
		f.Geometry = json.RawMessage(geom)
	}
	return f
}

func OIDField() arcgis.Field { return arcgis.Field{Name: "OBJECTID", Type: "esriFieldTypeOID"} }

func New(layers ...Layer) *Server {
	byID := map[string]Layer{}
	var refs []map[string]any
	for _, l := range layers {
		if l.MaxRecordCount == 0 {
			l.MaxRecordCount = 500
		}
		byID[strconv.Itoa(l.ID)] = l
		refs = append(refs, map[string]any{"id": l.ID, "name": l.Name})
	}
	s := &Server{}
	js := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	notFound := func(w http.ResponseWriter) {
		js(w, map[string]any{"error": map[string]any{"code": 400, "message": "Invalid layer"}})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+Base, func(w http.ResponseWriter, r *http.Request) { js(w, map[string]any{"layers": refs}) })
	mux.HandleFunc("GET "+Base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		l, ok := byID[r.PathValue("id")]
		if !ok {
			notFound(w)
			return
		}
		js(w, map[string]any{"name": l.Name, "geometryType": l.Geometry, "maxRecordCount": l.MaxRecordCount, "fields": l.Fields})
	})
	mux.HandleFunc("GET "+Base+"/{id}/query", func(w http.ResponseWriter, r *http.Request) {
		l, ok := byID[r.PathValue("id")]
		if !ok {
			notFound(w)
			return
		}
		q := r.URL.Query()
		s.mu.Lock()
		s.log = append(s.log, q)
		s.mu.Unlock()
		if l.QueryStatus != 0 {
			w.WriteHeader(l.QueryStatus)
			return
		}
		switch {
		case q.Get("returnCountOnly") == "true":
			if l.CountError {
				notFound(w)
				return
			}
			n := len(l.Features)
			if l.ReportedCount != 0 {
				n = l.ReportedCount
			}
			js(w, map[string]int{"count": n})
		case q.Get("outStatistics") != "":
			var st []struct {
				Field string `json:"onStatisticField"`
			}
			_ = json.Unmarshal([]byte(q.Get("outStatistics")), &st)
			cutoff := math.Inf(1)
			if where := q.Get("where"); where != "" && where != "1=1" {
				m := whereRE.FindStringSubmatch(where)
				if m == nil || l.RejectWhere {
					js(w, map[string]any{"error": map[string]any{"code": 400, "message": "Unable to complete operation."}})
					return
				}
				t, _ := time.Parse("2006-01-02", m[2])
				cutoff = float64(t.UnixMilli())
			}
			var lo, hi *float64
			for _, f := range l.Features {
				if v, ok := f.Properties[st[0].Field].(float64); ok && v <= cutoff {
					v := v
					if lo == nil || v < *lo {
						lo = &v
					}
					if hi == nil || v > *hi {
						hi = &v
					}
				}
			}
			js(w, map[string]any{"features": []map[string]any{{"attributes": map[string]any{"mn": lo, "mx": hi}}}})
		default:
			off, _ := strconv.Atoi(q.Get("resultOffset"))
			n, _ := strconv.Atoi(q.Get("resultRecordCount"))
			if l.IgnoreOffset {
				off = 0
			}
			if n <= 0 || n > l.MaxRecordCount {
				n = l.MaxRecordCount
			}
			if off > len(l.Features) {
				off = len(l.Features)
			}
			end := min(off+n, len(l.Features))
			out := make([]map[string]any, 0, end-off)
			for _, f := range l.Features[off:end] {
				out = append(out, map[string]any{"type": "Feature", "properties": f.Properties, "geometry": f.Geometry})
			}
			js(w, map[string]any{"type": "FeatureCollection", "features": out})
		}
	})
	s.Server = httptest.NewServer(mux)
	return s
}

func (s *Server) ServiceURL() string     { return s.URL + Base }
func (s *Server) LayerURL(id int) string { return fmt.Sprintf("%s/%d", s.ServiceURL(), id) }

// QueryLog returns the parameters of every /query request received
func (s *Server) QueryLog() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]url.Values(nil), s.log...)
}
