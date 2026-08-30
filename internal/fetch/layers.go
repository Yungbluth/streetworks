package fetch

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	permitsService = "https://gis.tucsonaz.gov/arcgis/rest/services/PublicMaps/PermitsCode/MapServer"
	prop409Service = "https://gis.tucsonaz.gov/arcgis/rest/services/TDOT/Prop409Public_rev/MapServer"
	openData       = "https://publicgis.tucsonaz.gov/open/rest/services/OpenData/OpenData_Transportation/MapServer"
)

// Layer is one ArcGIS layer to download
type Layer struct {
	Name string
	URL  string

	Priority int

	// Fallbacks are tried only when URL's host can't be reached at all
	Fallbacks []string
}

func (l Layer) IsPermits() bool { return strings.HasPrefix(l.Name, "permits") }

func Defaults() []Layer {
	current := func(name string, id int) Layer {
		return Layer{Name: name, URL: fmt.Sprintf("%s/%d", permitsService, id), Priority: 2}
	}
	return []Layer{
		{Name: "permits_inactive", URL: permitsService + "/75", Priority: 1},
		current("permits_row_active", 133),
		current("permits_row_utility", 139),
		current("permits_row_general", 135),
		current("permits_row_cip_joc", 62),
		current("permits_row_development", 64),
		current("permits_row_homedev", 134),
		current("permits_row_residential", 136),
		{Name: "prop409_major", URL: prop409Service + "/813", Fallbacks: []string{openData + "/13"}},
		{Name: "prop409_residential", URL: prop409Service + "/814", Fallbacks: []string{openData + "/14"}},
	}
}

var validName = regexp.MustCompile(`^[a-z0-9_]+$`)

func Select(extra []string, only string) ([]Layer, error) {
	layers := Defaults()
	for _, e := range extra {
		name, u, ok := strings.Cut(e, "=")
		name, u = strings.TrimSpace(name), strings.TrimSpace(u)
		if !ok || name == "" || u == "" {
			return nil, fmt.Errorf("-extra wants name=url, got %q", e)
		}
		if !validName.MatchString(name) {
			return nil, fmt.Errorf("layer name %q must be lowercase letters, digits and underscores", name)
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return nil, fmt.Errorf("layer %q: url must start with http:// or https://, got %q", name, u)
		}
		i := indexOf(layers, name)
		if i < 0 {
			l := Layer{Name: name, URL: u}
			if l.IsPermits() {
				l.Priority = 2
			}
			layers = append(layers, l)
			continue
		}
		layers[i].URL, layers[i].Fallbacks = u, nil
	}
	if only == "" {
		return layers, nil
	}
	var out []Layer
	for _, name := range strings.Split(only, ",") {
		name = strings.TrimSpace(name)
		i := indexOf(layers, name)
		if i < 0 {
			return nil, fmt.Errorf("-only: unknown layer %q", name)
		}
		out = append(out, layers[i])
	}
	return out, nil
}

func indexOf(layers []Layer, name string) int {
	for i, l := range layers {
		if l.Name == name {
			return i
		}
	}
	return -1
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return u
}
