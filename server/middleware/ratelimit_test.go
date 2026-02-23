package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func newRateLimitRouter(r rate.Limit, b int) *gin.Engine {
	eng := gin.New()
	eng.Use(RateLimit(r, b))
	eng.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	return eng
}

func TestRateLimit_AllowsFirst(t *testing.T) {
	r := newRateLimitRouter(100, 5)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "10.0.0.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimit_Burst(t *testing.T) {
	// Burst of 3, then reject
	r := newRateLimitRouter(0.001, 3) // near-zero refill so we exhaust quickly
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Real-IP", "10.0.1.1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "request %d should be allowed", i+1)
	}
	// 4th request should be rejected
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "10.0.1.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimit_PerIP(t *testing.T) {
	// Two IPs with burst=1 each → each gets one allowed request
	r := newRateLimitRouter(0.001, 1)

	for _, ip := range []string{"10.1.1.1", "10.1.1.2"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Real-IP", ip)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "first request from %s should be OK", ip)
	}

	// Second request from first IP should be rejected
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "10.1.1.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
