package main

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakes struct {
	mu          sync.Mutex
	calls       []string
	tokenHeader string
	putAuth     string
	putBody     string
	enabled     bool
	putStatus   int
	cfg         config
	logs        *bytes.Buffer
}

func newFakes(t *testing.T) *fakes {
	t.Helper()
	f := &fakes{enabled: true, putStatus: http.StatusOK, logs: &bytes.Buffer{}}

	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		f.tokenHeader = r.Header.Get("Metadata-Flavor")
		io.WriteString(w, `{"access_token":"tok-123","expires_in":3599,"token_type":"Bearer"}`)
	}))
	t.Cleanup(meta.Close)

	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			if f.enabled {
				io.WriteString(w, `{"billingEnabled":true,"billingAccountName":"billingAccounts/000"}`)
			} else {
				io.WriteString(w, `{"billingEnabled":false}`)
			}
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			f.putBody = string(b)
			f.putAuth = r.Header.Get("Authorization")
			w.WriteHeader(f.putStatus)
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(billing.Close)

	f.cfg = config{
		Project:     "my-proj",
		MetadataURL: meta.URL,
		BillingURL:  billing.URL,
		Client:      http.DefaultClient,
		Log:         newLogger(f.logs),
	}
	return f
}

func push(t *testing.T, cfg config, data string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"message":{"data":"` + data + `","messageId":"1"},"subscription":"s"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler(cfg).ServeHTTP(rec, req)
	return rec
}

func encode(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestDetachesOnCost(t *testing.T) {
	f := newFakes(t)
	rec := push(t, f.cfg, encode(`{"costAmount":0.01,"budgetAmount":1,"currencyCode":"USD"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	want := []string{
		"GET /computeMetadata/v1/instance/service-accounts/default/token",
		"GET /v1/projects/my-proj/billingInfo",
		"PUT /v1/projects/my-proj/billingInfo",
	}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
	if f.tokenHeader != "Google" {
		t.Errorf("Metadata-Flavor = %q, want Google", f.tokenHeader)
	}
	if f.putAuth != "Bearer tok-123" {
		t.Errorf("Authorization = %q", f.putAuth)
	}
	if strings.TrimSpace(f.putBody) != `{"billingAccountName":""}` {
		t.Errorf("PUT body = %q", f.putBody)
	}
}

func TestIgnoresZeroCost(t *testing.T) {
	f := newFakes(t)
	rec := push(t, f.cfg, encode(`{"costAmount":0,"budgetAmount":1,"currencyCode":"USD"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(f.calls) != 0 {
		t.Fatalf("calls = %v, want none", f.calls)
	}
	if !strings.Contains(f.logs.String(), "no cost reported") {
		t.Errorf("log %q does not say no cost reported", f.logs.String())
	}
}

func TestGarbage(t *testing.T) {
	cases := map[string]string{
		"bad base64":   "!!!not-base64!!!",
		"bad json":     encode(`{"costAmount":`),
		"missing cost": encode(`{"budgetAmount":1,"currencyCode":"USD"}`),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakes(t)
			rec := push(t, f.cfg, data)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", rec.Code)
			}
			if len(f.calls) != 0 {
				t.Fatalf("calls = %v, want none", f.calls)
			}
			if !strings.Contains(f.logs.String(), `"severity":"WARN"`) {
				t.Errorf("no warning logged: %q", f.logs.String())
			}
		})
	}
}

func TestAlreadyDetached(t *testing.T) {
	f := newFakes(t)
	f.enabled = false
	rec := push(t, f.cfg, encode(`{"costAmount":5,"budgetAmount":1,"currencyCode":"USD"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "PUT") {
			t.Fatalf("made a PUT although billing was already off: %v", f.calls)
		}
	}
}

func TestBillingFailureRetries(t *testing.T) {
	f := newFakes(t)
	f.putStatus = http.StatusInternalServerError
	rec := push(t, f.cfg, encode(`{"costAmount":0.01}`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
