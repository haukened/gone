package store

import "sync"

// blobGuard keeps Reconcile from deleting the blob of a save that has not
// committed its index row yet. Save and Fill write the blob before the row,
// and the row can wait seconds for its write turn, so a blob with no row is
// not necessarily an orphan.
//
// Every save holds its ID from before the blob is written until the row has
// committed or the save has failed. While a scan runs, every ID held at any
// point since it began is marked touched, and Reconcile deletes only blobs
// whose ID was never touched. Such a blob's save had finished before the scan
// began, so its row (if any) is visible to the scan's index read. The guard is
// per process: it relies on this Store being the only writer of its blob
// directory.
type blobGuard struct {
	mu      sync.Mutex
	held    map[string]int      // saves in flight, per ID
	touched map[string]struct{} // IDs held since the oldest running scan began; nil when no scan runs
	scans   int                 // running scans
}

// hold marks a save of id as in flight.
//
// Parameters:
//   - id: secret or request identifier being saved.
//
// Returns a function that ends the hold; call it once the index row has
// committed or the save has failed.
func (g *blobGuard) hold(id string) func() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.held == nil {
		g.held = make(map[string]int)
	}
	g.held[id]++
	if g.touched != nil {
		g.touched[id] = struct{}{}
	}
	return func() { g.release(id) }
}

// release ends one hold of id.
//
// Parameters:
//   - id: identifier passed to hold.
func (g *blobGuard) release(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.held[id]--; g.held[id] <= 0 {
		delete(g.held, id)
	}
}

// beginScan starts tracking touched IDs for a Reconcile pass. IDs already in
// flight count as touched. Call it before listing blobs, and endScan after
// the last deletion.
func (g *blobGuard) beginScan() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.touched == nil {
		g.touched = make(map[string]struct{}, len(g.held))
	}
	for id := range g.held {
		g.touched[id] = struct{}{}
	}
	g.scans++
}

// endScan stops tracking for one Reconcile pass; the touched set is dropped
// when no scan is left running.
func (g *blobGuard) endScan() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.scans--; g.scans <= 0 {
		g.scans, g.touched = 0, nil
	}
}

// deleteUntouched calls del for id unless a save of id was held since the
// scan began. The check and the deletion run under the guard's lock, so a
// save that starts meanwhile writes its blob only after the deletion.
//
// Parameters:
//   - id: blob identifier with no index row.
//   - del: blob deletion function.
func (g *blobGuard) deleteUntouched(id string, del func(string) error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.touched[id]; ok {
		return
	}
	_ = del(id) // best-effort; the next scan retries
}
