package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// watchForQuit calls stop when a line saying q, quit, exit or stop arrives on r
func watchForQuit(r io.Reader, stop func()) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		switch strings.ToLower(strings.TrimSpace(sc.Text())) {
		case "q", "quit", "exit", "stop":
			stop()
			return
		}
	}
}

func runServer(ctx context.Context, ln net.Listener, h http.Handler, grace time.Duration) error {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		srv.Close() // grace period over: drop whatever is still connected
	}
	<-errc
	return nil
}

func preview(ctx context.Context, stdin io.Reader, w io.Writer, args []string) error {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	dir := fs.String("dir", "site", "the built site to serve")
	addr := fs.String("addr", "127.0.0.1:8080", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(*dir, "data", "index.json")); err != nil {
		return fmt.Errorf("%s has no data/index.json; run streetworks build first", *dir)
	}
	if _, err := os.Stat(filepath.Join(*dir, "index.html")); err != nil {
		fmt.Fprintf(w, "note: %s has no index.html yet; build the page with npm run build in web/\n", *dir)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("can't listen on %s: %w\nIs another copy of streetworks still running? Stop it, or choose another address with -addr", *addr, err)
	}
	fmt.Fprintf(w, "serving %s at http://%s\n", *dir, ln.Addr())
	fmt.Fprintln(w, "press Ctrl+C to stop (or type q and press Enter)")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go watchForQuit(stdin, cancel)
	err = runServer(ctx, ln, http.FileServer(http.Dir(*dir)), 5*time.Second)
	fmt.Fprintln(w, "stopped")
	return err
}
