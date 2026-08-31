package permits

import "testing"

var priority = map[string]int{"permits_inactive": 1, "permits_row_general": 2, "permits_row_active": 2}

func copyOf(permit, layer, status string) Record {
	return Record{Permit: permit, Layer: layer, Status: status}
}

func TestCurrentLayerCopyWins(t *testing.T) {
	out, st := Dedupe([]Record{
		copyOf("T1", "permits_inactive", "CLOSED"),
		copyOf("T1", "permits_row_general", "Complete"),
	}, priority)
	if len(out) != 1 || out[0].Status != "Complete" || out[0].Layer != "permits_row_general" {
		t.Fatalf("kept %+v", out)
	}
	if st.StatusDiffered != 1 || st.Dropped != 1 || st.Kept != 1 || st.Rows != 2 {
		t.Errorf("stats %+v", st)
	}
}

func TestNoPermitNumberKept(t *testing.T) {
	out, _ := Dedupe([]Record{copyOf("", "permits_inactive", "CLOSED"), copyOf("  ", "permits_inactive", "CLOSED")}, priority)
	if len(out) != 2 {
		t.Errorf("kept %d", len(out))
	}
}

func TestPerLayerStatsAddUp(t *testing.T) {
	in := []Record{
		copyOf("A", "permits_inactive", "CLOSED"),
		copyOf("B", "permits_inactive", "CLOSED"),
		copyOf("B", "permits_inactive", "CLOSED"), // repeat inside one layer
		copyOf("A", "permits_row_general", "Complete"),
		copyOf("C", "permits_row_general", "Complete"),
	}
	out, st := Dedupe(in, priority)
	if len(out) != 3 || st.Rows != 5 || st.Kept != 3 || st.Dropped != 2 {
		t.Fatalf("out=%d stats=%+v", len(out), st)
	}
	want := []LayerStat{{"permits_inactive", 3, 1, 2}, {"permits_row_general", 2, 2, 0}}
	if len(st.PerLayer) != 2 || st.PerLayer[0] != want[0] || st.PerLayer[1] != want[1] {
		t.Errorf("per layer = %+v, want %+v", st.PerLayer, want)
	}
}

func TestDedupeOrder(t *testing.T) {
	out, _ := Dedupe([]Record{
		copyOf("A", "permits_inactive", "x"), copyOf("B", "permits_inactive", "x"), copyOf("A", "permits_row_general", "y"),
	}, priority)
	if out[0].Permit != "A" || out[1].Permit != "B" || out[0].Status != "y" {
		t.Errorf("%+v", out)
	}
}
