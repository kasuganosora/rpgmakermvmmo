package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
)

const AccountIDKey = "account_id"

// Auth validates the Bearer JWT token and checks the session cache.
func Auth(sec config.SecurityConfig, c cache.Cache) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		header := ctx.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")

		claims, err := ParseToken(tokenStr, sec.JWTSecret)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		// Check session still valid in cache.
		sessionKey := "session:" + tokenStr
		cacheCtx, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
		defer cancel()
		exists, err := c.Exists(cacheCtx, sessionKey)
		if err != nil || !exists {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
			return
		}

		ctx.Set(AccountIDKey, claims.AccountID)
		ctx.Next()
	}
}

// GetAccountID retrieves the authenticated account ID from the Gin context.
func GetAccountID(c *gin.Context) int64 {
	if v, exists := c.Get(AccountIDKey); exists {
		return v.(int64)
	}
	return 0
}
