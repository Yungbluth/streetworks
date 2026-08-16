package arcgis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	HTTP    *http.Client
	Retries int
	Backoff time.Duration
}

func New() *Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	// Fail fast on a host we cannot reach instead of waiting out the OS default
	tr.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 15 * time.Second
	return &Client{HTTP: &http.Client{Timeout: 90 * time.Second, Transport: tr}, Retries: 4, Backoff: time.Second}
}

func IsUnreachable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return true
	}
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return true
	}
	return strings.Contains(err.Error(), "TLS handshake timeout")
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

type LayerRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
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

type apiError struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details"`
}

func (e *apiError) Error() string {
	return fmt.Sprintf("arcgis error %d: %s %s", e.Code, e.Message, strings.Join(e.Details, "; "))
}

// get fetches a url and retries a few times on network errors, 429 and 5xx.
// anything else (a 404, say) fails straight away
func (c *Client) get(ctx context.Context, base string, q url.Values) ([]byte, error) {
	u := base
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var last error
	tries := 0
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.Backoff << (attempt - 1)):
			}
		}
		tries++
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "streetworks-time-machine/0.1 (portfolio project)")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			last = err
			if attempt >= 1 && IsUnreachable(err) {
				break // a host that cannot be reached twice will not answer on the fifth try
			}
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			last = fmt.Errorf("http %d", resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("http %d: %.200s", resp.StatusCode, body)
		}
		// arcgis sends errors back as a normal 200, so look for one near the start of the body
		head := body
		if len(head) > 64 {
			head = head[:64]
		}
		if bytes.Contains(head, []byte(`"error"`)) {
			var probe struct {
				Error *apiError `json:"error"`
			}
			if json.Unmarshal(body, &probe) == nil && probe.Error != nil {
				if probe.Error.Code >= 500 {
					last = probe.Error
					continue
				}
				return nil, probe.Error
			}
		}
		return body, nil
	}
	if IsUnreachable(last) {
		host := u
		if pu, err := url.Parse(u); err == nil && pu.Host != "" {
			host = pu.Host
		}
		return nil, fmt.Errorf("giving up after %d attempts: cannot connect to %s (check the network, firewall, VPN or proxy; other sites working does not mean this one is reachable): %w", tries, host, last)
	}
	return nil, fmt.Errorf("giving up after %d attempts: %w", tries, last)
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

func (c *Client) ServiceLayers(ctx context.Context, serviceURL string) ([]LayerRef, error) {
	body, err := c.get(ctx, serviceURL, url.Values{"f": {"json"}})
	if err != nil {
		return nil, err
	}
	var p struct {
		Layers []LayerRef `json:"layers"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return p.Layers, nil
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

// DateRange returns the min and max of a date field.
func (c *Client) DateRange(ctx context.Context, layerURL, field string) (lo, hi time.Time, err error) {
	return c.DateRangeWhere(ctx, layerURL, field, "1=1")
}

func (c *Client) DateRangeWhere(ctx context.Context, layerURL, field, where string) (lo, hi time.Time, err error) {
	stats := fmt.Sprintf(`[{"statisticType":"min","onStatisticField":%q,"outStatisticFieldName":"mn"},{"statisticType":"max","onStatisticField":%q,"outStatisticFieldName":"mx"}]`, field, field)
	body, err := c.get(ctx, layerURL+"/query", url.Values{"where": {where}, "outStatistics": {stats}, "f": {"json"}})
	if err != nil {
		return
	}
	var p struct {
		Features []struct {
			Attributes struct{ Mn, Mx *float64 } `json:"attributes"`
		} `json:"features"`
	}
	if err = json.Unmarshal(body, &p); err != nil || len(p.Features) == 0 {
		return
	}
	a := p.Features[0].Attributes
	if a.Mn != nil {
		lo = time.UnixMilli(int64(*a.Mn)).UTC()
	}
	if a.Mx != nil {
		hi = time.UnixMilli(int64(*a.Mx)).UTC()
	}
	return
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
