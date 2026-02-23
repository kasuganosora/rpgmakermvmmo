package rest

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RankingHandler handles leaderboard REST endpoints.
type RankingHandler struct {
	db     *gorm.DB
	cache  cache.Cache
	logger *zap.Logger
}

// NewRankingHandler creates a RankingHandler.
func NewRankingHandler(db *gorm.DB, c cache.Cache, logger *zap.Logger) *RankingHandler {
	return &RankingHandler{db: db, cache: c, logger: logger}
}

const rankingZKey = "ranking:exp"
const rankingTop = 100

// RankEntry is one row in the leaderboard.
type RankEntry struct {
	Rank     int    `json:"rank"`
	CharID   int64  `json:"char_id"`
	CharName string `json:"char_name"`
	Level    int    `json:"level"`
	Exp      int64  `json:"exp"`
}

// TopExp returns the top players sorted by experience.
// GET /api/ranking/exp?limit=20
func (h *RankingHandler) TopExp(c *gin.Context) {
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= rankingTop {
		limit = l
	}

	// Try cached ranking from sorted set.
	ctx := c.Request.Context()
	members, err := h.cache.ZRevRange(ctx, rankingZKey, 0, int64(limit-1))
	if err == nil && len(members) > 0 {
		entries := make([]RankEntry, 0, len(members))
		for i, m := range members {
			charID, err := strconv.ParseInt(m, 10, 64)
			if err != nil {
				continue
			}
			score, _ := h.cache.ZScore(ctx, rankingZKey, m)
			entries = append(entries, RankEntry{
				Rank:   i + 1,
				CharID: charID,
				Exp:    int64(score),
			})
		}
		// Enrich with character names.
		h.enrichNames(entries)
		c.JSON(http.StatusOK, gin.H{"ranking": entries})
		return
	}

	// Fall back to DB query.
	var chars []model.Character
	h.db.Select("id, name, level, exp").
		Order("exp DESC").
		Limit(limit).
		Find(&chars)

	entries := make([]RankEntry, len(chars))
	for i, ch := range chars {
		entries[i] = RankEntry{
			Rank:     i + 1,
			CharID:   ch.ID,
			CharName: ch.Name,
			Level:    ch.Level,
			Exp:      ch.Exp,
		}
		// Refresh cache entry.
		_ = h.cache.ZAdd(ctx, rankingZKey, float64(ch.Exp), strconv.FormatInt(ch.ID, 10))
	}
	c.JSON(http.StatusOK, gin.H{"ranking": entries})
}

// RefreshRanking rebuilds the ranking sorted set from the DB.
// Called periodically by the scheduler; also exposed as POST /api/admin/ranking/refresh.
func (h *RankingHandler) RefreshRanking(c *gin.Context) {
	var chars []model.Character
	if err := h.db.Select("id, exp").Order("exp DESC").Limit(rankingTop).Find(&chars).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	ctx := c.Request.Context()
	for _, ch := range chars {
		_ = h.cache.ZAdd(ctx, rankingZKey, float64(ch.Exp), strconv.FormatInt(ch.ID, 10))
	}
	c.JSON(http.StatusOK, gin.H{"refreshed": len(chars)})
}

func (h *RankingHandler) enrichNames(entries []RankEntry) {
	if len(entries) == 0 {
		return
	}
	ids := make([]int64, len(entries))
	for i, e := range entries {
		ids[i] = e.CharID
	}
	var chars []model.Character
	h.db.Select("id, name, level, exp").Where("id IN ?", ids).Find(&chars)
	nameMap := make(map[int64]model.Character, len(chars))
	for _, ch := range chars {
		nameMap[ch.ID] = ch
	}
	for i := range entries {
		if ch, ok := nameMap[entries[i].CharID]; ok {
			entries[i].CharName = ch.Name
			entries[i].Level = ch.Level
			entries[i].Exp = ch.Exp
		}
	}
}
