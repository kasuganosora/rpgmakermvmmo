package rest_test

import (
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
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func newSocialSetup(t *testing.T) (r *gin.Engine, db *gorm.DB, accountID int64, charID int64, token string) {
	db = testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}
	logger, _ := zap.NewDevelopment()
	sm := player.NewSessionManager(logger)

	authH := rest.NewAuthHandler(db, c, sec)
	socialH := rest.NewSocialHandler(db, sm)

	r = gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.GET("/social/friends", socialH.ListFriends)
	authGroup.POST("/social/friends/request", socialH.SendFriendRequest)
	authGroup.POST("/social/friends/accept/:id", socialH.AcceptFriendRequest)
	authGroup.DELETE("/social/friends/:id", socialH.DeleteFriend)
	authGroup.POST("/social/block/:id", socialH.BlockPlayer)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "socialuser", "password": "pass1234"})
	var lr map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lr))
	token = lr["token"].(string)
	accountID = int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "SocialPlayer", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)
	charID = char.ID

	return r, db, accountID, charID, token
}

// ---- ListFriends ----

func TestSocialListFriends_Empty(t *testing.T) {
	r, _, _, _, token := newSocialSetup(t)

	req := httptest.NewRequest(http.MethodGet, "/api/social/friends", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	friends := resp["friends"].([]interface{})
	assert.Len(t, friends, 0)
}

func TestSocialListFriends_WithFriends(t *testing.T) {
	r, db, _, charID, token := newSocialSetup(t)

	// Add a friendship
	require.NoError(t, db.Create(&model.Friendship{CharID: charID, FriendID: 999, Status: 1}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/social/friends", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	friends := resp["friends"].([]interface{})
	assert.Len(t, friends, 1)
}

func TestSocialListFriends_NoCharacter(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}
	logger, _ := zap.NewDevelopment()
	sm := player.NewSessionManager(logger)

	authH := rest.NewAuthHandler(db, c, sec)
	socialH := rest.NewSocialHandler(db, sm)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.GET("/social/friends", socialH.ListFriends)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "nocharfriend", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/social/friends", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)
	assert.Equal(t, http.StatusBadRequest, wr.Code)
}

// ---- SendFriendRequest ----

func TestSocialSendFriendRequest_Success(t *testing.T) {
	r, db, _, charID, token := newSocialSetup(t)

	// Create a target character
	target := &model.Character{AccountID: 9999, Name: "Target", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(target).Error)
	_ = charID

	w := postJSON(r, "/api/social/friends/request",
		map[string]interface{}{"target_char_id": target.ID},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestSocialSendFriendRequest_BadRequest(t *testing.T) {
	r, _, _, _, token := newSocialSetup(t)

	// Missing target_char_id
	w := postJSON(r, "/api/social/friends/request",
		map[string]interface{}{},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSocialSendFriendRequest_NoCharacter(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}
	logger, _ := zap.NewDevelopment()
	sm := player.NewSessionManager(logger)

	authH := rest.NewAuthHandler(db, c, sec)
	socialH := rest.NewSocialHandler(db, sm)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.POST("/social/friends/request", socialH.SendFriendRequest)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "nocharreq", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)

	w2 := postJSON(r, "/api/social/friends/request",
		map[string]interface{}{"target_char_id": 1},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

// ---- AcceptFriendRequest ----

func TestSocialAccept_Success(t *testing.T) {
	r, db, _, charID, token := newSocialSetup(t)

	// Another character sends us a friend request
	otherChar := &model.Character{AccountID: 8888, Name: "Other", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(otherChar).Error)

	// Friendship: otherChar → charID, status=0 (pending)
	friendship := &model.Friendship{CharID: otherChar.ID, FriendID: charID, Status: 0}
	require.NoError(t, db.Create(friendship).Error)

	w := postJSON(r, fmt.Sprintf("/api/social/friends/accept/%d", friendship.ID), nil,
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSocialAccept_NotFound(t *testing.T) {
	r, _, _, _, token := newSocialSetup(t)

	w := postJSON(r, "/api/social/friends/accept/9999", nil, "Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSocialAccept_InvalidID(t *testing.T) {
	r, _, _, _, token := newSocialSetup(t)

	w := postJSON(r, "/api/social/friends/accept/abc", nil, "Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---- DeleteFriend ----

func TestSocialDeleteFriend_Success(t *testing.T) {
	r, db, _, charID, token := newSocialSetup(t)

	friend := &model.Character{AccountID: 7777, Name: "Friend", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(friend).Error)
	require.NoError(t, db.Create(&model.Friendship{CharID: charID, FriendID: friend.ID, Status: 1}).Error)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/social/friends/%d", friend.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSocialDeleteFriend_InvalidID(t *testing.T) {
	r, _, _, _, token := newSocialSetup(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/social/friends/abc", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---- BlockPlayer ----

func TestSocialBlock_Success(t *testing.T) {
	r, db, _, _, token := newSocialSetup(t)

	target := &model.Character{AccountID: 6666, Name: "Blocked", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(target).Error)

	w := postJSON(r, fmt.Sprintf("/api/social/block/%d", target.ID), nil,
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSocialBlock_UpdatesExistingFriendship(t *testing.T) {
	r, db, _, charID, token := newSocialSetup(t)

	// Already a friend, now block
	friend := &model.Character{AccountID: 5555, Name: "TurnedEnemy", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(friend).Error)
	require.NoError(t, db.Create(&model.Friendship{CharID: charID, FriendID: friend.ID, Status: 1}).Error)

	w := postJSON(r, fmt.Sprintf("/api/social/block/%d", friend.ID), nil,
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSocialBlock_InvalidID(t *testing.T) {
	r, _, _, _, token := newSocialSetup(t)

	w := postJSON(r, "/api/social/block/abc", nil, "Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
