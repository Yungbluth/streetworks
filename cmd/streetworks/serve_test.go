package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func waitFor(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

var listenRE = regexp.MustCompile(`listening on (http://[0-9.:\[\]a-f]+)`)

func writeTestData(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	line := `{"layer":"permits_t","properties":{"PERMIT":"T1","PERMIT_TYPE":"EXCAVATION","PERMIT_STATUS":"CLOSED","STREET_NO":"100","STREET_DIR":"E","STREET_NAME":"MAIN ST","DATE_ISSUED":1158883200000},"geometry":{"type":"Point","coordinates":[-110.9,32.2]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "permits_t.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

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

func TestServeStartsAndStopsInProcess(t *testing.T) {
	buf := &lockedBuf{}
	log.SetOutput(buf)
	defer log.SetOutput(os.Stderr)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, strings.NewReader(""), []string{"-data", writeTestData(t), "-addr", "127.0.0.1:0"})
	}()
	waitFor(t, "the server to listen", 5*time.Second, func() bool { return listenRE.MatchString(buf.String()) })
	base := listenRE.FindStringSubmatch(buf.String())[1]
	if code, body := get(t, base+"/api/config"); code != 200 || !strings.Contains(body, "tile.openstreetmap.org") || strings.Contains(body, "cartocdn") {
		t.Errorf("default basemap must be keyless OpenStreetMap: %d %s", code, body)
	}
	if !strings.Contains(buf.String(), "basemap: OpenStreetMap") || !strings.Contains(buf.String(), "press Ctrl+C") {
		t.Errorf("startup log should say what basemap is used and how to stop:\n%s", buf.String())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after cancellation")
	}
	if !strings.Contains(buf.String(), "stopped") {
		t.Errorf("log: %s", buf.String())
	}
}

func TestServeStopsWhenQIsTyped(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	r, w := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- serve(context.Background(), r, []string{"-data", writeTestData(t), "-addr", "127.0.0.1:0"})
	}()
	time.Sleep(300 * time.Millisecond)
	fmt.Fprintln(w, "q")
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("typing q did not stop the server")
	}
}

func TestServeBasemapOptions(t *testing.T) {
	defer log.SetOutput(os.Stderr)
	run := func(args ...string) (string, error) {
		buf := &lockedBuf{}
		log.SetOutput(buf)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- serve(ctx, strings.NewReader(""), append([]string{"-data", writeTestData(t), "-addr", "127.0.0.1:0"}, args...))
		}()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case err := <-done:
				return "", err
			default:
			}
			if m := listenRE.FindStringSubmatch(buf.String()); m != nil {
				_, body := get(t, m[1]+"/api/config")
				cancel()
				<-done
				return body, nil
			}
			time.Sleep(20 * time.Millisecond)
		}
		return "", errors.New("timed out")
	}
	if body, err := run("-carto-key", "abc123"); err != nil || !strings.Contains(body, "light_all") || !strings.Contains(body, "key=abc123") || !strings.Contains(body, "dark_all") {
		t.Errorf("CARTO key: %v %s", err, body)
	}
	if body, err := run("-tiles", "https://tiles.example.test/{z}/{x}/{y}.png", "-attribution", "Example <b>Maps</b>"); err != nil ||
		!strings.Contains(body, "tiles.example.test") || strings.Contains(body, "<b>") {
		t.Errorf("custom tiles (attribution must be escaped): %v %s", err, body)
	}
	if body, err := run("-carto-key", "abc", "-tiles", "https://tiles.example.test/{z}/{x}/{y}.png"); err != nil || strings.Contains(body, "cartocdn") {
		t.Errorf("explicit tiles must beat a CARTO key: %v %s", err, body)
	}
	for _, bad := range [][]string{{"-tiles", "not-a-url"}, {"-tiles", "https://x.test/tiles.png"}, {"-tiles", "javascript:alert(1)"}, {"-carto-key", "a&b=c"}, {"-tiles", "https://x/{z}/{x}/{y}", "-tiles-dark", "ftp://x/{z}/{x}/{y}"}} {
		if _, err := run(bad...); err == nil {
			t.Errorf("%v should be rejected before the server starts", bad)
		}
	}
}

func TestServeExplainsAPortAlreadyInUse(t *testing.T) {
	ln := listener(t)
	defer ln.Close()
	err := serve(context.Background(), strings.NewReader(""), []string{"-data", writeTestData(t), "-addr", ln.Addr().String()})
	if err == nil || !strings.Contains(err.Error(), "another copy of streetworks") || !strings.Contains(err.Error(), "-addr") {
		t.Errorf("err = %v", err)
	}
}

func TestServeErrors(t *testing.T) {
	if err := serve(context.Background(), strings.NewReader(""), []string{"-data", t.TempDir()}); err == nil || !strings.Contains(err.Error(), "run ingest first") {
		t.Errorf("no data: %v", err)
	}
	if err := serve(context.Background(), strings.NewReader(""), []string{"-nope"}); err == nil {
		t.Error("unknown flag")
	}
	if err := serve(context.Background(), strings.NewReader(""), []string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: %v", err)
	}
}

// --- the real program, as a separate process -----------------------------------

type proc struct {
	cmd  *exec.Cmd
	out  *lockedBuf
	done chan error
}

// startProgram runs the program in its own process. With pipe set, the returned writer is its stdin.
func startProgram(t *testing.T, pipe bool, args ...string) (*proc, io.WriteCloser) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "STREETWORKS_RUN_MAIN=1", "STREETWORKS_ARGS="+strings.Join(args, "\x1f"))
	out := &lockedBuf{}
	cmd.Stdout, cmd.Stderr = out, out
	var w io.WriteCloser
	if pipe {
		var err error
		if w, err = cmd.StdinPipe(); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &proc{cmd: cmd, out: out, done: make(chan error, 1)}
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return p, w
}

func (p *proc) baseURL(t *testing.T) string {
	t.Helper()
	waitFor(t, "the program to start listening", 15*time.Second, func() bool { return listenRE.MatchString(p.out.String()) })
	return listenRE.FindStringSubmatch(p.out.String())[1]
}

func (p *proc) mustExitWithin(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case err := <-p.done:
		if err != nil {
			t.Fatalf("exited with %v\n%s", err, p.out.String())
		}
	case <-time.After(d):
		t.Fatalf("the program was still running %v after being asked to stop\n%s", d, p.out.String())
	}
}

func TestProgramReportsABusyPortAndExitsNonZero(t *testing.T) {
	ln := listener(t)
	defer ln.Close()
	p, _ := startProgram(t, false, "serve", "-data", writeTestData(t), "-addr", ln.Addr().String())
	select {
	case err := <-p.done:
		if err == nil || !strings.Contains(p.out.String(), "another copy of streetworks") {
			t.Errorf("err=%v\n%s", err, p.out.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("should have failed quickly")
	}
}
