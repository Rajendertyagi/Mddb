package main

import (
	"errors"
	"io"
	"net/http"
	"sync"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

var bucketColMeta = []byte("colmeta")

// CollectionConfig stores per-collection attributes: type, description, icon, color, custom metadata, and storage backend.
type CollectionConfig struct {
	Type           string            `json:"type"` // "default","website","images","audio","documents"
	Description    string            `json:"description,omitempty"`
	Icon           string            `json:"icon,omitempty"`
	Color          string            `json:"color,omitempty"`
	CustomMeta     map[string]string `json:"customMeta,omitempty"`
	StorageBackend string            `json:"storageBackend,omitempty"` // "boltdb" (default), "memory", "s3"
	StorageConfig  *StorageConfigDef `json:"storageConfig,omitempty"`  // backend-specific settings (required for s3)
}

// StorageConfigDef holds backend-specific configuration for non-default storage backends.
type StorageConfigDef struct {
	// S3 / MinIO settings
	Endpoint  string `json:"endpoint,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	Region    string `json:"region,omitempty"`
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	UseTLS    bool   `json:"useTLS,omitempty"`
}

// CollectionManager manages per-collection configuration in a dedicated BoltDB bucket.
type CollectionManager struct {
	db     *bolt.DB
	mu     sync.RWMutex
	cache  map[string]*CollectionConfig
	binlog *Binlog
}

// NewCollectionManager creates a new collection config manager.
func NewCollectionManager(db *bolt.DB) *CollectionManager {
	return &CollectionManager{
		db:    db,
		cache: make(map[string]*CollectionConfig),
	}
}

// SetBinlog sets the binlog for replication logging.
func (cm *CollectionManager) SetBinlog(bl *Binlog) {
	cm.binlog = bl
}

// EnsureBucket creates the colmeta bucket if it doesn't exist.
func (cm *CollectionManager) EnsureBucket() error {
	return cm.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketColMeta)
		return err
	})
}

// LoadAll loads all collection configs from BoltDB into the in-memory cache.
func (cm *CollectionManager) LoadAll() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return cm.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketColMeta)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var cfg CollectionConfig
			if err := json.Unmarshal(v, &cfg); err != nil {
				return nil // skip corrupt entries
			}
			cm.cache[string(k)] = &cfg
			return nil
		})
	})
}

// Get returns the config for a collection. ok is false if unconfigured.
func (cm *CollectionManager) Get(collection string) (*CollectionConfig, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	cfg, ok := cm.cache[collection]
	return cfg, ok
}

// Set stores or updates the config for a collection.
func (cm *CollectionManager) Set(collection string, cfg *CollectionConfig) error {
	if cfg.Type == "" {
		cfg.Type = "default"
	}

	val, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	key := []byte(collection)

	if err := cm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketColMeta)
		return b.Put(key, val)
	}); err != nil {
		return err
	}

	if cm.binlog != nil {
		_ = cm.binlog.Append(&BinlogEntry{Type: BinlogPut, BucketName: "colmeta", Key: copyBytes(key), Value: copyBytes(val)})
	}

	cm.mu.Lock()
	cm.cache[collection] = cfg
	cm.mu.Unlock()
	return nil
}

// Delete removes the config for a collection.
func (cm *CollectionManager) Delete(collection string) error {
	key := []byte(collection)
	if err := cm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketColMeta)
		return b.Delete(key)
	}); err != nil {
		return err
	}

	if cm.binlog != nil {
		_ = cm.binlog.Append(&BinlogEntry{Type: BinlogDelete, BucketName: "colmeta", Key: copyBytes(key)})
	}

	cm.mu.Lock()
	delete(cm.cache, collection)
	cm.mu.Unlock()
	return nil
}

// ListAll returns all configured collections.
func (cm *CollectionManager) ListAll() map[string]*CollectionConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make(map[string]*CollectionConfig, len(cm.cache))
	for k, v := range cm.cache {
		result[k] = v
	}
	return result
}

// --- HTTP Handlers ---

// SetCollectionConfigRequest is the request body for PUT /v1/collection-config.
type SetCollectionConfigRequest struct {
	Collection     string            `json:"collection"`
	Type           string            `json:"type,omitempty"`
	Description    string            `json:"description,omitempty"`
	Icon           string            `json:"icon,omitempty"`
	Color          string            `json:"color,omitempty"`
	CustomMeta     map[string]string `json:"customMeta,omitempty"`
	StorageBackend string            `json:"storageBackend,omitempty"` // "boltdb", "memory", "s3"
	StorageConfig  *StorageConfigDef `json:"storageConfig,omitempty"`
}

func (s *Server) handleCollectionConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleCollectionConfigGet(w, r)
	case http.MethodPut:
		s.handleCollectionConfigSet(w, r)
	case http.MethodDelete:
		s.handleCollectionConfigDelete(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCollectionConfigGet(w http.ResponseWriter, r *http.Request) {
	collection := r.URL.Query().Get("collection")
	if collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}

	// Check read permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	if s.Metrics != nil {
		s.Metrics.IncOp("collection_config_get", collection)
	}

	cfg, found := s.CollectionManager.Get(collection)
	if !found {
		cfg = &CollectionConfig{Type: "default"}
	}
	ok(w, map[string]interface{}{
		"collection": collection,
		"config":     cfg,
		"configured": found,
	})
}

func (s *Server) handleCollectionConfigSet(w http.ResponseWriter, r *http.Request) {
	if s.Mode == ModeRead {
		http.Error(w, `{"error":"read-only mode"}`, http.StatusForbidden)
		return
	}

	var req SetCollectionConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	if s.Metrics != nil {
		s.Metrics.IncOp("collection_config_set", req.Collection)
	}

	// Validate storage backend
	sb := req.StorageBackend
	if sb != "" && sb != "boltdb" && sb != "memory" && sb != "s3" {
		bad(w, errors.New("invalid storageBackend: must be boltdb, memory, or s3"))
		return
	}
	if sb == "s3" && (req.StorageConfig == nil || req.StorageConfig.Endpoint == "" || req.StorageConfig.Bucket == "") {
		bad(w, errors.New("s3 storageBackend requires storageConfig with endpoint and bucket"))
		return
	}

	cfg := &CollectionConfig{
		Type:           req.Type,
		Description:    req.Description,
		Icon:           req.Icon,
		Color:          req.Color,
		CustomMeta:     req.CustomMeta,
		StorageBackend: sb,
		StorageConfig:  req.StorageConfig,
	}
	if err := s.CollectionManager.Set(req.Collection, cfg); err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"status": "ok", "collection": req.Collection})
}

func (s *Server) handleCollectionConfigDelete(w http.ResponseWriter, r *http.Request) {
	if s.Mode == ModeRead {
		http.Error(w, `{"error":"read-only mode"}`, http.StatusForbidden)
		return
	}

	collection := r.URL.Query().Get("collection")
	if collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	if s.Metrics != nil {
		s.Metrics.IncOp("collection_config_delete", collection)
	}

	if err := s.CollectionManager.Delete(collection); err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"status": "ok", "collection": collection})
}

func (s *Server) handleCollectionConfigList(w http.ResponseWriter, r *http.Request) {
	// Check read permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), "*", PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	if s.Metrics != nil {
		s.Metrics.IncOp("collection_config_list", "")
	}

	_, _ = io.Copy(io.Discard, r.Body)
	configs := s.CollectionManager.ListAll()

	type configInfo struct {
		Collection string            `json:"collection"`
		Config     *CollectionConfig `json:"config"`
	}
	var result []configInfo
	for col, cfg := range configs {
		result = append(result, configInfo{Collection: col, Config: cfg})
	}
	if result == nil {
		result = []configInfo{}
	}
	ok(w, map[string]interface{}{
		"configs": result,
		"total":   len(result),
	})
}
