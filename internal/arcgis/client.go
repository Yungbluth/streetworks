package arcgis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	HTTP *http.Client
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 90 * time.Second}}
}

type Field struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Alias string `json:"alias"`
}

type LayerInfo struct {
	Name           string  `json:"name"`
	GeometryType   string  `json:"geometryType"`
	MaxRecordCount int     `json:"maxRecordCount"`
	Fields         []Field `json:"fields"`
}

type Feature struct {
	Properties map[string]any  `json:"properties"`
	Geometry   json.RawMessage `json:"geometry"`
}

// Stored is the on-disk JSONL form: a feature tagged with its source layer
type Stored struct {
	Layer string `json:"layer"`
	Feature
}

func (c *Client) get(ctx context.Context, base string, q url.Values) ([]byte, error) {
	u := base
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "streetworks-time-machine/0.1 (portfolio project)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %.200s", resp.StatusCode, body)
	}
	return body, nil
}

func (c *Client) Layer(ctx context.Context, layerURL string) (*LayerInfo, error) {
	body, err := c.get(ctx, layerURL, url.Values{"f": {"json"}})
	if err != nil {
		return nil, err
	}
	var info LayerInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("decode layer info: %w", err)
	}
	return &info, nil
}

func (c *Client) Count(ctx context.Context, layerURL string) (int, error) {
	body, err := c.get(ctx, layerURL+"/query", url.Values{"where": {"1=1"}, "returnCountOnly": {"true"}, "f": {"json"}})
	if err != nil {
		return 0, err
	}
	var p struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return 0, err
	}
	return p.Count, nil
}

// Fetch pages through every feature in a layer (WGS84 GeoJSON), calling fn per page
func (c *Client) Fetch(ctx context.Context, layerURL string, pageSize int, fn func([]Feature) error) (int, error) {
	info, err := c.Layer(ctx, layerURL)
	if err != nil {
		return 0, err
	}
	if info.MaxRecordCount > 0 && pageSize > info.MaxRecordCount {
		pageSize = info.MaxRecordCount
	}
	oid := ""
	for _, f := range info.Fields {
		if f.Type == "esriFieldTypeOID" {
			oid = f.Name
			break
		}
	}
	expected := -1
	if n, err := c.Count(ctx, layerURL); err == nil {
		expected = n
	}
	total := 0
	for {
		q := url.Values{
			"where": {"1=1"}, "outFields": {"*"}, "outSR": {"4326"}, "f": {"geojson"},
			"returnGeometry": {"true"}, "resultOffset": {strconv.Itoa(total)}, "resultRecordCount": {strconv.Itoa(pageSize)},
		}
		if oid != "" {
			q.Set("orderByFields", oid) // stable order so offset paging never skips rows
		}
		body, err := c.get(ctx, layerURL+"/query", q)
		if err != nil {
			return total, err
		}
		var p struct {
			Features []Feature `json:"features"`
		}
		if err := json.Unmarshal(body, &p); err != nil {
			return total, fmt.Errorf("decode page at offset %d: %w", total, err)
		}
		fmt.Println("page", total, len(p.Features))
		if len(p.Features) == 0 {
			return total, nil
		}
		if err := fn(p.Features); err != nil {
			return total, err
		}
		total += len(p.Features)
		if expected >= 0 && total >= expected {
			return total, nil
		}
	}
}
