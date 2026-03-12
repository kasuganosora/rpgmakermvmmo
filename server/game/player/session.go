package player

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
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
	MapID      int
	InstanceID int64 // 0 = shared room, >0 = private/party instance ID
	X, Y       int
	Dir        int // RPG Maker directions: 2=down 4=left 6=right 8=up
	HP, MaxHP int
	MP, MaxMP int
	Level     int
	Exp       int64
	States    map[int]bool // active states (state ID → true)
	Equips    map[int]int  // equipped items (slot index → item ID)

	// ShopGoods holds the goods list for the currently open shop.
	// Set by executor when sending ShopProcessing; cleared on shop close.
	// Format: each entry is [type, id, priceType, price] (RMMV goods format).
	// type: 0=item, 1=weapon, 2=armor.
	ShopGoods [][]interface{}

	SendChan     chan []byte
	Done         chan struct{}
	ChoiceCh      chan int      // receives choice index from npc_choice_reply
	DialogAckCh   chan struct{} // receives ack when client finishes displaying a dialog
	EffectAckCh   chan struct{} // receives ack when client finishes playing a visual effect
	SceneReadyCh  chan struct{} // receives signal when client Scene_Map is fully loaded
	TraceID      string
	LastSeq      uint64
	Dirty          bool // position changed this tick
	LastResetPos   time.Time
	LastGlobalChat time.Time
	LastTransfer   time.Time // set when entering a new map; moves ignored during grace period

	mu           sync.Mutex
	EventMu      sync.Mutex // 序列化每个玩家的事件执行（与 mu 分离：EventMu 持有秒级，mu 持有微秒级）
	inBattle     int32      // atomic: 1 = in battle, 0 = not
	mapGen       uint64     // incremented on each map entry; used to cancel stale autorun goroutines
	needEventEnd bool       // set when event transferred player; autorun should send event_end

	// Field combat: last attack timestamp for GCD enforcement.
	lastAttackTime time.Time

	// 反作弊：速度异常计数。每次检测到 speed hack 时 +1，
	// 每 speedHackWindow 重置。累计超过 speedHackKickThreshold 则断开连接。
	speedHackCount int
	speedHackReset time.Time

	// 反作弊：WS 消息频率限制（令牌桶）。
	// 每秒补充 rateLimitRefill 个令牌，最多持有 rateLimitBurst 个。
	rateBucket   int
	rateLastTime time.Time

	// 反作弊：NPC 交互冷却。防止快速连点触发多次事件。
	lastInteract time.Time

	logger       *zap.Logger
}

const (
	speedHackKickThreshold = 10               // 窗口内最大容忍次数
	speedHackWindow        = 60 * time.Second  // 计数重置周期

	rateLimitRefill = 30  // 每秒补充令牌数
	rateLimitBurst  = 60  // 令牌桶容量（允许短暂突发）

	interactCooldown = 300 * time.Millisecond // NPC 交互最小间隔
)

// RateLimit checks whether this message should be dropped due to rate limiting.
// Returns true if the message is allowed, false if it should be dropped.
// Uses a token bucket algorithm: rateLimitRefill tokens per second, up to rateLimitBurst.
func (s *PlayerSession) RateLimit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.rateLastTime.IsZero() {
		s.rateLastTime = now
		s.rateBucket = rateLimitBurst
	}
	// Refill tokens based on elapsed time.
	elapsed := now.Sub(s.rateLastTime).Seconds()
	s.rateLastTime = now
	s.rateBucket += int(elapsed * float64(rateLimitRefill))
	if s.rateBucket > rateLimitBurst {
		s.rateBucket = rateLimitBurst
	}
	if s.rateBucket <= 0 {
		return false
	}
	s.rateBucket--
	return true
}

// CheckInteractCooldown returns true if enough time has passed since last interaction.
func (s *PlayerSession) CheckInteractCooldown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.lastInteract) < interactCooldown {
		return false
	}
	s.lastInteract = time.Now()
	return true
}

// RecordSpeedHack increments the speed hack counter and returns true
// if the player should be kicked (exceeded threshold within window).
func (s *PlayerSession) RecordSpeedHack() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if now.Sub(s.speedHackReset) > speedHackWindow {
		s.speedHackCount = 0
		s.speedHackReset = now
	}
	s.speedHackCount++
	return s.speedHackCount >= speedHackKickThreshold
}

// SetLogger sets the session's logger. Useful for tests that construct
// sessions without NewPlayerSession.
func (s *PlayerSession) SetLogger(l *zap.Logger) {
	s.logger = l
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
		EffectAckCh:  make(chan struct{}, 1),
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

// HasState returns whether the player has the given state active.
func (s *PlayerSession) HasState(stateID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.States[stateID]
}

// AddState adds a state to the player.
func (s *PlayerSession) AddState(stateID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.States == nil {
		s.States = make(map[int]bool)
	}
	s.States[stateID] = true
}

// RemoveState removes a state from the player.
func (s *PlayerSession) RemoveState(stateID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.States, stateID)
}

// ClearStates removes all states (used by RecoverAll).
func (s *PlayerSession) ClearStates() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.States = nil
}

// StatesSnapshot returns a copy of the active states map for safe iteration.
func (s *PlayerSession) StatesSnapshot() map[int]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[int]bool, len(s.States))
	for k, v := range s.States {
		cp[k] = v
	}
	return cp
}

// SetEquip sets an equipped item for a slot.
func (s *PlayerSession) SetEquip(slotIndex, itemID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Equips == nil {
		s.Equips = make(map[int]int)
	}
	s.Equips[slotIndex] = itemID
}

// GetEquip returns the item ID for a slot (0 if empty).
func (s *PlayerSession) GetEquip(slotIndex int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Equips[slotIndex]
}

// EquipsSnapshot returns a copy of the equips map for safe iteration.
func (s *PlayerSession) EquipsSnapshot() map[int]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[int]int, len(s.Equips))
	for k, v := range s.Equips {
		cp[k] = v
	}
	return cp
}

// SetMapInfo atomically sets MapID and InstanceID together.
func (s *PlayerSession) SetMapInfo(mapID int, instanceID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MapID = mapID
	s.InstanceID = instanceID
}

// GetMapInfo returns MapID and InstanceID atomically.
func (s *PlayerSession) GetMapInfo() (mapID int, instanceID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.MapID, s.InstanceID
}

// GetMapID returns MapID thread-safely.
func (s *PlayerSession) GetMapID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.MapID
}

// SetLevel sets the player's level thread-safely.
func (s *PlayerSession) SetLevel(level int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Level = level
}

// GetLevel returns the player's level thread-safely.
func (s *PlayerSession) GetLevel() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Level
}

// SetExp sets the player's experience points thread-safely.
func (s *PlayerSession) SetExp(exp int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Exp = exp
}

// GetExp returns the player's experience points thread-safely.
func (s *PlayerSession) GetExp() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Exp
}

// SetClassID sets the player's class ID thread-safely.
func (s *PlayerSession) SetClassID(classID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ClassID = classID
}

// GetClassID returns the player's class ID thread-safely.
func (s *PlayerSession) GetClassID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ClassID
}

// SetCharName updates the character name thread-safely.
func (s *PlayerSession) SetCharName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CharName = name
}

// SetActorImages updates character/face images thread-safely.
func (s *PlayerSession) SetActorImages(walkName string, walkIndex int, faceName string, faceIndex int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.WalkName = walkName
	s.WalkIndex = walkIndex
	s.FaceName = faceName
	s.FaceIndex = faceIndex
}

// ActorImages returns the current actor image info thread-safely.
func (s *PlayerSession) ActorImages() (walkName string, walkIndex int, faceName string, faceIndex int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.WalkName, s.WalkIndex, s.FaceName, s.FaceIndex
}

// SetShopGoods stores the shop goods list thread-safely.
func (s *PlayerSession) SetShopGoods(goods [][]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ShopGoods = goods
}

// GetShopGoods retrieves the shop goods list thread-safely.
func (s *PlayerSession) GetShopGoods() [][]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ShopGoods
}

// SetLastTransfer records the current time as the last map-transfer time, thread-safely.
func (s *PlayerSession) SetLastTransfer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastTransfer = time.Now()
}

// CheckTransferCooldown returns true when the transfer grace period has elapsed.
func (s *PlayerSession) CheckTransferCooldown(grace time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.LastTransfer) >= grace
}

// CoreSnapshot returns HP/MaxHP/MP/MaxMP/Level/Exp/ClassID/MapID as a single locked read.
// Used by handleDisconnect to safely persist session state.
func (s *PlayerSession) CoreSnapshot() (hp, maxHP, mp, maxMP, level int, exp int64, classID, mapID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.HP, s.MaxHP, s.MP, s.MaxMP, s.Level, s.Exp, s.ClassID, s.MapID
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

// IncrMapGen increments the map generation counter and returns the new value.
// Called when the player enters a new map. Used to cancel stale autorun goroutines.
func (s *PlayerSession) IncrMapGen() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mapGen++
	return s.mapGen
}

// GetMapGen returns the current map generation counter.
func (s *PlayerSession) GetMapGen() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mapGen
}

// SetNeedEventEnd marks that the current event transferred the player
// and the subsequent autorun goroutine should send event_end.
func (s *PlayerSession) SetNeedEventEnd(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needEventEnd = v
}

// ClearNeedEventEnd atomically reads and clears needEventEnd.
// Returns true if the flag was set (autorun should send event_end).
func (s *PlayerSession) ClearNeedEventEnd() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.needEventEnd
	s.needEventEnd = false
	return v
}

// InBattle returns true if the player is currently in a battle.
func (s *PlayerSession) InBattle() bool {
	return atomic.LoadInt32(&s.inBattle) == 1
}

// SetInBattle sets or clears the in-battle flag atomically.
func (s *PlayerSession) SetInBattle(v bool) {
	if v {
		atomic.StoreInt32(&s.inBattle, 1)
	} else {
		atomic.StoreInt32(&s.inBattle, 0)
	}
}

// GetContext returns a background context (convenience helper).
func (s *PlayerSession) GetContext() context.Context {
	return context.Background()
}

// CheckAttackGCD returns true if enough time has passed since last attack.
// If allowed, updates the timestamp atomically.
func (s *PlayerSession) CheckAttackGCD(gcdMs int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.lastAttackTime) < time.Duration(gcdMs)*time.Millisecond {
		return false
	}
	s.lastAttackTime = time.Now()
	return true
}

// ApplyDamage subtracts dmg from HP (clamped to 0). Returns new HP and whether the player died.
func (s *PlayerSession) ApplyDamage(dmg int) (newHP int, dead bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HP -= dmg
	if s.HP < 0 {
		s.HP = 0
	}
	return s.HP, s.HP == 0
}

// Revive sets HP to the given value (clamped to MaxHP).
func (s *PlayerSession) Revive(hp int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if hp > s.MaxHP {
		hp = s.MaxHP
	}
	if hp < 1 {
		hp = 1
	}
	s.HP = hp
}

// ConsumeMP attempts to deduct cost from MP. Returns false if not enough MP.
func (s *PlayerSession) ConsumeMP(cost int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.MP < cost {
		return false
	}
	s.MP -= cost
	return true
}

// IsDead returns true if HP is 0 and MaxHP > 0 (i.e., stats have been initialized).
// A session with HP=0 and MaxHP=0 is considered uninitialized, not dead.
func (s *PlayerSession) IsDead() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.HP <= 0 && s.MaxHP > 0
}

// ClearNPCChannels drains and clears the NPC-related channels.
// Should be called when player enters a new map to prevent stale signals.
func (s *PlayerSession) ClearNPCChannels() {
	select {
	case <-s.DialogAckCh:
	default:
	}
	select {
	case <-s.EffectAckCh:
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
