package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestEncryptionStatus_Disabled returns Enabled=false when no key is set.
func TestEncryptionStatus_Disabled(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/encryption/status", nil)
	s.handleEncryptionStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got RotationStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Error("expected Enabled=false")
	}
}

// TestEncryptionStatus_Enabled wires an encryptor and confirms the
// summary echoes the primary keyID and per-collection counts.
func TestEncryptionStatus_Enabled(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	s.CollectionManager = NewCollectionManager(s.DB)
	if err := s.CollectionManager.EnsureBucket(); err != nil {
		t.Fatal(err)
	}

	k := genKey(t)
	e := withKey(t, k, "5", "")
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)
	e.SetCollectionEnabled("c", true)
	s.Encryptor = e
	s.RotationManager = NewRotationManager(s, e)
	if err := s.CollectionManager.Set("c", &CollectionConfig{Type: "default", Encrypted: true}); err != nil {
		t.Fatal(err)
	}
	addTestDoc(t, s, "c", "k1", "en", "v", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/encryption/status", nil)
	s.handleEncryptionStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got RotationStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.PrimaryKeyID != 5 {
		t.Fatalf("status: %+v", got)
	}
	if len(got.Collections) == 0 || got.Collections[0].WithPrimary != 1 {
		t.Fatalf("collection counts: %+v", got.Collections)
	}
}

// TestEncryptionStatus_WrongMethod refuses non-GET.
func TestEncryptionStatus_WrongMethod(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/encryption/status", nil)
	s.handleEncryptionStatus(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// TestEncryptionRotate_NoEncryption returns 400 when encryption is off.
func TestEncryptionRotate_NoEncryption(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/encryption/rotate", strings.NewReader("{}"))
	s.handleEncryptionRotate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestEncryptionRotate_Success kicks off a job and verifies the job
// JSON shape comes back.
func TestEncryptionRotate_Success(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	s.CollectionManager = NewCollectionManager(s.DB)
	if err := s.CollectionManager.EnsureBucket(); err != nil {
		t.Fatal(err)
	}

	k := genKey(t)
	e := withKey(t, k, "1", "")
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)
	s.Encryptor = e
	s.RotationManager = NewRotationManager(s, e)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/encryption/rotate", strings.NewReader(`{"collection":""}`))
	req.ContentLength = 16
	s.handleEncryptionRotate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var job RotationJob
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(job.ID, "rot-") {
		t.Errorf("bad job id: %s", job.ID)
	}
	// give the worker a beat to finish the (empty) DB walk
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		final := s.RotationManager.Get(job.ID)
		if final != nil && (final.Status == RotationCompleted || final.Status == RotationFailed) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("job did not finish in time")
}

// TestEncryptionJob_NotFound returns 404 for an unknown id.
func TestEncryptionJob_NotFound(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	k := genKey(t)
	e := withKey(t, k, "1", "")
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)
	s.RotationManager = NewRotationManager(s, e)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/encryption/jobs/rot-nope", nil)
	s.handleEncryptionJob(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// TestEncryptionJobs_List returns the empty list when nothing has run.
func TestEncryptionJobs_List(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	k := genKey(t)
	e := withKey(t, k, "1", "")
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)
	s.RotationManager = NewRotationManager(s, e)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/encryption/jobs", nil)
	s.handleEncryptionJob(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"jobs"`) {
		t.Errorf("missing jobs field: %s", rec.Body.String())
	}
}

// TestEncryptionStatus_NilManager returns Enabled=false when the
// rotation manager is not configured.
func TestEncryptionStatus_NilManager(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/encryption/status", nil)
	s.handleEncryptionStatus(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Errorf("got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestEncryptionRotate_BadJSON rejects malformed body.
func TestEncryptionRotate_BadJSON(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	k := genKey(t)
	e := withKey(t, k, "1", "")
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)
	s.RotationManager = NewRotationManager(s, e)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/encryption/rotate", strings.NewReader("not-json"))
	req.ContentLength = 8
	s.handleEncryptionRotate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// TestEncryptionRotate_WrongMethod refuses GET.
func TestEncryptionRotate_WrongMethod(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/encryption/rotate", nil)
	s.handleEncryptionRotate(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// TestEncryptionJob_NilManager returns 404 when no rotation manager.
func TestEncryptionJob_NilManager(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/encryption/jobs/anything", nil)
	s.handleEncryptionJob(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// TestEncryptionJob_WrongMethod refuses POST.
func TestEncryptionJob_WrongMethod(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/encryption/jobs", nil)
	s.handleEncryptionJob(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// TestEncryptionRotate_ReadOnlyMode blocks rotation in read-only mode.
func TestEncryptionRotate_ReadOnlyMode(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	s.Mode = ModeRead
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/encryption/rotate", strings.NewReader("{}"))
	s.handleEncryptionRotate(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
