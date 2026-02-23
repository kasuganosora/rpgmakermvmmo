package rest_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	"gorm.io/gorm"
)

// newGuildSetup creates a router with guild endpoints, a user account+character, and returns a token.
func newGuildSetup(t *testing.T) (r *gin.Engine, db *gorm.DB, accountID int64, charID int64, token string) {
	db = testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	guildH := rest.NewGuildHandler(db)

	r = gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.POST("/guilds", guildH.Create)
	authGroup.GET("/guilds/:id", guildH.Detail)
	authGroup.POST("/guilds/:id/join", guildH.Join)
	authGroup.DELETE("/guilds/:id/members/:cid", guildH.KickMember)
	authGroup.PUT("/guilds/:id/notice", guildH.UpdateNotice)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "guilduser", "password": "pass1234"})
	var lr map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lr))
	token = lr["token"].(string)
	accountID = int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "GuildLeader", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)
	charID = char.ID

	return r, db, accountID, charID, token
}

func putJSON(r *gin.Engine, path string, body interface{}, headers ...string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func deleteReq(r *gin.Engine, path string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---- Create ----

func TestGuildCreate_Success(t *testing.T) {
	r, _, _, _, token := newGuildSetup(t)

	w := postJSON(r, "/api/guilds", map[string]interface{}{"name": "TestGuild", "notice": ""},
		"Authorization", "Bearer "+token)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "TestGuild", resp["name"])
}

func TestGuildCreate_NoCharacter(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	guildH := rest.NewGuildHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.POST("/guilds", guildH.Create)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "nocharuser", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)

	w2 := postJSON(r, "/api/guilds", map[string]interface{}{"name": "GuildXY", "notice": ""},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestGuildCreate_InvalidName(t *testing.T) {
	r, _, _, _, token := newGuildSetup(t)

	// Name too short (min=2)
	w := postJSON(r, "/api/guilds", map[string]interface{}{"name": "X"},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---- Detail ----

func TestGuildDetail_Found(t *testing.T) {
	r, db, _, _, token := newGuildSetup(t)

	guild := &model.Guild{Name: "DetailGuild", LeaderID: 1, Level: 1}
	require.NoError(t, db.Create(guild).Error)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/guilds/%d", guild.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	guildResp := resp["guild"].(map[string]interface{})
	assert.Equal(t, "DetailGuild", guildResp["name"])
}

func TestGuildDetail_NotFound(t *testing.T) {
	r, _, _, _, token := newGuildSetup(t)

	req := httptest.NewRequest(http.MethodGet, "/api/guilds/9999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGuildDetail_InvalidID(t *testing.T) {
	r, _, _, _, token := newGuildSetup(t)

	req := httptest.NewRequest(http.MethodGet, "/api/guilds/abc", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---- Join ----

func TestGuildJoin_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	guildH := rest.NewGuildHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.POST("/guilds/:id/join", guildH.Join)

	guild := &model.Guild{Name: "JoinableGuild", LeaderID: 999, Level: 1}
	require.NoError(t, db.Create(guild).Error)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "joiner", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "Joiner", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	w2 := postJSON(r, fmt.Sprintf("/api/guilds/%d/join", guild.ID), nil,
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestGuildJoin_GuildNotFound(t *testing.T) {
	r, _, _, _, token := newGuildSetup(t)

	w := postJSON(r, "/api/guilds/9999/join", nil, "Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGuildJoin_InvalidID(t *testing.T) {
	r, _, _, _, token := newGuildSetup(t)

	w := postJSON(r, "/api/guilds/abc/join", nil, "Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGuildJoin_NoCharacter(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	guildH := rest.NewGuildHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.POST("/guilds/:id/join", guildH.Join)

	guild := &model.Guild{Name: "JoinGuild2", LeaderID: 999, Level: 1}
	require.NoError(t, db.Create(guild).Error)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "joiner2", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)

	// No character for this account
	w2 := postJSON(r, fmt.Sprintf("/api/guilds/%d/join", guild.ID), nil,
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

// ---- UpdateNotice ----

func TestGuildUpdateNotice_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	guildH := rest.NewGuildHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.PUT("/guilds/:id/notice", guildH.UpdateNotice)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "leader1", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "Leader1", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	guild := &model.Guild{Name: "NoticeGuild", LeaderID: char.ID, Level: 1}
	require.NoError(t, db.Create(guild).Error)

	w2 := putJSON(r, fmt.Sprintf("/api/guilds/%d/notice", guild.ID),
		map[string]string{"notice": "Hello guild!"},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestGuildUpdateNotice_NotLeader(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	guildH := rest.NewGuildHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.PUT("/guilds/:id/notice", guildH.UpdateNotice)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "notleader", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "NotLeader", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	// Guild with a different leader
	guild := &model.Guild{Name: "OtherLeaderGuild", LeaderID: 9999, Level: 1}
	require.NoError(t, db.Create(guild).Error)

	w2 := putJSON(r, fmt.Sprintf("/api/guilds/%d/notice", guild.ID),
		map[string]string{"notice": "Attempt"},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusForbidden, w2.Code)
}

func TestGuildUpdateNotice_GuildNotFound(t *testing.T) {
	r, _, _, _, token := newGuildSetup(t)

	w := putJSON(r, "/api/guilds/9999/notice",
		map[string]string{"notice": "x"},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---- KickMember ----

func TestGuildKick_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	guildH := rest.NewGuildHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.DELETE("/guilds/:id/members/:cid", guildH.KickMember)

	// Login as officer
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "officer", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	officerChar := &model.Character{AccountID: accountID, Name: "Officer", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(officerChar).Error)

	targetChar := &model.Character{AccountID: 9999, Name: "Target", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(targetChar).Error)

	guild := &model.Guild{Name: "KickGuild", LeaderID: officerChar.ID, Level: 1}
	require.NoError(t, db.Create(guild).Error)

	// Add officer as rank 1 (leader)
	require.NoError(t, db.Create(&model.GuildMember{GuildID: guild.ID, CharID: officerChar.ID, Rank: 1}).Error)
	// Add target as rank 3 (member)
	require.NoError(t, db.Create(&model.GuildMember{GuildID: guild.ID, CharID: targetChar.ID, Rank: 3}).Error)

	w2 := deleteReq(r, fmt.Sprintf("/api/guilds/%d/members/%d", guild.ID, targetChar.ID),
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestGuildKick_InsufficientRank(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	guildH := rest.NewGuildHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.DELETE("/guilds/:id/members/:cid", guildH.KickMember)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "lowrank", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	lowChar := &model.Character{AccountID: accountID, Name: "LowRank", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(lowChar).Error)

	targetChar := &model.Character{AccountID: 9999, Name: "Target2", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(targetChar).Error)

	guild := &model.Guild{Name: "RankGuild", LeaderID: 9999, Level: 1}
	require.NoError(t, db.Create(guild).Error)

	// low rank member (rank=3, cannot kick)
	require.NoError(t, db.Create(&model.GuildMember{GuildID: guild.ID, CharID: lowChar.ID, Rank: 3}).Error)

	w2 := deleteReq(r, fmt.Sprintf("/api/guilds/%d/members/%d", guild.ID, targetChar.ID),
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusForbidden, w2.Code)
}

func TestGuildKick_NotGuildMember(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	guildH := rest.NewGuildHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.DELETE("/guilds/:id/members/:cid", guildH.KickMember)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "outsider", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	outsiderChar := &model.Character{AccountID: accountID, Name: "Outsider", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(outsiderChar).Error)

	guild := &model.Guild{Name: "ClosedGuild", LeaderID: 9999, Level: 1}
	require.NoError(t, db.Create(guild).Error)
	// outsider is NOT a guild member

	w2 := deleteReq(r, fmt.Sprintf("/api/guilds/%d/members/%d", guild.ID, 9999),
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusForbidden, w2.Code)
}
