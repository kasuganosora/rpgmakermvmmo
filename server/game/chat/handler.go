package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"go.uber.org/zap"
)

const (
	maxMsgLen    = 200
	worldHistory = 200
	worldChannel = "chat:world"
)

// Handler handles all chat WS messages.
type Handler struct {
	cache   cache.Cache
	pubsub  cache.PubSub
	sm      *player.SessionManager
	wm      *world.WorldManager
	gameCfg config.GameConfig
	logger  *zap.Logger
}

// NewHandler creates a new chat Handler.
func NewHandler(c cache.Cache, ps cache.PubSub, sm *player.SessionManager, wm *world.WorldManager, gameCfg config.GameConfig, logger *zap.Logger) *Handler {
	return &Handler{cache: c, pubsub: ps, sm: sm, wm: wm, gameCfg: gameCfg, logger: logger}
}

type chatSendReq struct {
	Channel    string `json:"channel"` // "world"|"party"|"guild"|"whisper"
	Content    string `json:"content"`
	TargetName string `json:"target_name,omitempty"` // for whisper
}

// HandleSend processes a chat_send WS message.
func (h *Handler) HandleSend(ctx context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req chatSendReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	req.Content = strings.TrimSpace(req.Content)
	if len(req.Content) == 0 {
		return nil
	}
	if len([]rune(req.Content)) > maxMsgLen {
		return errors.New("message too long")
	}

	switch req.Channel {
	case "world":
		return h.handleWorldChat(ctx, s, req.Content)
	case "party":
		// TODO: look up party members and send directly
		msg := h.buildMsg("party", s, req.Content)
		msgJSON, _ := json.Marshal(msg)
		s.Send(&player.Packet{Type: "chat_recv", Payload: msgJSON})
		return nil
	case "guild":
		if s.CharID != 0 {
			msg := h.buildMsg("guild", s, req.Content)
			msgJSON, _ := json.Marshal(msg)
			recvPkt, _ := json.Marshal(&player.Packet{Type: "chat_recv", Payload: msgJSON})
			_ = h.pubsub.Publish(ctx, "guild:0", string(recvPkt))
		}
		return nil
	case "whisper":
		return h.handleWhisper(ctx, s, req.Content, req.TargetName)
	default:
		return errors.New("unknown channel")
	}
}

// handleWorldChat routes world chat based on prefix:
//   +text  → global broadcast (with cooldown)
//   !text  → whole map broadcast (prefix stripped)
//   text   → nearby players only (within chat_nearby_range tiles)
func (h *Handler) handleWorldChat(ctx context.Context, s *player.PlayerSession, content string) error {
	if strings.HasPrefix(content, "+") {
		return h.sendGlobal(ctx, s, strings.TrimPrefix(content, "+"))
	}
	if strings.HasPrefix(content, "!") {
		return h.sendMapWide(s, strings.TrimPrefix(content, "!"))
	}
	return h.sendNearby(s, content)
}

// sendGlobal broadcasts to all online players, with cooldown.
func (h *Handler) sendGlobal(ctx context.Context, s *player.PlayerSession, content string) error {
	content = strings.TrimSpace(content)
	if len(content) == 0 {
		return nil
	}

	// Cooldown check.
	cooldown := time.Duration(h.gameCfg.GlobalChatCooldownS) * time.Second
	if cooldown <= 0 {
		cooldown = 3 * time.Minute
	}
	since := s.CheckGlobalChatCooldown()
	if since < cooldown {
		remaining := cooldown - since
		secs := int(remaining.Seconds())
		errMsg := map[string]string{"message": formatCooldown(secs) + " before next global message"}
		errJSON, _ := json.Marshal(errMsg)
		s.Send(&player.Packet{Type: "error", Payload: errJSON})
		return nil
	}
	s.SetGlobalChatCooldown()

	msg := h.buildMsg("world", s, content)
	msg["scope"] = "global"
	msgJSON, _ := json.Marshal(msg)
	recvPkt, _ := json.Marshal(&player.Packet{Type: "chat_recv", Payload: msgJSON})

	// Broadcast to all.
	h.sm.BroadcastAll(recvPkt)
	_ = h.pubsub.Publish(ctx, worldChannel, string(recvPkt))
	_ = h.cache.LPush(ctx, worldChannel, string(recvPkt))
	_ = h.cache.LTrim(ctx, worldChannel, 0, worldHistory-1)
	return nil
}

// sendMapWide sends to all players on the same map.
func (h *Handler) sendMapWide(s *player.PlayerSession, content string) error {
	content = strings.TrimSpace(content)
	if len(content) == 0 {
		return nil
	}

	room := h.wm.Get(s.MapID)
	if room == nil {
		return nil
	}

	msg := h.buildMsg("world", s, content)
	msg["scope"] = "map"
	msgJSON, _ := json.Marshal(msg)
	recvPkt, _ := json.Marshal(&player.Packet{Type: "chat_recv", Payload: msgJSON})
	room.Broadcast(recvPkt)
	return nil
}

// sendNearby sends only to players within chat_nearby_range tiles on the same map.
func (h *Handler) sendNearby(s *player.PlayerSession, content string) error {
	room := h.wm.Get(s.MapID)
	if room == nil {
		return nil
	}

	rangeLimit := h.gameCfg.ChatNearbyRange
	if rangeLimit <= 0 {
		rangeLimit = 10
	}

	sx, sy, _ := s.Position()

	msg := h.buildMsg("world", s, content)
	msg["scope"] = "nearby"
	msgJSON, _ := json.Marshal(msg)
	recvPkt, _ := json.Marshal(&player.Packet{Type: "chat_recv", Payload: msgJSON})

	// Send to nearby players in the same room.
	room.ForEachPlayer(func(p *player.PlayerSession) {
		px, py, _ := p.Position()
		dx := math.Abs(float64(px - sx))
		dy := math.Abs(float64(py - sy))
		if dx <= float64(rangeLimit) && dy <= float64(rangeLimit) {
			p.SendRaw(recvPkt)
		}
	})
	return nil
}

func (h *Handler) handleWhisper(_ context.Context, s *player.PlayerSession, content, targetName string) error {
	target := h.sm.GetByName(targetName)
	if target == nil {
		errPayload, _ := json.Marshal(map[string]string{"message": "player not online"})
		s.Send(&player.Packet{Type: "error", Payload: errPayload})
		return nil
	}
	msg := h.buildMsg("whisper", s, content)
	msgJSON, _ := json.Marshal(msg)
	recvPkt, _ := json.Marshal(&player.Packet{Type: "chat_recv", Payload: msgJSON})
	target.SendRaw(recvPkt)
	s.SendRaw(recvPkt) // echo to sender
	return nil
}

func (h *Handler) buildMsg(channel string, s *player.PlayerSession, content string) map[string]interface{} {
	return map[string]interface{}{
		"channel":   channel,
		"from_id":   s.CharID,
		"from_name": s.CharName,
		"content":   content,
		"ts":        time.Now().UnixMilli(),
	}
}

func formatCooldown(totalSecs int) string {
	m := totalSecs / 60
	sec := totalSecs % 60
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, sec)
	}
	return fmt.Sprintf("%ds", sec)
}

// SendHistory pushes the last N world chat messages to a newly joined player.
func (h *Handler) SendHistory(ctx context.Context, s *player.PlayerSession, count int64) {
	msgs, err := h.cache.LRange(ctx, worldChannel, 0, count-1)
	if err != nil {
		return
	}
	for _, m := range msgs {
		s.SendRaw([]byte(m))
	}
}
