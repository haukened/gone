package store

import "testing"

// guardDeleted runs deleteUntouched for each id and returns those deleted.
//
// Parameters:
//   - g: guard under test.
//   - ids: blob ids with no index row.
//
// Returns: the ids passed to the deletion function.
func guardDeleted(g *blobGuard, ids ...string) []string {
	var deleted []string
	for _, id := range ids {
		g.deleteUntouched(id, func(id string) error {
			deleted = append(deleted, id)
			return nil
		})
	}
	return deleted
}

func TestBlobGuardSkipsHeldAndTouchedIDs(t *testing.T) {
	var g blobGuard
	releaseHeld := g.hold("held")
	g.beginScan()
	releaseTouched := g.hold("touched")
	releaseTouched()
	got := guardDeleted(&g, "held", "touched", "orphan")
	g.endScan()
	releaseHeld()
	if len(got) != 1 || got[0] != "orphan" {
		t.Fatalf("deleted = %v, want [orphan]", got)
	}
	if len(g.held) != 0 || g.touched != nil || g.scans != 0 {
		t.Fatalf("guard not reset: held=%v touched=%v scans=%d", g.held, g.touched, g.scans)
	}
}

func TestBlobGuardCountsRepeatedHolds(t *testing.T) {
	var g blobGuard
	first, second := g.hold("id"), g.hold("id")
	first()
	g.beginScan()
	if got := guardDeleted(&g, "id"); len(got) != 0 {
		t.Fatalf("deleted %v while a second hold is active", got)
	}
	g.endScan()
	second()
	if _, ok := g.held["id"]; ok {
		t.Fatalf("hold not cleared")
	}
}

func TestBlobGuardOverlappingScans(t *testing.T) {
	var g blobGuard
	g.beginScan()
	g.beginScan()
	g.hold("id")()
	g.endScan()
	if got := guardDeleted(&g, "id"); len(got) != 0 {
		t.Fatalf("deleted %v while the second scan runs", got)
	}
	g.endScan()
	if g.touched != nil {
		t.Fatalf("touched kept after the last scan")
	}
}
