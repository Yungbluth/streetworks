package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rawSample = "../../internal/permits/testdata/raw"

func readIndex(t *testing.T, dir string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var idx map[string]any
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestBuildSample(t *testing.T) {
	out := filepath.Join(t.TempDir(), "data")
	var buf bytes.Buffer
	if err := run(context.Background(), "build", []string{"-raw", rawSample, "-out", out}, &buf); err != nil {
		t.Fatalf("%v\n%s", err, buf.String())
	}
	idx := readIndex(t, out)
	if q := idx["quality"].(map[string]any); q["unique_permits"] != 10.0 {
		t.Errorf("quality = %v", q)
	}
	srcs := idx["sources"].([]any)
	if len(srcs) != 1 || srcs[0].(map[string]any)["rows"] != 10.0 {
		t.Errorf("sources = %v", srcs)
	}
	for _, want := range []string{"Unique permits:       10", "10 permits on the map at 10 places", "first load", "wrote " + out} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output missing %q:\n%s", want, buf.String())
		}
	}
}

func TestBuildOverlays(t *testing.T) {
	raw := t.TempDir()
	sample, _ := os.ReadFile(filepath.Join(rawSample, "permits_inactive.jsonl"))
	os.WriteFile(filepath.Join(raw, "permits_inactive.jsonl"), sample, 0o644)
	line := `{"layer":"prop409_major","properties":{"STREET":"BROADWAY"},"geometry":{"type":"LineString","coordinates":[[-110.9,32.2],[-110.8,32.2]]}}` + "\n"
	os.WriteFile(filepath.Join(raw, "prop409_major.jsonl"), []byte(line), 0o644)

	out := filepath.Join(t.TempDir(), "data")
	if err := run(context.Background(), "build", []string{"-raw", raw, "-out", out}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	ovs := readIndex(t, out)["overlays"].([]any)
	if len(ovs) != 1 {
		t.Fatalf("overlays = %v", ovs)
	}
	o := ovs[0].(map[string]any)
	if o["name"] != "prop409_major" || !strings.Contains(o["label"].(string), "Prop 409") || o["rows"] != 1.0 {
		t.Errorf("overlay = %v", o)
	}
	if q := readIndex(t, out)["quality"].(map[string]any); q["unique_permits"] != 10.0 {
		t.Errorf("an overlay's rows were counted as permits: %v", q)
	}
}

func TestBuildGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":1,"quality":{"unique_permits":100}}`)
	}))
	defer srv.Close()
	out := filepath.Join(t.TempDir(), "data")
	err := run(context.Background(), "build", []string{"-raw", rawSample, "-out", out, "-guard", srv.URL + "/data/index.json"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "down from 100") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("the guard failed but output was written anyway")
	}
}

func TestBuildArguments(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"no raw data", []string{"-raw", t.TempDir()}, "run streetworks fetch first"},
		{"missing raw dir", []string{"-raw", filepath.Join(t.TempDir(), "nope")}, "run streetworks fetch first"},
		{"bad tiles", []string{"-raw", rawSample, "-out", t.TempDir(), "-tiles", "not-a-url"}, "tile URL"},
		{"unknown flag", []string{"-nope"}, "flag provided but not defined"},
	} {
		err := run(context.Background(), "build", c.args, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}
