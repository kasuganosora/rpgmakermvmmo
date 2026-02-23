package rest_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/api/rest"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newAuthRouter(t *testing.T) (*gin.Engine, *rest.AuthHandler) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{
		JWTSecret: "test-secret",
		JWTTTLH:   72 * time.Hour,
	}
	h := rest.NewAuthHandler(db, c, sec)
	r := gin.New()
	r.POST("/api/auth/login", h.Login)
	r.POST("/api/auth/logout", mw.Auth(sec, c), h.Logout)
	r.POST("/api/auth/refresh", mw.Auth(sec, c), h.Refresh)
	return r, h
}

func postJSON(r *gin.Engine, path string, body interface{}, headers ...string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestLoginAutoRegister(t *testing.T) {
	r, _ := newAuthRouter(t)

	w := postJSON(r, "/api/auth/login", map[string]string{
		"username": "alice",
		"password": "pass1234",
	})
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["token"])
	assert.NotZero(t, resp["account_id"])
}

func TestLoginWrongPassword(t *testing.T) {
	r, _ := newAuthRouter(t)

	// Register first
	postJSON(r, "/api/auth/login", map[string]string{"username": "bob", "password": "correct"})

	// Wrong password
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "bob", "password": "wrong"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLoginSecondTime(t *testing.T) {
	r, _ := newAuthRouter(t)

	w1 := postJSON(r, "/api/auth/login", map[string]string{"username": "carol", "password": "pass1234"})
	require.Equal(t, http.StatusOK, w1.Code)

	// Same credentials → should succeed again
	w2 := postJSON(r, "/api/auth/login", map[string]string{"username": "carol", "password": "pass1234"})
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestLogout(t *testing.T) {
	r, _ := newAuthRouter(t)

	// Login
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "dave", "password": "pass1234"})
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token := resp["token"].(string)

	// Logout
	w2 := postJSON(r, "/api/auth/logout", nil, "Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Second attempt with same token should fail (session removed)
	w3 := postJSON(r, "/api/auth/logout", nil, "Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, w3.Code)
}

func TestRefresh(t *testing.T) {
	r, _ := newAuthRouter(t)

	// Login first
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "refreshuser", "password": "pass1234"})
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token := resp["token"].(string)

	// Refresh should succeed and return a token
	w2 := postJSON(r, "/api/auth/refresh", nil, "Authorization", "Bearer "+token)
	require.Equal(t, http.StatusOK, w2.Code)
	var resp2 map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	newToken := resp2["token"].(string)
	assert.NotEmpty(t, newToken)
}

func TestRefresh_NoToken(t *testing.T) {
	r, _ := newAuthRouter(t)
	// Without a valid Bearer token the Auth middleware rejects with 401
	w := postJSON(r, "/api/auth/refresh", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLoginBannedAccount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}
	h := rest.NewAuthHandler(db, c, sec)
	r := gin.New()
	r.POST("/api/auth/login", h.Login)

	// Auto-register "bannedacc"
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "bannedacc", "password": "pass1234"})
	require.Equal(t, http.StatusOK, w.Code)

	// Ban the account by setting Status=0
	db.Model(&model.Account{}).Where("username = ?", "bannedacc").Update("status", 0)

	// Login should now return 403 Forbidden
	w2 := postJSON(r, "/api/auth/login", map[string]string{"username": "bannedacc", "password": "pass1234"})
	assert.Equal(t, http.StatusForbidden, w2.Code)
}
