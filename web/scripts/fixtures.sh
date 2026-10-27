#!/bin/sh
set -e
cd "$(dirname "$0")/.."
rm -rf src/fixtures/data
go run ../cmd/streetworks build -raw ../internal/permits/testdata/raw -out src/fixtures/data -db src/fixtures/streetworks.db
