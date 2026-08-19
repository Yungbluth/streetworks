package web

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/permits"
)

//go:embed static
var static embed.FS

type Server struct {
	Index   *permits.Index
	Dir     string  // data directory holding the JSONL files
	Basemap Basemap // zero value means DefaultBasemap()
}

var nameRE = regexp.MustCompile(`^[a-z0-9_]+$`)

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(static, "static")
	mux.Handle("GET /", http.FileServer(http.FS(sub)))
	mux.HandleFunc("GET /api/config", s.config)
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("GET /api/nearby", s.nearby)
	mux.HandleFunc("GET /api/street", s.street)
	mux.HandleFunc("GET /api/meta", s.meta)
	mux.HandleFunc("GET /api/blocks", s.blocks)
	mux.HandleFunc("GET /api/block", s.block)
	mux.HandleFunc("GET /api/overlay/{name}", s.overlay)
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func parseFilter(r *http.Request) permits.Filter {
	q := r.URL.Query()
	f := permits.Filter{NoWork: q.Get("nowork") == "1"}
	f.From, _ = strconv.Atoi(q.Get("from"))
	f.To, _ = strconv.Atoi(q.Get("to"))
	// no cats parameter means every category
	if q.Has("cats") {
		f.Cats = map[string]bool{}
		for _, x := range strings.Split(q.Get("cats"), ",") {
			if x = strings.TrimSpace(x); x != "" {
				f.Cats[x] = true
			}
		}
	}
	return f
}

// parseBounds reads bbox=south,west,north,east
func parseBounds(r *http.Request) (*permits.Bounds, error) {
	raw := r.URL.Query().Get("bbox")
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return nil, fmt.Errorf("bbox wants south,west,north,east")
	}
	var v [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("bbox value %q is not a number", p)
		}
		v[i] = f
	}
	b := permits.Bounds{South: v[0], West: v[1], North: v[2], East: v[3]}
	if b.South > b.North || b.West > b.East || math.Abs(b.South) > 90 || math.Abs(b.North) > 90 || math.Abs(b.West) > 180 || math.Abs(b.East) > 180 {
		return nil, fmt.Errorf("bbox is not a valid south,west,north,east rectangle")
	}
	return &b, nil
}

func (s *Server) sources() []arcgis.SourceMeta {
	out := []arcgis.SourceMeta{} // never nil: the UI calls .map on it
	files, _ := filepath.Glob(filepath.Join(s.Dir, "*.meta.json"))
	for _, p := range files {
		var m arcgis.SourceMeta
		if b, err := os.ReadFile(p); err == nil && json.Unmarshal(b, &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func (s *Server) overlays() []string {
	out := []string{}
	files, _ := filepath.Glob(filepath.Join(s.Dir, "*.jsonl"))
	for _, p := range files {
		n := strings.TrimSuffix(filepath.Base(p), ".jsonl")
		if !strings.HasPrefix(n, "permits") && nameRE.MatchString(n) {
			out = append(out, n)
		}
	}
	return out
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	writeJSON(w, struct {
		Places []permits.Place `json:"places"`
	}{s.Index.Search(r.URL.Query().Get("q"), limit)})
}

// maxNearbyRecords caps how many permits one nearby answer lists
const maxNearbyRecords = 300

func parseCoord(v string, limit float64) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > limit {
		return 0, false
	}
	return f, true
}

func (s *Server) nearby(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, ok1 := parseCoord(q.Get("lat"), 90)
	lng, ok2 := parseCoord(q.Get("lng"), 180)
	rad, ok3 := parseCoord(q.Get("r"), permits.MaxRadius)
	if !ok1 || !ok2 || !ok3 || rad <= 0 {
		http.Error(w, fmt.Sprintf("nearby wants lat, lng and r (meters, up to %.0f)", permits.MaxRadius), http.StatusBadRequest)
		return
	}
	writeJSON(w, s.Index.Nearby(lat, lng, rad, parseFilter(r), maxNearbyRecords))
}

func (s *Server) street(w http.ResponseWriter, r *http.Request) {
	p, ok := s.Index.Street(r.URL.Query().Get("name"), parseFilter(r))
	if !ok {
		http.Error(w, "unknown street", http.StatusNotFound)
		return
	}
	writeJSON(w, p)
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	bm := s.Basemap
	if bm.Light.URL == "" {
		bm = DefaultBasemap()
	}
	writeJSON(w, struct {
		Basemap Basemap `json:"basemap"`
	}{bm})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, struct {
		permits.Meta
		Sources     []arcgis.SourceMeta `json:"sources"`
		Overlays    []string            `json:"overlays"`
		Attribution string              `json:"attribution"`
	}{s.Index.Meta, s.sources(), s.overlays(), "Data: City of Tucson GIS Services, provided as-is."})
}

func (s *Server) blocks(w http.ResponseWriter, r *http.Request) {
	f := parseFilter(r)
	b, err := parseBounds(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.Bounds = b
	out := s.Index.Blocks(f)
	if out == nil {
		out = []permits.BlockSummary{}
	}
	// drawing tens of thousands of map markers freezes browsers
	w.Header().Set("X-Total-Blocks", strconv.Itoa(len(out)))
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n < len(out) {
		out = out[:n]
	}
	writeJSON(w, out)
}

func (s *Server) block(w http.ResponseWriter, r *http.Request) {
	d, ok := s.Index.Block(r.URL.Query().Get("key"), parseFilter(r))
	if !ok {
		http.Error(w, "unknown block", http.StatusNotFound)
		return
	}
	writeJSON(w, d)
}

// overlay streams a stored non-permit layer
func (s *Server) overlay(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !nameRE.MatchString(name) || strings.HasPrefix(name, "permits") {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.Dir, name+".jsonl"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/geo+json")
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	bw.WriteString(`{"type":"FeatureCollection","features":[`)
	dec := json.NewDecoder(f)
	enc := json.NewEncoder(bw)
	for i := 0; ; i++ {
		var st arcgis.Stored
		if err := dec.Decode(&st); err == io.EOF || err != nil {
			break
		}
		if i > 0 {
			bw.WriteString(",")
		}
		_ = enc.Encode(struct {
			Type       string          `json:"type"`
			Properties map[string]any  `json:"properties"`
			Geometry   json.RawMessage `json:"geometry"`
		}{"Feature", st.Properties, st.Geometry})
	}
	bw.WriteString("]}")
}
