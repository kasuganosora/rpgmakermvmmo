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
	"go.uber.org/zap"
)

func newRankingRouter(t *testing.T) (*gin.Engine, func() string) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}
	logger, _ := zap.NewDevelopment()

	authH := rest.NewAuthHandler(db, c, sec)
	rankH := rest.NewRankingHandler(db, c, logger)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.GET("/ranking/exp", rankH.TopExp)
	authGroup.POST("/ranking/refresh", rankH.RefreshRanking)

	// Create some characters
	for i := 1; i <= 5; i++ {
		acc := &model.Account{
			Username:     "rankuser" + itoa(i),
			PasswordHash: "x",
			Status:       1,
		}
		db.Create(acc)
		char := &model.Character{
			AccountID: acc.ID,
			Name:      "Hero" + itoa(i),
			ClassID:   1,
			HP:        100, MaxHP: 100,
			Level: i,
			Exp:   int64(i * 100),
		}
		db.Create(char)
	}

	getToken := func() string {
		w := postJSON(r, "/api/auth/login", map[string]string{"username": "ranktest", "password": "pass1234"})
		var lr map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &lr)
		return lr["token"].(string)
	}
	return r, getToken
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

func TestRanking_TopExp_FromDB(t *testing.T) {
	r, getToken := newRankingRouter(t)
	token := getToken()

	req := httptest.NewRequest(http.MethodGet, "/api/ranking/exp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	ranking := resp["ranking"].([]interface{})
	assert.Len(t, ranking, 5)

	// First entry should be highest exp (Hero5 with 500 exp)
	first := ranking[0].(map[string]interface{})
	assert.Equal(t, float64(1), first["rank"])
	assert.Equal(t, float64(500), first["exp"])
}

func TestRanking_TopExp_LimitParam(t *testing.T) {
	r, getToken := newRankingRouter(t)
	token := getToken()

	req := httptest.NewRequest(http.MethodGet, "/api/ranking/exp?limit=3", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	ranking := resp["ranking"].([]interface{})
	assert.Len(t, ranking, 3)
}

func TestRanking_TopExp_DefaultLimit(t *testing.T) {
	r, getToken := newRankingRouter(t)
	token := getToken()

	// No limit param → default 20
	req := httptest.NewRequest(http.MethodGet, "/api/ranking/exp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRanking_RefreshRanking(t *testing.T) {
	r, getToken := newRankingRouter(t)
	token := getToken()

	req := httptest.NewRequest(http.MethodPost, "/api/ranking/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(5), resp["refreshed"])
}

func TestRanking_TopExp_FromCache(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}
	logger, _ := zap.NewDevelopment()

	authH := rest.NewAuthHandler(db, c, sec)
	rankH := rest.NewRankingHandler(db, c, logger)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.GET("/ranking/exp", rankH.TopExp)
	authGroup.POST("/ranking/refresh", rankH.RefreshRanking)

	// Create account+char
	acc := &model.Account{Username: "cacherank", PasswordHash: "x", Status: 1}
	db.Create(acc)
	char := &model.Character{AccountID: acc.ID, Name: "CacheHero", ClassID: 1, HP: 100, MaxHP: 100, Level: 10, Exp: 1000}
	db.Create(char)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "cachetest", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)

	// First call populates cache via DB fallback
	req1 := httptest.NewRequest(http.MethodGet, "/api/ranking/exp", nil)
	req1.Header.Set("Authorization", "Bearer "+token)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code)

	// Second call should hit cache
	req2 := httptest.NewRequest(http.MethodGet, "/api/ranking/exp", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)

	var resp map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &resp)
	assert.NotNil(t, resp["ranking"])
}
