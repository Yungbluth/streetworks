package site

import (
	"net/url"
	"strings"
	"testing"
)

func TestDefaultBasemapNeedsNoKey(t *testing.T) {
	b, err := NewBasemap(BasemapOptions{})
	if err != nil || b != DefaultBasemap() {
		t.Fatalf("%+v %v", b, err)
	}
	for name, src := range map[string]TileSource{"light": b.Light, "dark": b.Dark} {
		if strings.Contains(src.URL, "cartocdn") {
			t.Errorf("%s: the keyless default must not use CARTO: %s", name, src.URL)
		}
		if u, err := url.Parse(src.URL); err != nil || u.Host != "tile.openstreetmap.org" || u.Scheme != "https" || u.RawQuery != "" {
			t.Errorf("%s: %s", name, src.URL)
		}
		if !strings.Contains(src.Attribution, "openstreetmap.org/copyright") {
			t.Errorf("%s: OpenStreetMap must be credited: %s", name, src.Attribution)
		}
		if src.MaxZoom < 18 {
			t.Errorf("%s: max zoom %d is too low for block-level maps", name, src.MaxZoom)
		}
	}
	if b.Light.Filter != "mute" || b.Dark.Filter != "invert" {
		t.Errorf("filters: %q %q", b.Light.Filter, b.Dark.Filter)
	}
	if !strings.Contains(b.Describe(), "OpenStreetMap") {
		t.Error(b.Describe())
	}
}

func TestCartoKeyBasemap(t *testing.T) {
	b, err := NewBasemap(BasemapOptions{CartoKey: "AbC-123_x.y~z"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Light.URL != "https://basemaps.cartocdn.com/rastertiles/light_all/{z}/{x}/{y}.png?key=AbC-123_x.y~z" ||
		b.Dark.URL != "https://basemaps.cartocdn.com/rastertiles/dark_all/{z}/{x}/{y}.png?key=AbC-123_x.y~z" {
		t.Errorf("%s | %s", b.Light.URL, b.Dark.URL)
	}
	if b.Light.Filter != "" || b.Dark.Filter != "" {
		t.Error("real light/dark tiles need no filter")
	}
	for _, want := range []string{"openstreetmap.org/copyright", "carto.com/attributions"} {
		if !strings.Contains(b.Light.Attribution, want) {
			t.Errorf("attribution missing %s: %s", want, b.Light.Attribution)
		}
	}
	if !strings.Contains(b.Describe(), "CARTO") {
		t.Error(b.Describe())
	}
	for _, bad := range []string{"a&b=c", "key with space", "a/b", "<script>", "a?b", "é", strings.Repeat("a", 201), "a\nb", "%00"} {
		if _, err := NewBasemap(BasemapOptions{CartoKey: bad}); err == nil {
			t.Errorf("key %q should be rejected", bad)
		}
	}
}

func TestCustomTiles(t *testing.T) {
	tpl := "https://tiles.example.test/{z}/{x}/{y}.png"
	b, err := NewBasemap(BasemapOptions{LightURL: tpl, Attribution: "Maps by <b>Example</b> & co"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Light.URL != tpl || b.Dark.URL != tpl {
		t.Errorf("dark should default to the same URL: %+v", b)
	}
	if strings.Contains(b.Light.Attribution, "<b>") || !strings.Contains(b.Light.Attribution, "&lt;b&gt;Example&lt;/b&gt; &amp; co") {
		t.Errorf("operator text must be escaped: %s", b.Light.Attribution)
	}
	if !strings.Contains(b.Describe(), "tiles.example.test") {
		t.Error(b.Describe())
	}
	dark := "http://localhost:9000/{z}/{x}/{y}.png"
	if b, err = NewBasemap(BasemapOptions{LightURL: tpl, DarkURL: dark}); err != nil || b.Dark.URL != dark || b.Light.URL != tpl {
		t.Errorf("%+v %v", b, err)
	}
	if b, _ = NewBasemap(BasemapOptions{LightURL: tpl}); b.Light.Attribution == "" {
		t.Error("a placeholder credit is required so the source is never shown uncredited")
	}
	if b, _ = NewBasemap(BasemapOptions{LightURL: tpl, CartoKey: "abc"}); strings.Contains(b.Light.URL, "cartocdn") {
		t.Error("explicit tiles should win over a CARTO key")
	}
	for _, bad := range []string{"not a url", "ftp://x/{z}/{x}/{y}", "javascript:alert(1)//{z}{x}{y}", "https://x/{z}/{x}", "https://x/{x}/{y}", "https:///{z}/{x}/{y}", "//x/{z}/{x}/{y}", "/{z}/{x}/{y}.png", "data:text/plain,{z}{x}{y}"} {
		if _, err := NewBasemap(BasemapOptions{LightURL: bad}); err == nil {
			t.Errorf("tile URL %q should be rejected", bad)
		}
		if _, err := NewBasemap(BasemapOptions{LightURL: tpl, DarkURL: bad}); err == nil {
			t.Errorf("dark tile URL %q should be rejected", bad)
		}
	}
}
