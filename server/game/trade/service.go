package trade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var sessionIDCounter int64

func nextSessionID() int64 {
	return atomic.AddInt64(&sessionIDCounter, 1)
}

// TradeOffer is one side of the trade.
type TradeOffer struct {
	CharID int64
	ItemIDs []int64 // inventory IDs being offered
	Gold    int64
	Confirmed bool
}

// TradeSession tracks a pending trade between two players.
type TradeSession struct {
	ID      int64
	OfferA  *TradeOffer
	OfferB  *TradeOffer
	mu      sync.Mutex
}

// Service manages all active trade sessions.
type Service struct {
	db     *gorm.DB
	cache  cache.Cache
	sm     *player.SessionManager
	mu     sync.RWMutex
	active map[int64]*TradeSession // charID → session
	logger *zap.Logger
}

// NewService creates a new trade Service.
func NewService(db *gorm.DB, c cache.Cache, sm *player.SessionManager, logger *zap.Logger) *Service {
	return &Service{
		db:     db,
		cache:  c,
		sm:     sm,
		active: make(map[int64]*TradeSession),
		logger: logger,
	}
}

// RequestTrade initiates a trade request from initiator to target.
func (svc *Service) RequestTrade(initiator, target *player.PlayerSession) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	if _, ok := svc.active[initiator.CharID]; ok {
		return errors.New("already in a trade")
	}
	if _, ok := svc.active[target.CharID]; ok {
		return errors.New("target is in a trade")
	}

	// Notify target.
	payload, _ := json.Marshal(map[string]interface{}{
		"from_id":   initiator.CharID,
		"from_name": initiator.CharName,
	})
	target.Send(&player.Packet{Type: "trade_request", Payload: payload})
	return nil
}

// AcceptTrade creates the trade session when target accepts.
func (svc *Service) AcceptTrade(a, b *player.PlayerSession) *TradeSession {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	sess := &TradeSession{
		ID: nextSessionID(),
		OfferA: &TradeOffer{CharID: a.CharID},
		OfferB: &TradeOffer{CharID: b.CharID},
	}
	svc.active[a.CharID] = sess
	svc.active[b.CharID] = sess
	return sess
}

// UpdateOffer updates one side's offer and notifies both parties.
func (svc *Service) UpdateOffer(s *player.PlayerSession, itemIDs []int64, gold int64) error {
	svc.mu.RLock()
	sess := svc.active[s.CharID]
	svc.mu.RUnlock()
	if sess == nil {
		return errors.New("not in a trade")
	}

	sess.mu.Lock()
	offer := svc.getOffer(sess, s.CharID)
	if offer == nil {
		sess.mu.Unlock()
		return errors.New("invalid trade session")
	}
	offer.ItemIDs = itemIDs
	offer.Gold = gold
	offer.Confirmed = false // reset confirmation on offer change
	sess.mu.Unlock()

	svc.broadcastState(sess, s)
	return nil
}

// Confirm marks one side's offer as confirmed.
func (svc *Service) Confirm(ctx context.Context, s *player.PlayerSession) error {
	svc.mu.RLock()
	sess := svc.active[s.CharID]
	svc.mu.RUnlock()
	if sess == nil {
		return errors.New("not in a trade")
	}

	sess.mu.Lock()
	offer := svc.getOffer(sess, s.CharID)
	if offer != nil {
		offer.Confirmed = true
	}
	bothConfirmed := sess.OfferA.Confirmed && sess.OfferB.Confirmed
	sess.mu.Unlock()

	if bothConfirmed {
		return svc.Commit(ctx, sess)
	}
	svc.broadcastState(sess, s)
	return nil
}

// Cancel cancels the trade for both parties.
func (svc *Service) Cancel(s *player.PlayerSession) {
	svc.mu.Lock()
	sess := svc.active[s.CharID]
	if sess != nil {
		delete(svc.active, sess.OfferA.CharID)
		delete(svc.active, sess.OfferB.CharID)
	}
	svc.mu.Unlock()

	if sess != nil {
		payload, _ := json.Marshal(map[string]string{"reason": "cancelled"})
		pkt, _ := json.Marshal(&player.Packet{Type: "trade_cancel", Payload: payload})
		if svc.sm != nil {
			if a := svc.sm.Get(sess.OfferA.CharID); a != nil {
				a.SendRaw(pkt)
			}
			if b := svc.sm.Get(sess.OfferB.CharID); b != nil {
				b.SendRaw(pkt)
			}
		}
		svc.logger.Info("trade cancelled", zap.Int64("session_id", sess.ID))
	}
}

// Commit atomically executes the trade using a distributed lock + DB transaction.
func (svc *Service) Commit(ctx context.Context, sess *TradeSession) error {
	// Generate lock key (smaller ID first for consistent ordering).
	a, b := sess.OfferA.CharID, sess.OfferB.CharID
	if a > b {
		a, b = b, a
	}
	lockKey := fmt.Sprintf("lock:trade:%d_%d", a, b)

	ok, err := svc.cache.SetNX(ctx, lockKey, "1", 30*time.Second)
	if err != nil || !ok {
		return errors.New("trade in progress, please retry")
	}
	defer svc.cache.Del(ctx, lockKey)

	err = svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Re-validate both sides' items from DB.
		for _, invID := range sess.OfferA.ItemIDs {
			var inv model.Inventory
			if err := tx.Where("id = ? AND char_id = ?", invID, sess.OfferA.CharID).
				First(&inv).Error; err != nil {
				return fmt.Errorf("item %d no longer available for trader A", invID)
			}
			// Transfer to B.
			if err := tx.Model(&inv).Update("char_id", sess.OfferB.CharID).Error; err != nil {
				return err
			}
		}
		for _, invID := range sess.OfferB.ItemIDs {
			var inv model.Inventory
			if err := tx.Where("id = ? AND char_id = ?", invID, sess.OfferB.CharID).
				First(&inv).Error; err != nil {
				return fmt.Errorf("item %d no longer available for trader B", invID)
			}
			if err := tx.Model(&inv).Update("char_id", sess.OfferA.CharID).Error; err != nil {
				return err
			}
		}
		// Transfer gold.
		if sess.OfferA.Gold > 0 {
			if err := tx.Model(&model.Character{}).Where("id = ?", sess.OfferA.CharID).
				Update("gold", gorm.Expr("gold - ?", sess.OfferA.Gold)).Error; err != nil {
				return err
			}
			tx.Model(&model.Character{}).Where("id = ?", sess.OfferB.CharID).
				Update("gold", gorm.Expr("gold + ?", sess.OfferA.Gold))
		}
		if sess.OfferB.Gold > 0 {
			if err := tx.Model(&model.Character{}).Where("id = ?", sess.OfferB.CharID).
				Update("gold", gorm.Expr("gold - ?", sess.OfferB.Gold)).Error; err != nil {
				return err
			}
			tx.Model(&model.Character{}).Where("id = ?", sess.OfferA.CharID).
				Update("gold", gorm.Expr("gold + ?", sess.OfferB.Gold))
		}
		return nil
	})

	// Clean up session.
	svc.mu.Lock()
	delete(svc.active, sess.OfferA.CharID)
	delete(svc.active, sess.OfferB.CharID)
	svc.mu.Unlock()

	if err != nil {
		return err
	}

	// Notify both parties that the trade is done.
	if svc.sm != nil {
		donePayload, _ := json.Marshal(map[string]interface{}{
			"session_id": sess.ID,
			"phase":      "done",
		})
		for _, cid := range []int64{sess.OfferA.CharID, sess.OfferB.CharID} {
			if p := svc.sm.Get(cid); p != nil {
				p.Send(&player.Packet{Type: "trade_update", Payload: donePayload})
			}
		}
	}

	svc.logger.Info("trade committed", zap.Int64("session_id", sess.ID))
	return nil
}

func (svc *Service) getOffer(sess *TradeSession, charID int64) *TradeOffer {
	if sess.OfferA.CharID == charID {
		return sess.OfferA
	}
	if sess.OfferB.CharID == charID {
		return sess.OfferB
	}
	return nil
}

func (svc *Service) broadcastState(sess *TradeSession, sender *player.PlayerSession) {
	if svc.sm == nil {
		return
	}
	// Determine phase.
	phase := "negotiating"
	if sess.OfferA.Confirmed || sess.OfferB.Confirmed {
		phase = "confirming"
	}
	// Send per-player payloads so each side gets their_offer and confirmed status.
	type perPlayer struct {
		charID     int64
		myOffer    *TradeOffer
		theirOffer *TradeOffer
	}
	sides := []perPlayer{
		{sess.OfferA.CharID, sess.OfferA, sess.OfferB},
		{sess.OfferB.CharID, sess.OfferB, sess.OfferA},
	}
	for _, s := range sides {
		p := svc.sm.Get(s.charID)
		if p == nil {
			continue
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"session_id": sess.ID,
			"phase":      phase,
			"their_offer": map[string]interface{}{
				"items": s.theirOffer.ItemIDs,
				"gold":  s.theirOffer.Gold,
			},
			"confirmed": map[string]interface{}{
				"me":   s.myOffer.Confirmed,
				"them": s.theirOffer.Confirmed,
			},
		})
		p.Send(&player.Packet{Type: "trade_update", Payload: payload})
	}
}
