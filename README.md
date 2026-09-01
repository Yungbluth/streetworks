# Streetworks

Why does the same road keep getting dug up? This maps every permit the City of Tucson has issued to dig up, close or work in a street, back to 1996.

The data is the city's open right-of-way permit records, about 93,000 permits with dates since 2005 (older ones are thin). Permits get put on hundred-blocks and intersections, so there's no geocoder and no API key anywhere.

## What's where

- `cmd/streetworks` is the Go command. `fetch` pulls the city's ArcGIS layers, `build` turns them into the JSON files the page loads, `preview` serves the result, `survey` lists the city's permit layers.
- `internal/` is the rest of the Go code: permit parsing, the ArcGIS client, the site writer.

## Running it

Needs Go.

```sh
go run ./cmd/streetworks fetch      # downloads to data/raw, takes a while
go run ./cmd/streetworks build      # writes site/data
go run ./cmd/streetworks preview
```

Preview serves `site/` on 127.0.0.1:8080.

Tests: `go test ./...`
