package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTraceRouter() *gin.Engine {
	r := gin.New()
	r.Use(TraceID())
	r.GET("/trace", func(c *gin.Context) {
		c.String(http.StatusOK, GetTraceID(c))
	})
	return r
}

func TestTraceID_Generated(t *testing.T) {
	r := newTraceRouter()
	req := httptest.NewRequest(http.MethodGet, "/trace", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	id := w.Body.String()
	assert.NotEmpty(t, id)
	// UUID format: 36 chars
	assert.Len(t, id, 36)
	// Also in response header
	assert.Equal(t, id, w.Header().Get(TraceIDHeader))
}

func TestTraceID_Provided(t *testing.T) {
	r := newTraceRouter()
	req := httptest.NewRequest(http.MethodGet, "/trace", nil)
	req.Header.Set(TraceIDHeader, "my-custom-trace")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	id := w.Body.String()
	assert.Equal(t, "my-custom-trace", id)
	assert.Equal(t, "my-custom-trace", w.Header().Get(TraceIDHeader))
}

func TestGetTraceID_Missing(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.Equal(t, "", GetTraceID(c))
}

func TestTraceID_UniquePerRequest(t *testing.T) {
	r := newTraceRouter()

	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/trace", nil))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/trace", nil))

	assert.NotEqual(t, w1.Body.String(), w2.Body.String())
}
