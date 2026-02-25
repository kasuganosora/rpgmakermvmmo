package player

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	sendChanBuf    = 256
	writeDeadline  = 10 * time.Second
	readDeadlineS  = 60 * time.Second
	pingInterval   = 30 * time.Second // server-side WS ping
)

// Packet is the unified WS message envelope.
type Packet struct {
	Seq     uint64          `json:"seq"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// PlayerSession represents a connected player's WebSocket session.
type PlayerSession struct {
	PlayerID  int64
	AccountID int64
	CharID    int64
	CharName  string
	WalkName  string
	WalkIndex int
	FaceName  string
	FaceIndex int
	ClassID   int

	Conn     *websocket.Conn
	MapID    int
	X, Y     int
	Dir      int // RPG Maker directions: 2=down 4=left 6=right 8=up
	HP, MaxHP int
	MP, MaxMP int
	Level     int
	Exp       int64

	SendChan     chan []byte
	Done         chan struct{}
	ChoiceCh      chan int      // receives choice index from npc_choice_reply
	DialogAckCh   chan struct{} // receives ack when client finishes displaying a dialog
	SceneReadyCh  chan struct{} // receives signal when client Scene_Map is fully loaded
	TraceID      string
	LastSeq      uint64
	Dirty          bool // position changed this tick
	LastResetPos   time.Time
	LastGlobalChat time.Time
	LastTransfer   time.Time // set when entering a new map; moves ignored during grace period

	mu     sync.Mutex
	logger *zap.Logger
}

// NewPlayerSession creates a new PlayerSession with write goroutine started.
func NewPlayerSession(accountID, charID int64, conn *websocket.Conn, logger *zap.Logger) *PlayerSession {
	s := &PlayerSession{
		AccountID: accountID,
		CharID:    charID,
		Conn:      conn,
		SendChan:     make(chan []byte, sendChanBuf),
		Done:         make(chan struct{}),
		ChoiceCh:     make(chan int, 1),
		DialogAckCh:  make(chan struct{}, 1),
		SceneReadyCh: make(chan struct{}, 1),
		logger:    logger,
	}
	go s.writePump()
	return s
}

// writePump drains SendChan and writes to the WebSocket connection.
// Also sends periodic WebSocket pings to detect dead connections quickly.
func (s *PlayerSession) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	defer s.Conn.Close()
	for {
		select {
		case data, ok := <-s.SendChan:
			if !ok {
				return
			}
			_ = s.Conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err := s.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				s.logger.Warn("ws write error",
					zap.Int64("account_id", s.AccountID),
					zap.Error(err))
				return
			}
		case <-ticker.C:
			_ = s.Conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err := s.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-s.Done:
			_ = s.Conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
	}
}

// Send encodes pkt and sends it non-blocking. Drops if channel full or closed.
func (s *PlayerSession) Send(pkt *Packet) {
	// Skip if session is already closed
	if s.IsClosed() {
		return
	}
	data, err := json.Marshal(pkt)
	if err != nil {
		return
	}
	select {
	case s.SendChan <- data:
	case <-s.Done:
		// Session closed while sending
	default:
		// Only log if not closed (to avoid spam on normal disconnect)
		if !s.IsClosed() {
			s.logger.Warn("send channel full, dropping packet",
				zap.Int64("account_id", s.AccountID),
				zap.String("type", pkt.Type))
		}
	}
}

// SendRaw sends raw bytes non-blocking. Drops if channel full or closed.
func (s *PlayerSession) SendRaw(data []byte) {
	// Skip if session is already closed
	if s.IsClosed() {
		return
	}
	select {
	case s.SendChan <- data:
	case <-s.Done:
		// Session closed while sending
	default:
		// Only log if not closed (to avoid spam on normal disconnect)
		if !s.IsClosed() {
			s.logger.Warn("send channel full, dropping raw packet",
				zap.Int64("account_id", s.AccountID))
		}
	}
}

// Close signals the writePump to shut down.
func (s *PlayerSession) Close() {
	select {
	case <-s.Done:
	default:
		close(s.Done)
	}
}

// IsClosed returns true if the session has been closed.
func (s *PlayerSession) IsClosed() bool {
	select {
	case <-s.Done:
		return true
	default:
		return false
	}
}

// SetPosition updates the session position fields thread-safely.
func (s *PlayerSession) SetPosition(x, y, dir int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.X = x
	s.Y = y
	s.Dir = dir
	s.Dirty = true
}

// Position returns the current position thread-safely.
func (s *PlayerSession) Position() (x, y, dir int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.X, s.Y, s.Dir
}

// SetStats updates HP/MaxHP/MP/MaxMP thread-safely.
func (s *PlayerSession) SetStats(hp, maxHP, mp, maxMP int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HP = hp
	s.MaxHP = maxHP
	s.MP = mp
	s.MaxMP = maxMP
}

// Stats returns HP/MaxHP/MP/MaxMP thread-safely.
func (s *PlayerSession) Stats() (hp, maxHP, mp, maxMP int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.HP, s.MaxHP, s.MP, s.MaxMP
}

// ResetDirty clears the dirty flag and returns whether it was set.
func (s *PlayerSession) ResetDirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.Dirty
	s.Dirty = false
	return d
}

// SendHeartbeatPong sends a pong packet in response to a client ping.
func (s *PlayerSession) SendHeartbeatPong(clientTS int64) {
	type pongPayload struct {
		ClientTS int64 `json:"client_ts"`
		ServerTS int64 `json:"server_ts"`
	}
	payload, _ := json.Marshal(pongPayload{
		ClientTS: clientTS,
		ServerTS: time.Now().UnixMilli(),
	})
	s.Send(&Packet{Type: "pong", Payload: payload})
}

// SetReadDeadline resets the WebSocket read deadline to 60 s from now.
func (s *PlayerSession) SetReadDeadline() {
	_ = s.Conn.SetReadDeadline(time.Now().Add(readDeadlineS))
}

// CheckResetPosCooldown returns the time since the last reset_pos.
func (s *PlayerSession) CheckResetPosCooldown() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.LastResetPos)
}

// SetResetPosCooldown marks the current time as last reset_pos usage.
func (s *PlayerSession) SetResetPosCooldown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastResetPos = time.Now()
}

// CheckGlobalChatCooldown returns the time since the last global chat message.
func (s *PlayerSession) CheckGlobalChatCooldown() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.LastGlobalChat)
}

// SetGlobalChatCooldown marks the current time as last global chat usage.
func (s *PlayerSession) SetGlobalChatCooldown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastGlobalChat = time.Now()
}

// GetContext returns a background context (convenience helper).
func (s *PlayerSession) GetContext() context.Context {
	return context.Background()
}

// ClearNPCChannels drains and clears the NPC-related channels.
// Should be called when player enters a new map to prevent stale signals.
func (s *PlayerSession) ClearNPCChannels() {
	select {
	case <-s.DialogAckCh:
	default:
	}
	select {
	case <-s.ChoiceCh:
	default:
	}
	select {
	case <-s.SceneReadyCh:
	default:
	}
}
