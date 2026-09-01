package site

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CheckGuard refuses a build that has lost a lot of permits. If the city's server gives back half
// the data on a bad day I don't want that published
func CheckGuard(ctx context.Context, c *http.Client, publishedIndexURL string, newCount int) error {
	if publishedIndexURL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, publishedIndexURL, nil)
	if err != nil {
		return fmt.Errorf("guard: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("guard: can't read the published index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("guard: %s returned %s", publishedIndexURL, resp.Status)
	}

	var idx struct {
		Quality struct {
			UniquePermits *int `json:"unique_permits"`
		} `json:"quality"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&idx); err != nil {
		return fmt.Errorf("guard: the published index isn't valid JSON: %w", err)
	}
	if idx.Quality.UniquePermits == nil {
		return fmt.Errorf("guard: the published index has no permit count")
	}
	old := *idx.Quality.UniquePermits
	// integer math, no rounding: new < 90% of old
	if newCount*10 < old*9 {
		return fmt.Errorf("guard: the new build has %d permits, down from %d published; that's more than a 10%% drop, so nothing was written", newCount, old)
	}
	return nil
}
