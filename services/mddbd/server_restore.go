package main

import (
	bolt "go.etcd.io/bbolt"
)

// GO-004: replication snapshot restore swaps the follower's *bolt.DB out from
// under live read handlers. These helpers funnel every production BoltDB access
// through a shared RWMutex (restoreMu) so a restore can drain in-flight reads
// and publish the new handle atomically — instead of closing the database while
// a DB.View is mid-flight (panic / "database not open") and racing the pointer
// swap.
//
// NOTE: this file is intentionally excluded from the codemod that rewrote
// `<server>.DB.View(` → `<server>.DBView(`. The literal `s.DB.View`/`s.DB.Update`
// below are the *only* unguarded access points and must call bolt directly.

// DBView runs fn inside a read-only BoltDB transaction while holding the restore
// read lock, so the handle cannot be closed/swapped underneath it.
func (s *Server) DBView(fn func(*bolt.Tx) error) error {
	s.restoreMu.RLock()
	defer s.restoreMu.RUnlock()
	return s.DB.View(fn)
}

// DBUpdate runs fn inside a read-write BoltDB transaction while holding the
// restore read lock.
func (s *Server) DBUpdate(fn func(*bolt.Tx) error) error {
	s.restoreMu.RLock()
	defer s.restoreMu.RUnlock()
	return s.DB.Update(fn)
}

// withRestoreLock executes swap while holding the exclusive restore lock. It is
// the only path that may close/replace s.DB or reload the in-memory managers,
// and it blocks until every in-flight DBView/DBUpdate has returned.
func (s *Server) withRestoreLock(swap func() error) error {
	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	return swap()
}
