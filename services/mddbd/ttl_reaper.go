package main

import "mddb/internal/storage"

// serverTTLReaper adapts *Server to ttl.Reaper, the dependency-inversion seam
// introduced in GO-015. It lets internal/ttl reap expired documents without
// importing the Server god-object.
type serverTTLReaper struct {
	s *Server
}

func (r serverTTLReaper) LoadDoc(v []byte) (*storage.Doc, error) { return loadDoc(v) }

func (r serverTTLReaper) DeleteDocument(collection, key, lang string) error {
	return r.s.deleteDocumentInternal(collection, key, lang)
}

func (r serverTTLReaper) GenID(collection, key, lang string) string {
	return genID(collection, key, lang)
}
