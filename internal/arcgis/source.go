package arcgis

import "time"

type SourceMeta struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	FetchedAt time.Time `json:"fetched_at"`
	Count     int       `json:"count"`
}
