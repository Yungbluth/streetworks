// Command streetworks builds a static map of Tucson's right-of-way permits
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/yungbluth/streetworks/internal/arcgis"
	"github.com/yungbluth/streetworks/internal/fetch"
	"github.com/yungbluth/streetworks/internal/survey"
)

var errUsage = errors.New("usage: streetworks fetch|survey [flags]")

var newClient = arcgis.New

type listFlag []string

func (l *listFlag) String() string     { return fmt.Sprint(*l) }
func (l *listFlag) Set(s string) error { *l = append(*l, s); return nil }

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, errUsage)
		os.Exit(2)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		signal.Stop(sigs) // a second Ctrl+C now kills the process outright
		fmt.Fprintln(os.Stderr, "\nstopping... (press Ctrl+C again to force quit)")
		cancel()
	}()

	err := run(ctx, os.Args[1], os.Args[2:], os.Stdout)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if errors.Is(err, errUsage) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cmd string, args []string, w io.Writer) error {
	switch cmd {
	case "fetch":
		return fetchCmd(ctx, w, args)
	case "survey":
		return surveyCmd(ctx, w, args)
	}
	return fmt.Errorf("unknown command %q\n%w", cmd, errUsage)
}

func fetchCmd(ctx context.Context, w io.Writer, args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	out := fs.String("out", "data/raw", "where to save the layers")
	only := fs.String("only", "", "comma-separated layer names to fetch (default: all)")
	page := fs.Int("page", 2000, "records per request")
	var extra listFlag
	fs.Var(&extra, "extra", "name=url of another layer (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *page <= 0 {
		return fmt.Errorf("-page must be positive, got %d", *page)
	}
	layers, err := fetch.Select(extra, *only)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if failed := fetch.Run(ctx, w, newClient(), layers, *out, *page); failed > 0 {
		return fmt.Errorf("%d layer(s) failed; the others were saved", failed)
	}
	return nil
}

func surveyCmd(ctx context.Context, w io.Writer, args []string) error {
	fs := flag.NewFlagSet("survey", flag.ContinueOnError)
	service := fs.String("service", survey.DefaultService, "MapServer to examine")
	var extra listFlag
	fs.Var(&extra, "extra", "name=url of a layer you already fetch (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	have, err := fetch.Select(extra, "")
	if err != nil {
		return err
	}
	return survey.Run(ctx, w, newClient(), *service, have)
}
