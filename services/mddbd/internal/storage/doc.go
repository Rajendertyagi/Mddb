// Package storage holds MDDB's core persisted data model: the Doc document
// type, its protobuf mapping, and the BoltDB key builders. It is dependency-free
// (no Server, no transport) so every layer can share the same types and keys.
package storage

// Doc is a stored MDDB document.
type Doc struct {
	ID        string              `json:"id"`        // generated
	Key       string              `json:"key"`       // e.g. "homepage"
	Lang      string              `json:"lang"`      // e.g. "en_GB"
	Meta      map[string][]string `json:"meta"`      // meta values (multi)
	ContentMD string              `json:"contentMd"` // raw markdown
	AddedAt   int64               `json:"addedAt"`
	UpdatedAt int64               `json:"updatedAt"`
	ExpiresAt int64               `json:"expiresAt,omitempty"` // unix timestamp; 0 = never
}
