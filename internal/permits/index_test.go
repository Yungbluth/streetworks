package permits

import (
	"time"

)

func rec(permit, cat, status, street string, block int, year int) Record {
	r := Record{Permit: permit, Category: cat, Status: status, Street: street, Block: block,
		HasGeo: true, Lat: 32.2 + float64(block)/1e5, Lng: -110.9}
	if year > 0 {
		r.When = time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC)
		r.Date = r.When.Format("2006-01-02")
		r.DateBasis = "DATE_ISSUED"
	}
	return r
}
