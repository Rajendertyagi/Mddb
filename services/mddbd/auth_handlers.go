package main

import (
	"net/http"
	"strings"
	"time"

	json "github.com/goccy/go-json"
)

// ---- Request/Response types ----

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterResponse struct {
	Username  string `json:"username"`
	CreatedAt int64  `json:"createdAt"`
}

type CreateAPIKeyRequest struct {
	Description string `json:"description"`
	ExpiresAt   int64  `json:"expiresAt,omitempty"` // 0 = never
}

type CreateAPIKeyResponse struct {
	Key         string `json:"key"`         // shown only once!
	Description string `json:"description"`
	ExpiresAt   int64  `json:"expiresAt"`
	CreatedAt   int64  `json:"createdAt"`
}

type GetMeResponse struct {
	Username  string `json:"username"`
	Admin     bool   `json:"admin"`
	CreatedAt int64  `json:"createdAt"`
}

type SetPermissionRequest struct {
	Username   string `json:"username"`
	Collection string `json:"collection"`
	Read       bool   `json:"read"`
	Write      bool   `json:"write"`
	Admin      bool   `json:"admin"`
}

type SetPermissionResponse struct {
	Status string `json:"status"`
}

// ---- Handlers ----

// handleAuthLogin handles POST /v1/auth/login
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	// Authenticate user
	user, err := s.AuthManager.Authenticate(req.Username, req.Password)
	if err != nil {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	// Check if user has admin privileges
	isAdmin := s.AuthManager.IsAdmin(user.Username)

	// Generate JWT
	token, err := GenerateJWT(user.Username, isAdmin, s.AuthManager.config.JWTSecret, s.AuthManager.config.JWTExpiry)
	if err != nil {
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	// Calculate expiry
	expiresAt := time.Now().Add(s.AuthManager.config.JWTExpiry).Unix()

	resp := LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleAuthRegister handles POST /v1/auth/register (admin only)
func (s *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Check admin permission
	claims, ok := GetClaimsFromContext(r.Context())
	if !ok || !claims.Admin {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	// Create user
	user, err := s.AuthManager.CreateUser(req.Username, req.Password)
	if err != nil {
		if err == ErrUserExists {
			http.Error(w, `{"error":"user already exists"}`, http.StatusConflict)
		} else {
			http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
		}
		return
	}

	resp := RegisterResponse{
		Username:  user.Username,
		CreatedAt: user.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleAuthAPIKey handles POST /v1/auth/api-key
func (s *Server) handleAuthAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Check authentication
	claims, ok := GetClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}

	var req CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	// Create API key
	key, err := s.AuthManager.CreateAPIKey(claims.Username, req.Description, req.ExpiresAt)
	if err != nil {
		http.Error(w, `{"error":"failed to create api key"}`, http.StatusInternalServerError)
		return
	}

	resp := CreateAPIKeyResponse{
		Key:         key,
		Description: req.Description,
		ExpiresAt:   req.ExpiresAt,
		CreatedAt:   time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleAuthMe handles GET /v1/auth/me
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	claims, ok := GetClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}

	user, err := s.AuthManager.GetUser(claims.Username)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	resp := GetMeResponse{
		Username:  user.Username,
		Admin:     claims.Admin,
		CreatedAt: user.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleAuthPermissions handles POST /v1/auth/permissions and GET /v1/auth/permissions
func (s *Server) handleAuthPermissions(w http.ResponseWriter, r *http.Request) {
	// Check admin permission
	claims, ok := GetClaimsFromContext(r.Context())
	if !ok || !claims.Admin {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method == "POST" {
		// Set permissions
		var req SetPermissionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		perm := &Permission{
			Username:   req.Username,
			Collection: req.Collection,
			Read:       req.Read,
			Write:      req.Write,
			Admin:      req.Admin,
		}

		if err := s.AuthManager.SetPermission(perm); err != nil {
			http.Error(w, `{"error":"failed to set permission"}`, http.StatusInternalServerError)
			return
		}

		resp := SetPermissionResponse{Status: "ok"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

	} else if r.Method == "GET" {
		// Get permissions
		username := r.URL.Query().Get("username")
		if username == "" {
			http.Error(w, `{"error":"username parameter required"}`, http.StatusBadRequest)
			return
		}

		perms := s.AuthManager.GetPermissions(username)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(perms)

	} else {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleAuthDeleteUser handles DELETE /v1/auth/users/:username
func (s *Server) handleAuthDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Check admin permission
	claims, ok := GetClaimsFromContext(r.Context())
	if !ok || !claims.Admin {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	// Extract username from path: /v1/auth/users/alice
	path := strings.TrimPrefix(r.URL.Path, "/v1/auth/users/")
	if path == "" {
		http.Error(w, `{"error":"username required"}`, http.StatusBadRequest)
		return
	}

	if err := s.AuthManager.DeleteUser(path); err != nil {
		if err == ErrUserNotFound {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"failed to delete user"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}
