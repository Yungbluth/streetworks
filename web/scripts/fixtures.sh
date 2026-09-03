#!/bin/sh
# rebuilds the test fixtures from the ten real permit rows in the Go testdata
set -e
cd "$(dirname "$0")/.."
go run ../cmd/streetworks build -raw ../internal/permits/testdata/raw -out src/fixtures/data
