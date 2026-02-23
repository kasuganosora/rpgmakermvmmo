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

func newMailSetup(t *testing.T) (r *gin.Engine, accountID int64, charID int64, token string) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	mailH := rest.NewMailHandler(db)

	r = gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.GET("/characters/:id/mail", mailH.List)
	authGroup.POST("/characters/:id/mail/:mail_id/claim", mailH.Claim)

	// Login to create account
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "mailuser", "password": "pass1234"})
	var lr map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lr))
	token = lr["token"].(string)
	accountID = int64(lr["account_id"].(float64))

	// Create character
	char := &model.Character{
		AccountID: accountID, Name: "MailPlayer", ClassID: 1, HP: 100, MaxHP: 100,
	}
	require.NoError(t, db.Create(char).Error)
	charID = char.ID

	// Create some mail
	mail1 := &model.Mail{
		ToCharID: charID,
		Subject:  "Welcome",
		Body:     "Hello!",
		Claimed:  0,
	}
	mail2 := &model.Mail{
		ToCharID: charID,
		Subject:  "Reward",
		Body:     "Here is your reward",
		Claimed:  0,
	}
	require.NoError(t, db.Create(mail1).Error)
	require.NoError(t, db.Create(mail2).Error)

	return r, accountID, charID, token
}

// ---- Mail List ----

func TestMailList_Success(t *testing.T) {
	r, _, charID, token := newMailSetup(t)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/characters/%d/mail", charID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	mails := resp["mails"].([]interface{})
	assert.Len(t, mails, 2)
}

func TestMailList_InvalidCharID(t *testing.T) {
	r, _, _, token := newMailSetup(t)

	req := httptest.NewRequest(http.MethodGet, "/api/characters/abc/mail", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMailList_ForbiddenOtherChar(t *testing.T) {
	r, _, _, token := newMailSetup(t)

	// charID=9999 doesn't belong to this account
	req := httptest.NewRequest(http.MethodGet, "/api/characters/9999/mail", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ---- Mail Claim ----

func TestMailClaim_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	mailH := rest.NewMailHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.POST("/characters/:id/mail/:mail_id/claim", mailH.Claim)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "claimer", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "Claimer", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	mail := &model.Mail{ToCharID: char.ID, Subject: "Gift", Body: "x", Claimed: 0}
	require.NoError(t, db.Create(mail).Error)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/characters/%d/mail/%d/claim", char.ID, mail.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)
	require.Equal(t, http.StatusOK, wr.Code)

	// Verify claimed
	var updated model.Mail
	db.First(&updated, mail.ID)
	assert.Equal(t, 1, updated.Claimed)
}

func TestMailClaim_AlreadyClaimed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	mailH := rest.NewMailHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.POST("/characters/:id/mail/:mail_id/claim", mailH.Claim)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "claimer2", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "Claimer2", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	mail := &model.Mail{ToCharID: char.ID, Subject: "Gift", Body: "x", Claimed: 1}
	require.NoError(t, db.Create(mail).Error)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/characters/%d/mail/%d/claim", char.ID, mail.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)
	assert.Equal(t, http.StatusConflict, wr.Code)
}

func TestMailClaim_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	mailH := rest.NewMailHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.POST("/characters/:id/mail/:mail_id/claim", mailH.Claim)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "claimer3", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "Claimer3", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/characters/%d/mail/9999/claim", char.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)
	assert.Equal(t, http.StatusNotFound, wr.Code)
}

func TestMailClaim_InvalidIDs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	mailH := rest.NewMailHandler(db)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.POST("/characters/:id/mail/:mail_id/claim", mailH.Claim)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "claimer4", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	// Create a character owned by this account so the handler's ownership check passes
	// and we can reach the mail_id parse step.
	char := &model.Character{AccountID: accountID, Name: "Claimer4", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	cases := []struct {
		path string
		code int
	}{
		{"/api/characters/abc/mail/1/claim", http.StatusBadRequest},
		{fmt.Sprintf("/api/characters/%d/mail/abc/claim", char.ID), http.StatusBadRequest},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		wr := httptest.NewRecorder()
		r.ServeHTTP(wr, req)
		assert.Equal(t, tc.code, wr.Code, "path: %s", tc.path)
	}
}
