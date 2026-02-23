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
	"golang.org/x/crypto/bcrypt"
)

// loginAndGetToken registers/logs in and returns the JWT.
func loginAndGetToken(t *testing.T, r *gin.Engine, user, pass string) string {
	t.Helper()
	w := postJSON(r, "/api/auth/login", map[string]string{"username": user, "password": pass})
	require.Equal(t, http.StatusOK, w.Code, "login failed: %s", w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp["token"].(string)
}

func doRequest(r *gin.Engine, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func newCharRouter(t *testing.T) (*gin.Engine, string) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authHandler := rest.NewAuthHandler(db, c, sec)
	charHandler := rest.NewCharacterHandler(db, nil, config.GameConfig{StartMapID: 1, StartX: 5, StartY: 5}) // no resource loader in tests

	r := gin.New()
	r.POST("/api/auth/login", authHandler.Login)
	auth := r.Group("/api/characters", mw.Auth(sec, c))
	{
		auth.GET("", charHandler.List)
		auth.POST("", charHandler.Create)
		auth.DELETE("/:id", charHandler.Delete)
	}

	// Pre-create a user and return token
	hash, _ := bcrypt.GenerateFromPassword([]byte("testpass"), 12)
	acc := &model.Account{Username: "chartest", PasswordHash: string(hash), Status: 1}
	require.NoError(t, db.Create(acc).Error)

	// Login to get token
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "chartest", "password": "testpass"})
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return r, resp["token"].(string)
}

func TestCreateCharacter(t *testing.T) {
	r, token := newCharRouter(t)

	w := doRequest(r, http.MethodPost, "/api/characters", map[string]interface{}{
		"name":       "Hero",
		"class_id":   1,
		"walk_name":  "Actor1",
		"walk_index": 0,
		"face_name":  "Actor1",
		"face_index": 0,
	}, token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var char map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &char))
	assert.Equal(t, "Hero", char["name"])
}

func TestCreateCharacterDuplicateName(t *testing.T) {
	r, token := newCharRouter(t)

	body := map[string]interface{}{
		"name": "Unique", "class_id": 1,
		"walk_name": "Actor1", "face_name": "Actor1",
	}
	w1 := doRequest(r, http.MethodPost, "/api/characters", body, token)
	require.Equal(t, http.StatusCreated, w1.Code)

	w2 := doRequest(r, http.MethodPost, "/api/characters", body, token)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestCreateCharacterMaxReached(t *testing.T) {
	r, token := newCharRouter(t)

	for i := 1; i <= 3; i++ {
		body := map[string]interface{}{
			"name": fmt.Sprintf("Char%d", i), "class_id": 1,
			"walk_name": "Actor1", "face_name": "Actor1",
		}
		w := doRequest(r, http.MethodPost, "/api/characters", body, token)
		require.Equal(t, http.StatusCreated, w.Code, "char %d should be created", i)
	}

	// 4th character should fail
	w := doRequest(r, http.MethodPost, "/api/characters", map[string]interface{}{
		"name": "Char4", "class_id": 1, "walk_name": "Actor1", "face_name": "Actor1",
	}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListCharacters(t *testing.T) {
	r, token := newCharRouter(t)

	// Create a character first
	doRequest(r, http.MethodPost, "/api/characters", map[string]interface{}{
		"name": "ListHero", "class_id": 1, "walk_name": "Actor1", "face_name": "Actor1",
	}, token)

	w := doRequest(r, http.MethodGet, "/api/characters", nil, token)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	chars := resp["characters"].([]interface{})
	assert.Len(t, chars, 1)
}

func TestNoTokenReturns401(t *testing.T) {
	r, _ := newCharRouter(t)
	w := doRequest(r, http.MethodGet, "/api/characters", nil, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// newDeleteCharRouter creates a test router with full character routes and
// returns the router, the account DB, and the JWT token for the test user.
func newDeleteCharRouter(t *testing.T) (*gin.Engine, *model.Account, string) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	charH := rest.NewCharacterHandler(db, nil, config.GameConfig{StartMapID: 1, StartX: 5, StartY: 5})

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	auth := r.Group("/api/characters", mw.Auth(sec, c))
	auth.POST("", charH.Create)
	auth.DELETE("/:id", charH.Delete)

	// Login auto-registers the user
	const pass = "delpass456"
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "delcharuser", "password": pass})
	require.Equal(t, http.StatusOK, w.Code)
	var lr map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lr))
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	var acc model.Account
	require.NoError(t, db.First(&acc, accountID).Error)

	return r, &acc, token
}

func TestDeleteCharacter_Success(t *testing.T) {
	r, acc, token := newDeleteCharRouter(t)

	// Create a character directly via the API
	wc := doRequest(r, http.MethodPost, "/api/characters",
		map[string]interface{}{"name": "DelHero", "class_id": 1, "walk_name": "A", "face_name": "B"},
		token)
	require.Equal(t, http.StatusCreated, wc.Code)
	var ch map[string]interface{}
	require.NoError(t, json.Unmarshal(wc.Body.Bytes(), &ch))
	charID := int64(ch["id"].(float64))

	_ = acc // account exists; Delete verifies password stored at auto-register time
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/characters/%d", charID),
		map[string]string{"password": "delpass456"}, token)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteCharacter_WrongPassword(t *testing.T) {
	r, _, token := newDeleteCharRouter(t)

	wc := doRequest(r, http.MethodPost, "/api/characters",
		map[string]interface{}{"name": "DelHero2", "class_id": 1, "walk_name": "A", "face_name": "B"},
		token)
	require.Equal(t, http.StatusCreated, wc.Code)
	var ch map[string]interface{}
	json.Unmarshal(wc.Body.Bytes(), &ch)
	charID := int64(ch["id"].(float64))

	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/characters/%d", charID),
		map[string]string{"password": "wrongpass"}, token)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeleteCharacter_NotFound(t *testing.T) {
	r, _, token := newDeleteCharRouter(t)

	w := doRequest(r, http.MethodDelete, "/api/characters/99999",
		map[string]string{"password": "delpass456"}, token)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteCharacter_InvalidID(t *testing.T) {
	r, _, token := newDeleteCharRouter(t)

	w := doRequest(r, http.MethodDelete, "/api/characters/notanid",
		map[string]string{"password": "delpass456"}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteCharacter_NoBody(t *testing.T) {
	r, _, token := newDeleteCharRouter(t)

	w := doRequest(r, http.MethodDelete, "/api/characters/1", nil, token)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
