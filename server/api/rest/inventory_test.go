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
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newInventorySetup(t *testing.T) (r *gin.Engine, accountID int64, charID int64, token string) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	invH := rest.NewInventoryHandler(db)

	r = gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.GET("/characters/:id/inventory", invH.List)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "invuser", "password": "pass1234"})
	var lr map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lr))
	token = lr["token"].(string)
	accountID = int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "InvPlayer", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)
	charID = char.ID

	// Add some inventory items
	require.NoError(t, db.Create(&model.Inventory{CharID: charID, ItemID: 1, Kind: 1, Qty: 5}).Error)
	require.NoError(t, db.Create(&model.Inventory{CharID: charID, ItemID: 2, Kind: 2, Qty: 1}).Error)

	return r, accountID, charID, token
}

func TestInventoryList_Success(t *testing.T) {
	r, _, charID, token := newInventorySetup(t)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/characters/%d/inventory", charID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	inventory := resp["inventory"].([]interface{})
	assert.Len(t, inventory, 2)
}

func TestInventoryList_EmptyInventory(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	invH := rest.NewInventoryHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.GET("/characters/:id/inventory", invH.List)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "emptyinv", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "EmptyPlayer", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/characters/%d/inventory", char.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)

	require.Equal(t, http.StatusOK, wr.Code)
	var resp map[string]interface{}
	json.Unmarshal(wr.Body.Bytes(), &resp)
	inventory := resp["inventory"].([]interface{})
	assert.Len(t, inventory, 0)
}

func TestInventoryList_InvalidCharID(t *testing.T) {
	r, _, _, token := newInventorySetup(t)

	req := httptest.NewRequest(http.MethodGet, "/api/characters/abc/inventory", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestInventoryList_CharNotFound(t *testing.T) {
	r, _, _, token := newInventorySetup(t)

	req := httptest.NewRequest(http.MethodGet, "/api/characters/9999/inventory", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// 9999 does not belong to this account
	assert.Equal(t, http.StatusNotFound, w.Code)
}
