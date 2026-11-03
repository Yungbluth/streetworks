# Streetworks

Why does the same road keep getting dug up? This maps every permit the City of Tucson has issued to dig up, close or work in a street, back to 1996. Look up one block and read its history, or rewind the whole city a year at a time.

The data is the city's open right-of-way permit records, about 93,000 permits with dates since 2005 (older ones are thin). Permits get put on hundred-blocks and intersections, so there's no geocoder and no API key anywhere.

Map colors are the APWA utility color code, the paint crews spray on the street before they dig.

## What's where

- `cmd/streetworks` is the Go command. `fetch` pulls the city's ArcGIS layers, `build` turns them into the page's JSON files plus a SQLite database, `serve` is the API, `preview` runs the site and API together, `survey` lists the city's permit layers.
- `cmd/killswitch` detaches the Google Cloud billing account if a budget alert shows any cost. See `docs/deploy.md`.
- `internal/` is the rest of the Go code: permit parsing, the ArcGIS client, the SQLite store, the HTTP API.
- `web/` is the page. TypeScript, Vite and Leaflet, no framework. The map, filters and address search run in the browser. Full permit records and permit text search come from the API.

## Running it

Needs Go and Node 22.

```sh
go run ./cmd/streetworks fetch      # downloads to data/raw, takes a while
go run ./cmd/streetworks build      # writes site/data and streetworks.db
cd web && npm ci && npm run build   # builds the page into ../site
cd .. && go run ./cmd/streetworks preview
```

Preview serves the site on 127.0.0.1:8080 and the API on 8081. When working on the page, `npm run dev` in `web/` reads data from `site/data`.

Tests:

```sh
go test ./...
cd web && npm test && npx playwright test
```

`web/scripts/fixtures.sh` rebuilds the test fixtures from the ten permit rows in `internal/permits/testdata`. Run it if you change the data format.

## Deploys

A GitHub Action rebuilds everything on Mondays and deploys the page to GitHub Pages and the API to Cloud Run. If the new build has more than 10% fewer permits than the live site it stops instead of publishing. Google Cloud setup, budget and kill switch are in `docs/deploy.md`.

## Known gaps

- Records before 2005 are thin, and the page says so.
- Search only knows streets with at least one permit on them.
- Some permits have no date, so they drop out as soon as you pick a range of years.
- The API's rate limits are in memory, so they only work with a single Cloud Run instance.

## todo

- search only knows streets that have a permit, would be nice to have a proper street list
- undated permits vanish when you pick years
- the permit lookups could be cached in the browser
