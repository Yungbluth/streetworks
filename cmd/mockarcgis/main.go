package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

type feat struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Geometry   any            `json:"geometry"`
}

const maxRecords = 500

func ms(t time.Time) float64 { return float64(t.UnixMilli()) }

func main() {
	addr := flag.String("addr", "127.0.0.1:9100", "listen address")
	n := flag.Int("n", 2500, "number of permit rows")
	flag.Parse()
	r := rand.New(rand.NewSource(42))

	type street struct {
		name string
		lat  float64
	}
	streets := []street{{"SAGUARO", 32.230}, {"MESQUITE", 32.236}, {"IRONWOOD", 32.242}, {"CHOLLA", 32.248}, {"22ND", 32.254}}
	suffix := [][]string{{"BLVD", "Boulevard", "BLVD."}, {"AVE", "Avenue", "AV"}, {"RD", "Road", "RD."}, {"ST", "Street", "ST"}, {"WAY", "Way", "WAY"}}
	subs := []string{"Excavation", "Barricade", "Combo", "Civic Event"}
	statuses := []string{"Final", "Final", "Final", "Expired", "Void", "Issued"}
	utils := []string{"Water", "Gas", "Telecom", "Electric", "Sewer"}

	var permits []feat
	oid := 1
	for len(permits) < *n {
		si := r.Intn(len(streets))
		// hot corridors: weight the first two streets and a couple of blocks
		if r.Float64() < 0.35 {
			si = r.Intn(2)
		}
		s := streets[si]
		suf := suffix[si]
		num := 1000 + r.Intn(40)*100 + r.Intn(100)
		if r.Float64() < 0.3 {
			num = 2300 + r.Intn(100)
		}
		lng := -110.99 + float64(num-1000)/4000*0.06
		issued := time.Date(2019+r.Intn(8), time.Month(1+r.Intn(12)), 1+r.Intn(28), 0, 0, 0, 0, time.UTC)
		if issued.After(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
			issued = issued.AddDate(-1, 0, 0)
		}
		sub := subs[r.Intn(len(subs))]
		u := utils[r.Intn(len(utils))]
		p := map[string]any{
			"OBJECTID": oid, "PERMIT": fmt.Sprintf("TR-ROW-%02d%02d-%05d", int(issued.Month()), issued.Year()%100, r.Intn(99999)),
			"PERMIT_TYPE": "Right-of-Way (ROW)", "SUB_TYPE": sub, "SUBTYPE_DESC": sub + " permit",
			"PERMIT_STATUS": statuses[r.Intn(len(statuses))], "DATE_ISSUED": ms(issued), "DATE_ENTERED": ms(issued.AddDate(0, 0, -3)),
			"DESCRIPTION": u + " work: " + sub, "UTILITY": u, "APPLICANT": "Example Contractor " + strconv.Itoa(r.Intn(40)),
			"STREET_NO": strconv.Itoa(num), "STREET_DIR": "E", "STREET_NAME": s.name + " " + suf[r.Intn(3)],
			"GEOCODE_NOTE": []string{"Matched", "Matched", "Intersection"}[r.Intn(3)], "PRO_URL": "https://example.com/pro/" + strconv.Itoa(oid),
		}
		if r.Float64() < 0.12 {
			p["DATE_ISSUED"] = nil
		}
		if r.Float64() < 0.04 {
			p["STREET_NO"] = ""
		}
		switch x := r.Float64(); {
		case x < 0.15:
			p["STREET_DIR"], p["STREET_NAME"] = "", "EAST "+s.name+" "+suf[1]
		case x < 0.25:
			p["STREET_DIR"] = "E."
		}
		if p["PERMIT_STATUS"] == "Final" {
			p["DATE_FINAL"] = ms(issued.AddDate(0, 0, 5+r.Intn(60)))
		}
		var geom any
		if r.Float64() > 0.03 {
			d := 0.00008
			lat := s.lat + (r.Float64()-.5)*0.0004
			geom = map[string]any{"type": "Polygon", "coordinates": [][][]float64{{{lng - d, lat - d}, {lng + d, lat - d}, {lng + d, lat + d}, {lng - d, lat + d}, {lng - d, lat - d}}}}
		}
		permits = append(permits, feat{"Feature", p, geom})
		oid++
		// the city repeats a permit once per unit/building
		if r.Float64() < 0.10 {
			dup := map[string]any{}
			for k, v := range p {
				dup[k] = v
			}
			dup["OBJECTID"] = oid
			permits = append(permits, feat{"Feature", dup, geom})
			oid++
		}
	}
	var overlay []feat
	for i, s := range streets {
		overlay = append(overlay, feat{"Feature", map[string]any{"OBJECTID": i + 1, "STREETNAME": s.name, "STATUS": "Complete", "FINISHED": ms(time.Date(2023, 5, 1, 0, 0, 0, 0, time.UTC))},
			map[string]any{"type": "LineString", "coordinates": [][]float64{{-110.99, s.lat}, {-110.93, s.lat}}}})
	}

	layers := map[string]struct {
		name   string
		geom   string
		fields []map[string]string
		feats  []feat
	}{
		"75": {"MOCK_PERMITS", "esriGeometryPolygon", []map[string]string{{"name": "OBJECTID", "type": "esriFieldTypeOID"}, {"name": "PERMIT", "type": "esriFieldTypeString"}, {"name": "DATE_ISSUED", "type": "esriFieldTypeDate"}, {"name": "DATE_ENTERED", "type": "esriFieldTypeDate"}}, permits},
		"13": {"MOCK_RESURFACING", "esriGeometryPolyline", []map[string]string{{"name": "OBJECTID", "type": "esriFieldTypeOID"}, {"name": "FINISHED", "type": "esriFieldTypeDate"}}, overlay},
	}
	js := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}

	mux := http.NewServeMux()
	const base = "/arcgis/rest/services/Mock/MapServer"
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, _ *http.Request) {
		js(w, map[string]any{"layers": []map[string]any{{"id": 75, "name": "MOCK_PERMITS"}, {"id": 13, "name": "MOCK_RESURFACING"}}})
	})
	mux.HandleFunc("GET "+base+"/{id}", func(w http.ResponseWriter, req *http.Request) {
		l, ok := layers[req.PathValue("id")]
		if !ok {
			js(w, map[string]any{"error": map[string]any{"code": 400, "message": "Invalid layer"}})
			return
		}
		js(w, map[string]any{"name": l.name, "geometryType": l.geom, "maxRecordCount": maxRecords, "fields": l.fields})
	})
	mux.HandleFunc("GET "+base+"/{id}/query", func(w http.ResponseWriter, req *http.Request) {
		l, ok := layers[req.PathValue("id")]
		q := req.URL.Query()
		if !ok {
			js(w, map[string]any{"error": map[string]any{"code": 400, "message": "Invalid layer"}})
			return
		}
		switch {
		case q.Get("returnCountOnly") == "true":
			js(w, map[string]int{"count": len(l.feats)})
		case q.Get("outStatistics") != "":
			var st []struct {
				Field string `json:"onStatisticField"`
			}
			json.Unmarshal([]byte(q.Get("outStatistics")), &st)
			var lo, hi *float64
			for _, f := range l.feats {
				if v, ok := f.Properties[st[0].Field].(float64); ok {
					if lo == nil || v < *lo {
						x := v
						lo = &x
					}
					if hi == nil || v > *hi {
						x := v
						hi = &x
					}
				}
			}
			js(w, map[string]any{"features": []map[string]any{{"attributes": map[string]any{"mn": lo, "mx": hi}}}})
		default:
			off, _ := strconv.Atoi(q.Get("resultOffset"))
			n, _ := strconv.Atoi(q.Get("resultRecordCount"))
			if n <= 0 || n > maxRecords {
				n = maxRecords
			}
			end := off + n
			if off > len(l.feats) {
				off = len(l.feats)
			}
			if end > len(l.feats) {
				end = len(l.feats)
			}
			js(w, map[string]any{"type": "FeatureCollection", "features": l.feats[off:end]})
		}
	})
	log.Printf("mock ArcGIS on http://%s%s (%d permit rows)", *addr, base, len(permits))
	log.Fatal(http.ListenAndServe(*addr, mux))
}
