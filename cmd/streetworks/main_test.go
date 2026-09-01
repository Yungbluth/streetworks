package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
)

func TestMain(m *testing.M) {
	// The process-level tests re-run this test binary as the program itself
	if os.Getenv("STREETWORKS_RUN_MAIN") == "1" {
		os.Args = append([]string{"streetworks"}, strings.Split(os.Getenv("STREETWORKS_ARGS"), "\x1f")...)
		main()
		os.Exit(0)
	}
	newClient = func() *arcgis.Client {
		c := arcgis.New()
		c.Retries, c.Backoff = 1, time.Millisecond
		return c
	}
	os.Exit(m.Run())
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	err := run(context.Background(), "bogus", nil, &bytes.Buffer{})
	if !errors.Is(err, errUsage) || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("err = %v", err)
	}
}

func TestFetchArguments(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"zero page", []string{"-page", "0"}, "-page must be positive"},
		{"negative page", []string{"-page", "-5"}, "-page must be positive"},
		{"unknown flag", []string{"-nope"}, "flag provided but not defined"},
		{"help", []string{"-h"}, "help requested"},
		{"unknown layer", []string{"-only", "nope"}, "unknown layer"},
		{"bad extra", []string{"-extra", "oops"}, "name=url"},
		{"output path is a file", []string{"-out", file, "-only", "permits_inactive"}, "not a directory"},
	} {
		err := run(context.Background(), "fetch", c.args, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}

func TestSurveyArguments(t *testing.T) {
	if err := run(context.Background(), "survey", []string{"-extra", "bad"}, &bytes.Buffer{}); err == nil {
		t.Error("bad -extra should fail")
	}
	if err := run(context.Background(), "survey", []string{"-nope"}, &bytes.Buffer{}); err == nil {
		t.Error("unknown flag should fail")
	}
}
