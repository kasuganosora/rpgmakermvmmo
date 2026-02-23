package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// IPWhitelist returns a middleware that only allows requests from specified IPs.
// If the whitelist is empty, all IPs are allowed.
func IPWhitelist(ips []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(ips))
	for _, ip := range ips {
		allowed[ip] = true
	}
	return func(c *gin.Context) {
		if len(allowed) == 0 {
			c.Next()
			return
		}
		if !allowed[c.ClientIP()] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
		c.Next()
	}
}
