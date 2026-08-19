package web

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

type TileSource struct {
	URL         string `json:"url"`
	Attribution string `json:"attribution"`
	MaxZoom     int    `json:"max_zoom"`
	Filter      string `json:"filter"`
}

type Basemap struct {
	Light TileSource `json:"light"`
	Dark  TileSource `json:"dark"`
}

// BasemapOptions are what the person running the program asked for
type BasemapOptions struct {
	CartoKey    string // a free key from carto.com/basemaps/apikey
	LightURL    string // a custom tile URL template, e.g. https://host/{z}/{x}/{y}.png
	DarkURL     string // optional, for dark mode; defaults to LightURL
	Attribution string // optional plain-text credit for a custom source
}

const (
	osmURL           = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
	osmAttribution   = `&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors`
	cartoAttribution = osmAttribution + ` &copy; <a href="https://carto.com/attributions">CARTO</a>`
)

// CARTO stamps "API KEY REQUIRED" on keyless tile requests (since this summer)
var cartoKeyRE = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,200}$`)

// DefaultBasemap is OpenStreetMap's standard tiles: no key, street names to the highest zoom
func DefaultBasemap() Basemap {
	return Basemap{
		Light: TileSource{URL: osmURL, Attribution: osmAttribution, MaxZoom: 19, Filter: "mute"},
		Dark:  TileSource{URL: osmURL, Attribution: osmAttribution, MaxZoom: 19, Filter: "invert"},
	}
}

func checkTemplate(u string) error {
	pu, err := url.Parse(u)
	if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" {
		return fmt.Errorf("tile URL %q must be an http or https address", u)
	}
	for _, p := range []string{"{z}", "{x}", "{y}"} {
		if !strings.Contains(u, p) {
			return fmt.Errorf("tile URL %q must contain %s", u, p)
		}
	}
	return nil
}

// NewBasemap picks the basemap: a custom URL wins, then a CARTO key, then the default
func NewBasemap(o BasemapOptions) (Basemap, error) {
	switch {
	case o.LightURL != "":
		if err := checkTemplate(o.LightURL); err != nil {
			return Basemap{}, err
		}
		dark := o.DarkURL
		if dark == "" {
			dark = o.LightURL
		} else if err := checkTemplate(dark); err != nil {
			return Basemap{}, err
		}
		credit := "Map tiles: custom source"
		if o.Attribution != "" {
			credit = html.EscapeString(o.Attribution) // operator text is shown as text, never as markup
		}
		return Basemap{
			Light: TileSource{URL: o.LightURL, Attribution: credit, MaxZoom: 19},
			Dark:  TileSource{URL: dark, Attribution: credit, MaxZoom: 19},
		}, nil
	case o.CartoKey != "":
		if !cartoKeyRE.MatchString(o.CartoKey) {
			return Basemap{}, fmt.Errorf("the CARTO key may only contain letters, digits and . _ ~ -")
		}
		tile := func(style string) string {
			return "https://basemaps.cartocdn.com/rastertiles/" + style + "/{z}/{x}/{y}.png?key=" + url.QueryEscape(o.CartoKey)
		}
		return Basemap{
			Light: TileSource{URL: tile("light_all"), Attribution: cartoAttribution, MaxZoom: 19},
			Dark:  TileSource{URL: tile("dark_all"), Attribution: cartoAttribution, MaxZoom: 19},
		}, nil
	}
	return DefaultBasemap(), nil
}

func (b Basemap) Describe() string {
	switch {
	case strings.Contains(b.Light.URL, "cartocdn.com"):
		return "CARTO basemap (using your key)"
	case b.Light.URL == osmURL:
		return "OpenStreetMap standard tiles (no key needed; fine for running this on your own computer)"
	}
	return "custom tiles from " + tileHost(b.Light.URL)
}

func tileHost(u string) string {
	if pu, err := url.Parse(u); err == nil && pu.Host != "" {
		return pu.Host
	}
	return u
}
