package arcgis

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func fastClient() *Client {
	c := New()
	c.Backoff = time.Millisecond
	return c
}

func TestFetchPagesAndOrders(t *testing.T) {
	const total = 1234
	var sawOrder atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("f") == "json" && r.URL.Path == "/L/0" {
			fmt.Fprint(w, `{"name":"t","maxRecordCount":500,"fields":[{"name":"OBJECTID","type":"esriFieldTypeOID"}]}`)
			return
		}
		if q.Get("orderByFields") == "OBJECTID" {
			sawOrder.Store(true)
		}
		if q.Get("returnCountOnly") == "true" {
			fmt.Fprintf(w, `{"count":%d}`, total)
			return
		}
		off, _ := strconv.Atoi(q.Get("resultOffset"))
		n, _ := strconv.Atoi(q.Get("resultRecordCount"))
		if n > 500 {
			t.Errorf("page size %d exceeds server max", n)
		}
		fmt.Fprint(w, `{"type":"FeatureCollection","features":[`)
		for i := off; i < off+n && i < total; i++ {
			if i > off {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"type":"Feature","properties":{"OBJECTID":%d},"geometry":null}`, i)
		}
		fmt.Fprint(w, `]}`)
	}))
	defer srv.Close()
	got := 0
	n, err := fastClient().Fetch(context.Background(), srv.URL+"/L/0", 2000, func(fs []Feature) error { got += len(fs); return nil })
	if err != nil || n != total || got != total {
		t.Fatalf("n=%d got=%d err=%v", n, got, err)
	}
	if !sawOrder.Load() {
		t.Error("expected orderByFields=OBJECTID")
	}
}

func TestRetriesOnServerErrorAndBodyError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(503)
		case 2:
			fmt.Fprint(w, `{"error":{"code":500,"message":"Error performing query operation"}}`)
		default:
			fmt.Fprint(w, `{"count":42}`)
		}
	}))
	defer srv.Close()
	n, err := fastClient().Count(context.Background(), srv.URL)
	if err != nil || n != 42 || calls.Load() != 3 {
		t.Fatalf("n=%d calls=%d err=%v", n, calls.Load(), err)
	}
}

func TestClientErrorIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"error":{"code":400,"message":"Invalid or missing input parameters"}}`)
	}))
	defer srv.Close()
	if _, err := fastClient().Count(context.Background(), srv.URL); err == nil || calls.Load() != 1 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}
