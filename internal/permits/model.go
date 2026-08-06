package permits

import (
	"regexp"
	"time"

)

// Record is one right-of-way permit, normalized from the city's layer.
type Record struct {
	Layer        string    `json:"layer"`
	Permit       string    `json:"permit"`
	Type         string    `json:"type"`
	SubType      string    `json:"sub_type,omitempty"`
	Category     string    `json:"category"`
	Status       string    `json:"status"`
	Description  string    `json:"description"`
	Applicant    string    `json:"applicant,omitempty"`
	Utility      string    `json:"utility,omitempty"`
	Location     string    `json:"location,omitempty"`
	RawStreet    string    `json:"raw_street"`
	Street       string    `json:"street"`
	Block        int       `json:"block"`
	StreetSource string    `json:"street_source,omitempty"`
	DateFlag     string    `json:"date_flag,omitempty"`
	NoWorkStatus bool      `json:"no_work,omitempty"` // set when records are served, from the status
	GeocodeNote  string    `json:"geocode_note,omitempty"`
	ProURL       string    `json:"pro_url,omitempty"`
	Date         string    `json:"date,omitempty"`
	DateBasis    string    `json:"date_basis,omitempty"`
	Final        string    `json:"final,omitempty"`
	Lat          float64   `json:"lat,omitempty"`
	Lng          float64   `json:"lng,omitempty"`
	HasGeo       bool      `json:"-"`
	When         time.Time `json:"-"`
}

var addrRE = regexp.MustCompile(`^(\d+)\s+([NSEW])\.?\s+(\S.*)$`)

func date(p map[string]any, k string) (time.Time, bool) {
	f, ok := p[k].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(f)).UTC(), true
}
