package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newWhitelistRouter(ips []string) *gin.Engine {
	r := gin.New()
	r.Use(IPWhitelist(ips))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestIPWhitelist_Empty_AllowsAll(t *testing.T) {
	r := newWhitelistRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIPWhitelist_AllowedIP(t *testing.T) {
	r := newWhitelistRouter([]string{"192.168.1.1"})
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	// gin.Context.ClientIP() falls back to RemoteAddr when no X-Forwarded-For
	req.Header.Set("X-Real-IP", "192.168.1.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIPWhitelist_BlockedIP(t *testing.T) {
	r := newWhitelistRouter([]string{"10.0.0.1"})
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Real-IP", "1.2.3.4")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestIPWhitelist_MultipleIPs(t *testing.T) {
	allowed := []string{"10.0.0.1", "10.0.0.2"}
	r := newWhitelistRouter(allowed)

	for _, ip := range allowed {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.Header.Set("X-Real-IP", ip)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "expected OK for %s", ip)
	}

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Real-IP", "10.0.0.3")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
