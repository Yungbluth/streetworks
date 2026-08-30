package fetch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/yungbluth/streetworks/internal/arcgis"
)

// Run downloads every layer into dir, printing progress to w
func Run(ctx context.Context, w io.Writer, c *arcgis.Client, layers []Layer, dir string, page int) (failed int) {
	for _, l := range layers {
		fmt.Fprintf(w, "%s ...\n", l.Name)
		n, used, err := Download(ctx, w, c, l, dir, page)
		if err != nil {
			failed++
			fmt.Fprintf(w, "  FAILED after %d rows: %v\n", n, err)
			if arcgis.IsUnreachable(err) {
				host := hostOf(used)
				fmt.Fprintf(w, "  This is a network problem, not a data problem: this computer could not connect to %s.\n"+
					"  Check that https://%s opens in a browser, and whether a VPN, firewall or antivirus is blocking it.\n", host, host)
			}
			continue
		}
		fmt.Fprintf(w, "  %d rows saved\n", n)
		if want, err := c.Count(ctx, used); err == nil && want != n {
			fmt.Fprintf(w, "  WARNING: saved %d rows but the server reports %d. The layer may have changed during the download.\n", n, want)
		}
		if n == 0 {
			fmt.Fprintf(w, "  WARNING: this layer returned no rows. Check it with: streetworks survey\n")
		}
	}
	return failed
}

func Download(ctx context.Context, w io.Writer, c *arcgis.Client, l Layer, dir string, page int) (rows int, used string, err error) {
	// the main address first, then the mirrors
	urls := append([]string{l.URL}, l.Fallbacks...)
	for i, u := range urls {
		rows, err = save(ctx, c, l.Name, u, dir, page)
		if err == nil || !arcgis.IsUnreachable(err) || i == len(urls)-1 {
			return rows, u, err
		}
		fmt.Fprintf(w, "  could not reach %s; trying %s\n", hostOf(u), hostOf(urls[i+1]))
	}
	return 0, l.URL, nil
}

// save writes to a .tmp file and renames it at the end, so a download that fails
// halfway doesn't wipe out the old copy
func save(ctx context.Context, c *arcgis.Client, name, layerURL, dir string, page int) (int, error) {
	path := filepath.Join(dir, name+".jsonl")
	f, err := os.Create(path + ".tmp")
	if err != nil {
		return 0, err
	}
	buf := bufio.NewWriter(f)
	enc := json.NewEncoder(buf)
	n, err := c.Fetch(ctx, layerURL, page, func(features []arcgis.Feature) error {
		for _, ft := range features {
			if err := enc.Encode(arcgis.Stored{Layer: name, Feature: ft}); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		err = buf.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path + ".tmp")
		return n, err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return n, err
	}
	meta, _ := json.Marshal(arcgis.SourceMeta{Name: name, URL: layerURL, FetchedAt: time.Now().UTC(), Count: n})
	return n, os.WriteFile(filepath.Join(dir, name+".meta.json"), meta, 0o644)
}
