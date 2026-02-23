package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimit provides per-IP token-bucket rate limiting.
// r = requests per second, b = burst size.
func RateLimit(r rate.Limit, b int) gin.HandlerFunc {
	limiters := &sync.Map{}

	// Cleanup goroutine: remove stale entries every 5 minutes.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-10 * time.Minute)
			limiters.Range(func(k, v interface{}) bool {
				if v.(*ipLimiter).lastSeen.Before(cutoff) {
					limiters.Delete(k)
				}
				return true
			})
		}
	}()

	getLimiter := func(ip string) *rate.Limiter {
		v, _ := limiters.LoadOrStore(ip, &ipLimiter{limiter: rate.NewLimiter(r, b)})
		il := v.(*ipLimiter)
		il.lastSeen = time.Now()
		return il.limiter
	}

	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !getLimiter(ip).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}
