package main

import (
	"net/url"
	"regexp"
	"strings"

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
