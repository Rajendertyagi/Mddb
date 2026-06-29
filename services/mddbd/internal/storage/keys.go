package storage

// DocKey is the BoltDB key for a document by collection+id.
func DocKey(coll, id string) []byte { return []byte("doc|" + coll + "|" + id) }

// ByKeyKey is the secondary-index key mapping collection+key+lang to a doc id.
func ByKeyKey(coll, key, lang string) []byte {
	return []byte("bykey|" + coll + "|" + key + "|" + lang)
}

// RevPrefix is the key prefix for a document's revision history.
func RevPrefix(coll, id string) []byte { return []byte("rev|" + coll + "|" + id + "|") }

// MetaKeyPrefix is the key prefix for the metadata-value index.
func MetaKeyPrefix(coll, mk, mv string) []byte {
	return []byte("meta|" + coll + "|" + mk + "|" + mv + "|")
}
