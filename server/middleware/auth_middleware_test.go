package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestCache(t *testing.T) cache.Cache {
	t.Helper()
	c, err := cache.NewCache(cache.CacheConfig{})
	require.NoError(t, err)
	return c
}

func newProtectedRouter(sec config.SecurityConfig, c cache.Cache) *gin.Engine {
	r := gin.New()
	r.Use(Auth(sec, c))
	r.GET("/protected", func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})
	return r
}

func TestAuth_MissingAuthHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sec := config.SecurityConfig{JWTSecret: "secret", JWTTTLH: time.Hour}
	c := setupTestCache(t)
	r := newProtectedRouter(sec, c)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_NoBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sec := config.SecurityConfig{JWTSecret: "secret", JWTTTLH: time.Hour}
	c := setupTestCache(t)
	r := newProtectedRouter(sec, c)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Token abc123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sec := config.SecurityConfig{JWTSecret: "secret", JWTTTLH: time.Hour}
	c := setupTestCache(t)
	r := newProtectedRouter(sec, c)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer notavalidtoken")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_SessionExpired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sec := config.SecurityConfig{JWTSecret: "secret", JWTTTLH: time.Hour}
	c := setupTestCache(t)
	r := newProtectedRouter(sec, c)

	// Generate a valid JWT but do NOT store session in cache
	token, err := GenerateToken(42, "secret", time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sec := config.SecurityConfig{JWTSecret: "secret", JWTTTLH: time.Hour}
	c := setupTestCache(t)
	r := newProtectedRouter(sec, c)

	token, err := GenerateToken(42, "secret", time.Hour)
	require.NoError(t, err)
	require.NoError(t, c.Set(context.Background(), "session:"+token, "42", time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuth_SetsAccountIDInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sec := config.SecurityConfig{JWTSecret: "secret", JWTTTLH: time.Hour}
	c := setupTestCache(t)

	var gotAccountID int64
	r := gin.New()
	r.Use(Auth(sec, c))
	r.GET("/me", func(ctx *gin.Context) {
		gotAccountID = GetAccountID(ctx)
		ctx.Status(http.StatusOK)
	})

	token, err := GenerateToken(42, "secret", time.Hour)
	require.NoError(t, err)
	require.NoError(t, c.Set(context.Background(), "session:"+token, "42", time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(42), gotAccountID)
}

func TestGetAccountID_Missing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.Equal(t, int64(0), GetAccountID(c))
}

func TestGetAccountID_Present(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(AccountIDKey, int64(99))
	assert.Equal(t, int64(99), GetAccountID(c))
}

func TestRecovery_CatchesPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, _ := zap.NewDevelopment()

	r := gin.New()
	r.Use(TraceID())
	r.Use(Recovery(logger))
	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRecovery_NoPanic_PassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, _ := zap.NewDevelopment()

	r := gin.New()
	r.Use(Recovery(logger))
	r.GET("/ok", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestLogger_RequestLogged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, _ := zap.NewDevelopment()

	r := gin.New()
	r.Use(TraceID())
	r.Use(Logger(logger))
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestLogger_ErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, _ := zap.NewDevelopment()

	r := gin.New()
	r.Use(Logger(logger))
	r.GET("/fail", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
