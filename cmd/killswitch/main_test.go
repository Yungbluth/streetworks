package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
		Log:         slog.New(slog.NewJSONHandler(f.logs, nil)),
	}
	return f
}
