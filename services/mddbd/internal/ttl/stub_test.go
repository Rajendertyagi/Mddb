package ttl

import "mddb/internal/storage"

// stubReaper is a no-op Reaper for the manager tests that never trigger cleanup.
type stubReaper struct{}

func (stubReaper) LoadDoc([]byte) (*storage.Doc, error) { return nil, nil }
func (stubReaper) DeleteDocument(_, _, _ string) error  { return nil }
func (stubReaper) GenID(_, _, _ string) string          { return "" }
