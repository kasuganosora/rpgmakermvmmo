package rest

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthHandler handles authentication REST endpoints.
type AuthHandler struct {
	db    *gorm.DB
	cache cache.Cache
	sec   config.SecurityConfig
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db *gorm.DB, c cache.Cache, sec config.SecurityConfig) *AuthHandler {
	return &AuthHandler{db: db, cache: c, sec: sec}
}

type loginRequest struct {
	Username string `json:"username" binding:"required,min=2,max=32"`
	Password string `json:"password" binding:"required,min=4,max=64"`
}

// Login handles POST /api/auth/login.
// Auto-registers on first login if the username does not exist.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var acc model.Account
	err := h.db.Where("username = ?", req.Username).First(&acc).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Auto-register
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		acc = model.Account{
			Username:     req.Username,
			PasswordHash: string(hash),
			Status:       1,
		}
		if createErr := h.db.Create(&acc).Error; createErr != nil {
			// Unique constraint violation: another goroutine registered same name.
			if isUniqueViolation(createErr) {
				c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
			}
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	} else {
		// Existing account: verify password
		if err := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(req.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		if acc.Status == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "account banned"})
			return
		}
	}

	token, err := mw.GenerateToken(acc.ID, h.sec.JWTSecret, h.sec.JWTTTLH)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
		return
	}

	// Store session in cache as a simple KV entry so Exists() works uniformly.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	sessionKey := "session:" + token
	_ = h.cache.Set(ctx, sessionKey, strconv.FormatInt(acc.ID, 10), h.sec.JWTTTLH)

	// Update last login (best-effort).
	now := time.Now()
	ip := c.ClientIP()
	_ = h.db.Model(&acc).Updates(map[string]interface{}{
		"last_login_at": now,
		"last_login_ip": ip,
	})

	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"account_id": acc.ID,
	})
}

// Logout handles POST /api/auth/logout.
func (h *AuthHandler) Logout(c *gin.Context) {
	header := c.GetHeader("Authorization")
	tokenStr := strings.TrimPrefix(header, "Bearer ")
	if tokenStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing token"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	_ = h.cache.Del(ctx, "session:"+tokenStr)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// Refresh handles POST /api/auth/refresh.
func (h *AuthHandler) Refresh(c *gin.Context) {
	accountID := mw.GetAccountID(c)
	if accountID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Invalidate old token
	header := c.GetHeader("Authorization")
	oldToken := strings.TrimPrefix(header, "Bearer ")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	_ = h.cache.Del(ctx, "session:"+oldToken)

	// Issue new token
	newToken, err := mw.GenerateToken(accountID, h.sec.JWTSecret, h.sec.JWTTTLH)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
		return
	}
	sessionKey := "session:" + newToken
	_ = h.cache.Set(ctx, sessionKey, strconv.FormatInt(accountID, 10), h.sec.JWTTTLH)

	c.JSON(http.StatusOK, gin.H{"token": newToken})
}

// isUniqueViolation detects duplicate-key errors from common database drivers.
func isUniqueViolation(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "already exists")
}
