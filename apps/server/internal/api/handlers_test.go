package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/andyfcx/observer/server/internal/api"
)

// TestRouterHealth verifies the /health endpoint returns 200 without auth.
func TestRouterHealth(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestRouterAuthRequired verifies that /api/v1 endpoints reject requests without a token.
func TestRouterAuthRequired(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestEnrollRoutePublicValidation(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/enroll", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLoginSuccess(t *testing.T) {
	router := newTestRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte(`{"username":"admin","password":"secret"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp api.LoginResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "admin", resp.Username)
	assert.NotEmpty(t, resp.Token)
}

func TestLoginSessionCanAccessAPI(t *testing.T) {
	router := newTestRouter(t)

	loginRec := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte(`{"username":"admin","password":"secret"}`)))
	loginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(loginRec, loginReq)
	require.Equal(t, http.StatusOK, loginRec.Code)

	var loginResp api.LoginResponse
	require.NoError(t, json.NewDecoder(loginRec.Body).Decode(&loginResp))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/heartbeat",
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRegisterAgentValidation verifies that missing fields return 400.
func TestRegisterAgentValidation(t *testing.T) {
	router := newTestRouter(t)

	// Missing machine_id and hostname.
	body := map[string]any{"ip_address": "1.2.3.4"}
	b, _ := json.Marshal(body)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Contains(t, resp["error"].(string), "required")
}

// TestParsePagination exercises the pagination helper via the executions endpoint.
// Since we have no DB in unit tests, we only check the auth+routing layer.
func TestHeartbeatRequiresHostID(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/heartbeat",
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	// No X-Host-ID header → 400
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// newTestRouter builds a router with stub/nil services for route-level tests.
// For integration tests that need a real DB, use a test container or test DB.
func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	// Passing nil services — only tests that don't reach service layer pass.
	// TODO: replace with mocks for service-level tests.
	auth := api.NewAuthManager("test-token", "admin", "secret", nil)
	h := api.NewHandler(auth, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	return api.NewRouter(h)
}
