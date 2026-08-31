package permits

import "strings"

type DedupeStats struct {
	Rows, Kept, Dropped int
	StatusDiffered      int // duplicates whose copies disagreed on status
	PerLayer            []LayerStat
}

// Dedupe keeps one record per permit number. If two layers have the same permit,
// the layer with the higher priority wins
func Dedupe(recs []Record, priority map[string]int) ([]Record, DedupeStats) {
	var st DedupeStats
	out := make([]Record, 0, len(recs))
	at := map[string]int{} // permit number -> index in out
	perLayer := map[string]*LayerStat{}
	var order []string // layers in the order they were first seen

	for _, r := range recs {
		st.Rows++
		ls := perLayer[r.Layer]
		if ls == nil {
			ls = &LayerStat{Layer: r.Layer}
			perLayer[r.Layer] = ls
			order = append(order, r.Layer)
		}
		ls.Rows++

		// same number in two layers is the same permit
		key := strings.ToUpper(strings.TrimSpace(r.Permit))
		i, dup := at[key]
		// no number means no way to match it, so it always stays
		if key == "" || !dup {
			if key != "" {
				at[key] = len(out)
			}
			out = append(out, r)
			ls.Kept++
			continue
		}

		kept := out[i]
		if !strings.EqualFold(strings.TrimSpace(kept.Status), strings.TrimSpace(r.Status)) {
			st.StatusDiffered++
		}
		if priority[r.Layer] > priority[kept.Layer] {
			out[i] = r
			ls.Kept++
			loser := perLayer[kept.Layer]
			loser.Kept--
			loser.Dupes++
		} else {
			ls.Dupes++
		}
	}

	for _, name := range order {
		ls := *perLayer[name]
		st.PerLayer = append(st.PerLayer, ls)
		st.Kept += ls.Kept
		st.Dropped += ls.Dupes
	}
	return out, st
}
