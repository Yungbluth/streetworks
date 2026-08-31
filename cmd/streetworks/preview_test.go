package main

import (
	"context"

	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func listener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func TestRunServerCancel(t *testing.T) {
	ln := listener(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runServer(ctx, ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }), time.Second)
	}()
	url := "http://" + ln.Addr().String() + "/"
	if code, body := get(t, url); code != 200 || body != "ok" {
		t.Fatalf("%d %q", code, body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a requested stop is not an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop after the context was cancelled (this is the Ctrl+C bug)")
	}
	if _, err := http.Get(url); err == nil {
		t.Error("the port should no longer accept connections")
	}
}

func TestRunServerStuck(t *testing.T) {
	ln := listener(t)
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runServer(ctx, ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}), 150*time.Millisecond)
	}()
	go func() { _, _ = http.Get("http://" + ln.Addr().String() + "/") }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a stuck request kept the server from stopping past the grace period")
	}
}

func TestWatchForQuit(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  bool
	}{
		"q":                        {"q\n", true},
		"Q uppercase":              {"Q\n", true},
		"quit":                     {"quit\n", true},
		"exit with spaces":         {"  exit  \n", true},
		"stop":                     {"stop\n", true},
		"after other input":        {"hello\nwhat\nq\n", true},
		"windows line ending":      {"q\r\n", true},
		"no newline at end":        {"q", true},
		"other words do not stop":  {"quiet\nqq\nquite\n", false},
		"empty input":              {"", false},
		"just blank lines":         {"\n\n\n", false},
		"end of input is not stop": {"hello\n", false},
	} {
		stopped := false
		watchForQuit(strings.NewReader(tc.input), func() { stopped = true })
		if stopped != tc.want {
			t.Errorf("%s: stopped=%v, want %v", name, stopped, tc.want)
		}
	}
	n := 0
	watchForQuit(strings.NewReader("q\nq\nq\n"), func() { n++ })
	if n != 1 {
		t.Errorf("stop called %d times", n)
	}
}
