package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/yungbluth/streetworks/internal/api"
	"github.com/yungbluth/streetworks/internal/store"
)

func serveCmd(ctx context.Context, w io.Writer, args []string) error {
	addr := "127.0.0.1:8081"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port // Cloud Run says where to listen
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dbPath := fs.String("db", "streetworks.db", "the database written by streetworks build")
	fs.StringVar(&addr, "addr", addr, "address to listen on")
	origin := fs.String("origin", os.Getenv("ORIGIN"), "the site allowed to call the API from a browser (default: any http://localhost page)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, h, err := openAPI(ctx, *dbPath, api.Options{Origin: *origin, Log: slog.New(slog.NewJSONHandler(os.Stderr, nil))})
	if err != nil {
		return err
	}
	defer db.Close()
	ln, err := listen(addr, "-addr")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "serving %s at http://%s\n", *dbPath, ln.Addr())
	return runServer(ctx, ln, h, 5*time.Second)
}

// openAPI opens the database and returns the API's handler over it
func openAPI(ctx context.Context, path string, o api.Options) (*store.DB, http.Handler, error) {
	db, err := store.Open(path)
	if err != nil {
		return nil, nil, err
	}
	m, err := db.Meta(ctx)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return db, api.New(db, m.Built, o), nil
}
