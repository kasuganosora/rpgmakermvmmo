package player

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SessionManager maintains the registry of all connected PlayerSessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[int64]*PlayerSession // charID → session
	logger   *zap.Logger
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(logger *zap.Logger) *SessionManager {
	return &SessionManager{
		sessions: make(map[int64]*PlayerSession),
		logger:   logger,
	}
}

// Register adds a session. If a previous session exists for the same charID,
// it is closed first (handles duplicate login / reconnect).
func (sm *SessionManager) Register(s *PlayerSession) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if old, ok := sm.sessions[s.CharID]; ok {
		old.Close()
		sm.logger.Info("duplicate session displaced",
			zap.Int64("char_id", s.CharID))
	}
	sm.sessions[s.CharID] = s
	sm.logger.Info("player session registered",
		zap.Int64("char_id", s.CharID),
		zap.Int64("account_id", s.AccountID))
}

// Unregister removes the session for a charID.
func (sm *SessionManager) Unregister(charID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, charID)
	sm.logger.Info("player session unregistered", zap.Int64("char_id", charID))
}

// Get returns the session for a charID, or nil if not found.
func (sm *SessionManager) Get(charID int64) *PlayerSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[charID]
}

// GetByName finds the session for a character by name (case-insensitive).
func (sm *SessionManager) GetByName(name string) *PlayerSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	nameLower := strings.ToLower(name)
	for _, s := range sm.sessions {
		if strings.ToLower(s.CharName) == nameLower {
			return s
		}
	}
	return nil
}

// IsOnline reports whether a character is currently connected.
func (sm *SessionManager) IsOnline(charID int64) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	_, ok := sm.sessions[charID]
	return ok
}

// Count returns the number of currently connected sessions.
func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// All returns a snapshot slice of all current sessions.
func (sm *SessionManager) All() []*PlayerSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	out := make([]*PlayerSession, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		out = append(out, s)
	}
	return out
}

// BroadcastAll sends a raw pre-encoded packet to every connected session.
// Uses non-blocking send to prevent slow connections from blocking the broadcast.
func (sm *SessionManager) BroadcastAll(data []byte) {
	sm.mu.RLock()
	sessions := make([]*PlayerSession, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		sessions = append(sessions, s)
	}
	sm.mu.RUnlock()

	for _, s := range sessions {
		select {
		case s.SendChan <- data:
		default:
			// Channel full, drop packet for this session.
			sm.logger.Warn("broadcast dropped packet for slow client",
				zap.Int64("char_id", s.CharID))
		}
	}
}

// BroadcastToAll sends a packet to every connected session (typed version).
func (sm *SessionManager) BroadcastToAll(pkt *Packet) {
	data, err := json.Marshal(pkt)
	if err != nil {
		sm.logger.Error("failed to marshal broadcast packet", zap.Error(err))
		return
	}
	sm.BroadcastAll(data)
}

// BroadcastSystemMessage sends a system message to all online players.
func (sm *SessionManager) BroadcastSystemMessage(message string) {
	type chatPayload struct {
		Channel string `json:"channel"`
		From    string `json:"from"`
		Message string `json:"message"`
	}
	payload, _ := json.Marshal(chatPayload{
		Channel: "system",
		From:    "系统",
		Message: message,
	})
	sm.BroadcastToAll(&Packet{Type: "chat_message", Payload: payload})
}

// CloseAllSessions gracefully closes all connected sessions.
func (sm *SessionManager) CloseAllSessions() {
	sm.mu.Lock()
	sessions := make([]*PlayerSession, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		sessions = append(sessions, s)
	}
	sm.mu.Unlock()

	sm.logger.Info("closing all sessions", zap.Int("count", len(sessions)))
	for _, s := range sessions {
		s.Close()
	}

	// Wait for all sessions to close (with timeout)
	maxWait := 10 * time.Second
	start := time.Now()
	for time.Since(start) < maxWait {
		sm.mu.RLock()
		count := len(sm.sessions)
		sm.mu.RUnlock()
		if count == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
