package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/yungbluth/streetworks/internal/store"
)

const built = "2026-10-24T10:00:00Z"

type fakeStore struct {
	err       error
	gotIDs    []int
	gotLimit  int
	searchErr error
}

func (f *fakeStore) Meta(context.Context) (store.Meta, error) {
	return store.Meta{Built: built, Version: 2, Permits: 3}, f.err
}

func (f *fakeStore) Lookup(_ context.Context, ids []int) (map[int]store.Detail, error) {
	f.gotIDs = ids
	out := map[int]store.Detail{}
	for _, id := range ids {
		out[id] = store.Detail{Permit: "T" + strconv.Itoa(id)}
	}
	return out, f.err
}

func (f *fakeStore) ByNumber(_ context.Context, number string) (store.Permit, error) {
	if f.err != nil {
		return store.Permit{}, f.err
	}
	if number != "T23SY041246" {
		return store.Permit{}, store.ErrNotFound
	}
	return store.Permit{ID: 7, Place: 4, Detail: store.Detail{Permit: number}}, nil
}

func (f *fakeStore) Search(_ context.Context, q string, limit int) ([]store.Hit, error) {
	f.gotLimit = limit
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	if f.err != nil {
		return nil, f.err
	}
	return []store.Hit{{ID: 1, Number: "T1", Snippet: "<mark>curb</mark>"}}, nil
}

func do(t *testing.T, h http.Handler, method, target, body string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q isn't JSON: %v", rec.Body.String(), err)
	}
	return m
}

func newHandler(f *fakeStore) http.Handler {
	return New(f, built, Options{Origin: "https://streetworks.example"})
}

func TestHealthz(t *testing.T) {
	rec := do(t, newHandler(&fakeStore{}), "GET", "/healthz", "")
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content type %q", ct)
	}
	if rec.Header().Get("X-Streetworks-Built") != built {
		t.Error("missing built header")
	}
}

func TestMeta(t *testing.T) {
	rec := do(t, newHandler(&fakeStore{}), "GET", "/v1/meta", "")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	m := decode(t, rec)
	if m["built"] != built || rec.Header().Get("X-Streetworks-Built") != m["built"] {
		t.Errorf("built %v, header %q", m["built"], rec.Header().Get("X-Streetworks-Built"))
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("content type %q", ct)
	}
}

func TestLookup(t *testing.T) {
	f := &fakeStore{}
	rec := do(t, newHandler(f), "POST", "/v1/permits/lookup", `{"ids":[0,2,2]}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	m := decode(t, rec)
	permits := m["permits"].(map[string]any)
	if _, ok := permits["0"]; !ok || len(permits) != 2 {
		t.Errorf("permits %v", permits)
	}
	if _, ok := permits["2"]; !ok {
		t.Errorf("permits %v", permits)
	}
	if !reflect.DeepEqual(f.gotIDs, []int{0, 2}) {
		t.Errorf("store got %v", f.gotIDs)
	}
	if m["built"] != built {
		t.Errorf("built %v", m["built"])
	}
}

func TestLookup400(t *testing.T) {
	many := make([]string, 2001)
	for i := range many {
		many[i] = strconv.Itoa(i)
	}
	cases := []struct{ name, body, msg string }{
		{"not json", `nope`, ""},
		{"ids a string", `{"ids":"x"}`, ""},
		{"negative", `{"ids":[-1]}`, ""},
		{"fraction", `{"ids":[1.5]}`, ""},
		{"unknown field", `{"ids":[1],"x":2}`, ""},
		{"too many", `{"ids":[` + strings.Join(many, ",") + `]}`, "Ask for at most 2000 permits at a time."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newHandler(&fakeStore{}), "POST", "/v1/permits/lookup", c.body)
			if rec.Code != 400 {
				t.Fatalf("status %d", rec.Code)
			}
			msg, _ := decode(t, rec)["message"].(string)
			if msg == "" || (c.msg != "" && msg != c.msg) {
				t.Errorf("message %q", msg)
			}
		})
	}
}

func TestByNumber(t *testing.T) {
	h := newHandler(&fakeStore{})
	rec := do(t, h, "GET", "/v1/permits/T23SY041246", "")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	m := decode(t, rec)
	if m["built"] != built || m["id"] != float64(7) || m["place"] != float64(4) || m["permit"] == nil {
		t.Errorf("body %v", m)
	}

	rec = do(t, h, "GET", "/v1/permits/%20t00xx000000", "")
	if rec.Code != 404 {
		t.Fatalf("status %d", rec.Code)
	}
	if msg := decode(t, rec)["message"]; msg != "No permit T00XX000000 is on record." {
		t.Errorf("message %q", msg)
	}
}

func TestSearch(t *testing.T) {
	f := &fakeStore{}
	h := newHandler(f)
	rec := do(t, h, "GET", "/v1/search?q=curb", "")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if hits, _ := decode(t, rec)["hits"].([]any); len(hits) != 1 {
		t.Errorf("hits %v", hits)
	}
	if f.gotLimit != 20 {
		t.Errorf("default limit %d", f.gotLimit)
	}

	f.searchErr = errBad
	rec = do(t, h, "GET", "/v1/search?q=*", "")
	if rec.Code != 400 || decode(t, rec)["message"] != errBad.Error() {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
	f.searchErr = nil

	if rec = do(t, h, "GET", "/v1/search?q=curb&limit=abc", ""); rec.Code != 400 {
		t.Errorf("limit=abc gave %d", rec.Code)
	}
	if rec = do(t, h, "GET", "/v1/search?q=curb&limit=80", ""); rec.Code != 200 || f.gotLimit != 50 {
		t.Errorf("limit=80 gave %d, store got %d", rec.Code, f.gotLimit)
	}
}

var errBad = badQueryErr("Type at least 2 letters or digits.")

type badQueryErr string

func (e badQueryErr) Error() string        { return string(e) }
func (e badQueryErr) Is(target error) bool { return target == store.ErrBadQuery }
