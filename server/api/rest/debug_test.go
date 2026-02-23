package rest_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/api/rest"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestDebugCharCreation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authHandler := rest.NewAuthHandler(db, c, sec)
	charHandler := rest.NewCharacterHandler(db, nil)

	r := gin.New()
	r.POST("/api/auth/login", authHandler.Login)
	auth := r.Group("/api/characters", mw.Auth(sec, c))
	{
		auth.GET("", charHandler.List)
		auth.POST("", charHandler.Create)
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte("testpass"), 12)
	acc := &model.Account{Username: "chartest2", PasswordHash: string(hash), Status: 1}
	require.NoError(t, db.Create(acc).Error)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "chartest2", "password": "testpass"})
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token := resp["token"].(string)

	for i := 1; i <= 3; i++ {
		body := map[string]interface{}{
			"name": fmt.Sprintf("Char%d", i), "class_id": 1,
			"walk_name": "Actor1", "face_name": "Actor1",
		}

		// Also directly test db.Create
		charDirect := &model.Character{
			AccountID: acc.ID,
			Name:      fmt.Sprintf("Direct%d", i),
			ClassID:   1, HP: 100, MP: 50, MaxHP: 100, MaxMP: 50,
		}
		directErr := db.Create(charDirect).Error
		t.Logf("Direct DB create Char%d: err=%v, id=%d", i, directErr, charDirect.ID)

		// Count before
		var cnt int64
		db.WithContext(context.Background()).Model(&model.Character{}).Count(&cnt)
		t.Logf("Count before http request: %d", cnt)

		w2 := doRequest(r, http.MethodPost, "/api/characters", body, token)
		t.Logf("HTTP Char%d response: %d - %s", i, w2.Code, w2.Body.String())
	}
}
