package rest_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/api/rest"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/scheduler"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func nopLogger() *zap.Logger { l, _ := zap.NewDevelopment(); return l }

func newAdminRouter(t *testing.T, adminKey string) (*gin.Engine, *rest.AdminHandler) {
	db := testutil.SetupTestDB(t)
	sm := player.NewSessionManager(nopLogger())
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nopLogger())
	sched := scheduler.New(nopLogger())
	h := rest.NewAdminHandler(db, sm, wm, sched, nopLogger())

	r := gin.New()
	r.Use(rest.AdminAuth(adminKey))
	r.GET("/api/admin/metrics", h.Metrics)
	r.GET("/api/admin/players", h.ListPlayers)
	r.POST("/api/admin/kick/:id", h.KickPlayer)
	r.POST("/api/admin/accounts/:id/ban", h.BanAccount)
	r.GET("/api/admin/scheduler", h.ListSchedulerTasks)

	return r, h
}

func adminGet(r *gin.Engine, path, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func adminPost(r *gin.Engine, path, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---- AdminAuth ----

func TestAdminAuth_NoKey_Disabled(t *testing.T) {
	// When adminKey is empty, admin endpoints must be disabled (503) so the
	// server cannot be accidentally deployed without protection.
	r, _ := newAdminRouter(t, "")
	w := adminGet(r, "/api/admin/metrics", "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestAdminAuth_WrongKey(t *testing.T) {
	r, _ := newAdminRouter(t, "secret")
	w := adminGet(r, "/api/admin/metrics", "wrong")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminAuth_CorrectKey(t *testing.T) {
	r, _ := newAdminRouter(t, "secret")
	w := adminGet(r, "/api/admin/metrics", "secret")
	assert.Equal(t, http.StatusOK, w.Code)
}

// ---- Metrics ----

func TestMetrics_Structure(t *testing.T) {
	r, _ := newAdminRouter(t, "test-key")
	w := adminGet(r, "/api/admin/metrics", "test-key")
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "online_players")
	assert.Contains(t, resp, "active_rooms")
	assert.Contains(t, resp, "scheduler_tasks")
}

// ---- ListPlayers ----

func TestListPlayers_Empty(t *testing.T) {
	r, _ := newAdminRouter(t, "test-key")
	w := adminGet(r, "/api/admin/players", "test-key")
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"])
}

// ---- KickPlayer ----

func TestKickPlayer_NotFound(t *testing.T) {
	r, _ := newAdminRouter(t, "test-key")
	w := adminPost(r, "/api/admin/kick/999", "test-key", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestKickPlayer_InvalidID(t *testing.T) {
	r, _ := newAdminRouter(t, "test-key")
	w := adminPost(r, "/api/admin/kick/abc", "test-key", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---- BanAccount ----

func TestBanAccount_NotFound(t *testing.T) {
	r, _ := newAdminRouter(t, "test-key")
	w := adminPost(r, "/api/admin/accounts/999/ban", "test-key", `{"ban":true}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBanAccount_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	sm := player.NewSessionManager(nopLogger())
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nopLogger())
	sched := scheduler.New(nopLogger())
	h := rest.NewAdminHandler(db, sm, wm, sched, nopLogger())

	acc := &model.Account{Username: "testuser", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)

	r := gin.New()
	r.POST("/api/admin/accounts/:id/ban", h.BanAccount)

	body, _ := json.Marshal(map[string]bool{"ban": true})
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/admin/accounts/%d/ban", acc.ID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updatedAcc model.Account
	db.First(&updatedAcc, acc.ID)
	assert.Equal(t, 0, updatedAcc.Status)
}

func TestBanAccount_Unban(t *testing.T) {
	db := testutil.SetupTestDB(t)
	sm := player.NewSessionManager(nopLogger())
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nopLogger())
	sched := scheduler.New(nopLogger())
	h := rest.NewAdminHandler(db, sm, wm, sched, nopLogger())

	acc := &model.Account{Username: "unbanned", PasswordHash: "x", Status: 0}
	require.NoError(t, db.Create(acc).Error)

	r := gin.New()
	r.POST("/api/admin/accounts/:id/ban", h.BanAccount)

	// ban=false → status=1
	body, _ := json.Marshal(map[string]bool{"ban": false})
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/admin/accounts/%d/ban", acc.ID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updatedAcc model.Account
	db.First(&updatedAcc, acc.ID)
	assert.Equal(t, 1, updatedAcc.Status)
}

func TestBanAccount_InvalidID(t *testing.T) {
	r, _ := newAdminRouter(t, "test-key")
	w := adminPost(r, "/api/admin/accounts/abc/ban", "test-key", `{"ban":true}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---- ListSchedulerTasks ----

func TestListSchedulerTasks_Empty(t *testing.T) {
	r, _ := newAdminRouter(t, "test-key")
	w := adminGet(r, "/api/admin/scheduler", "test-key")
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "tasks")
}
