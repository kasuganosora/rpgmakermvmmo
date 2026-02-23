package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	gskill "github.com/kasuganosora/rpgmakermvmmo/server/game/skill"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/trade"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---- GameHandlers: HandlePing ----

func TestHandlePing_SendsPong(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	gh := NewGameHandlers(nil, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 1)
	raw := makePacket(t, 1, "ping", map[string]interface{}{"ts": int64(12345)})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		require.NoError(t, json.Unmarshal(data, &pkt))
		assert.Equal(t, "pong", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected pong within 200ms")
	}
}

func TestHandlePing_EmptyPayload(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	gh := NewGameHandlers(nil, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 1)
	raw := makePacket(t, 1, "ping", nil)
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		require.NoError(t, json.Unmarshal(data, &pkt))
		assert.Equal(t, "pong", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected pong within 200ms")
	}
}

// ---- GameHandlers: HandleEnterMap ----

func TestHandleEnterMap_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	gh := NewGameHandlers(db, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	acc := &model.Account{Username: "mapuser", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{AccountID: acc.ID, Name: "MapHero", ClassID: 1, HP: 100, MaxHP: 100, MapID: 1}
	require.NoError(t, db.Create(char).Error)

	s := newSession(acc.ID, 0)
	raw := makePacket(t, 1, "enter_map", map[string]interface{}{
		"char_id": char.ID,
		"map_id":  1,
	})
	r.Dispatch(s, raw)

	// map_init should be sent
	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		require.NoError(t, json.Unmarshal(data, &pkt))
		assert.Equal(t, "map_init", pkt.Type)
	case <-time.After(500 * time.Millisecond):
		t.Error("expected map_init within 500ms")
	}
	assert.Equal(t, 1, s.MapID)
	assert.Equal(t, char.ID, s.CharID)
}

func TestHandleEnterMap_InvalidChar(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	gh := NewGameHandlers(db, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 0)
	raw := makePacket(t, 1, "enter_map", map[string]interface{}{
		"char_id": int64(9999),
		"map_id":  1,
	})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		require.NoError(t, json.Unmarshal(data, &pkt))
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(500 * time.Millisecond):
		t.Error("expected error packet")
	}
}

func TestHandleEnterMap_MalformedPayload(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	gh := NewGameHandlers(db, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 0)
	pkt := player.Packet{Seq: 1, Type: "enter_map", Payload: json.RawMessage(`{invalid`)}
	raw, _ := json.Marshal(pkt)
	r.Dispatch(s, raw)
	// Should not panic
}

func TestHandleEnterMap_LeavesOldMap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	gh := NewGameHandlers(db, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	acc := &model.Account{Username: "remapuser", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{AccountID: acc.ID, Name: "ReMapper", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	s := newSession(acc.ID, char.ID)
	s.MapID = 1
	wm.GetOrCreate(1).AddPlayer(s)

	raw := makePacket(t, 1, "enter_map", map[string]interface{}{
		"char_id": char.ID,
		"map_id":  2,
	})
	r.Dispatch(s, raw)

	// Drain send chan
	for len(s.SendChan) > 0 {
		<-s.SendChan
	}
	assert.Equal(t, 2, s.MapID)
}

// ---- GameHandlers: HandleMove ----

func TestHandleMove_ValidMove(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	r := NewRouter(nop())
	gh := NewGameHandlers(nil, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1
	s.SetPosition(5, 5, 2)

	raw := makePacket(t, 1, "player_move", map[string]interface{}{
		"x": 5, "y": 6, "dir": 2, "seq": uint64(1),
	})
	r.Dispatch(s, raw)

	x, y, dir := s.Position()
	assert.Equal(t, 5, x)
	assert.Equal(t, 6, y)
	assert.Equal(t, 2, dir)
}

func TestHandleMove_NotInMap(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	gh := NewGameHandlers(nil, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 999 // non-existent map → room == nil

	raw := makePacket(t, 1, "player_move", map[string]interface{}{
		"x": 1, "y": 1, "dir": 2, "seq": uint64(1),
	})
	r.Dispatch(s, raw)

	// Distance from (0,0) to (1,1) = 2 > 1.3 → speed violation → move_reject
	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "move_reject", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected move_reject for move when not in a map")
	}
}

func TestHandleMove_SpeedHack_Rejected(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	gh := NewGameHandlers(nil, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1
	s.SetPosition(0, 0, 2)

	raw := makePacket(t, 1, "player_move", map[string]interface{}{
		"x": 10, "y": 10, "dir": 2, "seq": uint64(1),
	})
	r.Dispatch(s, raw)

	x, y, _ := s.Position()
	assert.Equal(t, 0, x)
	assert.Equal(t, 0, y)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "move_reject", pkt.Type)
	default:
		t.Error("expected move_reject packet")
	}
}

func TestHandleMove_MalformedPayload(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	gh := NewGameHandlers(nil, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 10)
	pkt := player.Packet{Seq: 1, Type: "player_move", Payload: json.RawMessage(`{bad`)}
	raw, _ := json.Marshal(pkt)
	r.Dispatch(s, raw)
}

// ---- BattleHandlers: HandleAttack ----

func TestHandleAttack_NotInMap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": int64(1), "target_type": "monster",
	})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error packet")
	}
}

func TestHandleAttack_PvPNotEnabled(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": int64(2), "target_type": "player",
	})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error packet")
	}
}

func TestHandleAttack_UnknownTargetType(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": int64(1), "target_type": "npc",
	})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error for unknown target type")
	}
}

func TestHandleAttack_MonsterNotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": int64(9999), "target_type": "monster",
	})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error for monster not found")
	}
}

// ---- BattleHandlers: HandlePickup ----

func TestHandlePickup_NotInMap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)

	raw := makePacket(t, 1, "pickup_item", map[string]interface{}{"drop_id": int64(1)})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error packet")
	}
}

func TestHandlePickup_DropNotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1

	raw := makePacket(t, 1, "pickup_item", map[string]interface{}{"drop_id": int64(9999)})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error packet for drop not found")
	}
}

func TestHandlePickup_TooFar(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)
	room.AddDrop(1, 1, 1, 10, 10) // drop at (10,10)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1
	s.SetPosition(0, 0, 2) // player at (0,0)

	raw := makePacket(t, 1, "pickup_item", map[string]interface{}{"drop_id": int64(1)})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error: too far to pick up")
	}
}

// ---- NPCHandlers ----

func TestHandleNPCInteract_InvalidEventID(t *testing.T) {
	db := testutil.SetupTestDB(t)

	r := NewRouter(nop())
	wm2 := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	npcH := NewNPCHandlers(db, nil, wm2, nop())
	npcH.RegisterHandlers(r)

	s := newSession(1, 1)
	raw := makePacket(t, 1, "npc_interact", map[string]interface{}{"event_id": 0})
	r.Dispatch(s, raw)
	// No panic expected, handler returns nil for invalid event_id
}


// ---- WS Handler: ServeWS early returns ----

func TestServeWS_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	c, _ := cache.NewCache(cache.CacheConfig{})
	sec := config.SecurityConfig{JWTSecret: "secret", JWTTTLH: time.Hour}
	sm := player.NewSessionManager(nop())
	wsRouter := NewRouter(nop())

	h := NewHandler(db, c, sec, sm, nil, nil, nil, wsRouter, nop())
	r := gin.New()
	r.GET("/ws", h.ServeWS)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestServeWS_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	c, _ := cache.NewCache(cache.CacheConfig{})
	sec := config.SecurityConfig{JWTSecret: "secret", JWTTTLH: time.Hour}
	sm := player.NewSessionManager(nop())
	wsRouter := NewRouter(nop())

	h := NewHandler(db, c, sec, sm, nil, nil, nil, wsRouter, nop())
	r := gin.New()
	r.GET("/ws", h.ServeWS)

	req := httptest.NewRequest(http.MethodGet, "/ws?token=notvalid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestServeWS_SessionExpired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	c, _ := cache.NewCache(cache.CacheConfig{})
	sec := config.SecurityConfig{JWTSecret: "secret", JWTTTLH: time.Hour}
	sm := player.NewSessionManager(nop())
	wsRouter := NewRouter(nop())

	h := NewHandler(db, c, sec, sm, nil, nil, nil, wsRouter, nop())
	r := gin.New()
	r.GET("/ws", h.ServeWS)

	token, err := mw.GenerateToken(1, "secret", time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/ws?token="+token, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---- TradeHandlers ----

func TestHandleTrade_TargetOffline(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger, _ := zap.NewDevelopment()
	c, _ := cache.NewCache(cache.CacheConfig{})
	sm := player.NewSessionManager(logger)

	tradeSvc := trade.NewService(db, c, sm, logger)
	r := NewRouter(nop())
	th := NewTradeHandlers(db, tradeSvc, sm, nop())
	th.RegisterHandlers(r)

	s := newSession(1, 10)
	raw := makePacket(t, 1, "trade_request", map[string]interface{}{"target_char_id": int64(9999)})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error for offline target")
	}
}

func TestHandleTradeCancel_NoActiveTrade(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger, _ := zap.NewDevelopment()
	c, _ := cache.NewCache(cache.CacheConfig{})
	sm := player.NewSessionManager(logger)

	tradeSvc := trade.NewService(db, c, sm, logger)
	r := NewRouter(nop())
	th := NewTradeHandlers(db, tradeSvc, sm, nop())
	th.RegisterHandlers(r)

	s := newSession(1, 10)
	raw := makePacket(t, 1, "trade_cancel", nil)
	r.Dispatch(s, raw)
	// Should not panic
}

func TestHandleTradeAccept_InitiatorOffline(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger, _ := zap.NewDevelopment()
	c, _ := cache.NewCache(cache.CacheConfig{})
	sm := player.NewSessionManager(logger)

	tradeSvc := trade.NewService(db, c, sm, logger)
	r := NewRouter(nop())
	th := NewTradeHandlers(db, tradeSvc, sm, nop())
	th.RegisterHandlers(r)

	s := newSession(1, 10)
	raw := makePacket(t, 1, "trade_accept", map[string]interface{}{"from_char_id": int64(9999)})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error for offline initiator")
	}
}

func TestHandleTradeUpdate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger, _ := zap.NewDevelopment()
	c, _ := cache.NewCache(cache.CacheConfig{})
	sm := player.NewSessionManager(logger)

	tradeSvc := trade.NewService(db, c, sm, logger)
	r := NewRouter(nop())
	th := NewTradeHandlers(db, tradeSvc, sm, nop())
	th.RegisterHandlers(r)

	s := newSession(1, 10)
	raw := makePacket(t, 1, "trade_update", map[string]interface{}{
		"item_ids": []int64{1, 2},
		"gold":     int64(100),
	})
	r.Dispatch(s, raw)
	// Should not panic (no active trade, svc.UpdateOffer returns error → replyError)
}

func TestHandleTradeConfirm(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger, _ := zap.NewDevelopment()
	c, _ := cache.NewCache(cache.CacheConfig{})
	sm := player.NewSessionManager(logger)

	tradeSvc := trade.NewService(db, c, sm, logger)
	r := NewRouter(nop())
	th := NewTradeHandlers(db, tradeSvc, sm, nop())
	th.RegisterHandlers(r)

	s := newSession(1, 10)
	raw := makePacket(t, 1, "trade_confirm", nil)
	r.Dispatch(s, raw)
	// Should not panic (no active trade)
}

// ---- SkillItemHandlers ----

func newSkillItemHandlersForTest(t *testing.T) (*SkillItemHandlers, *world.WorldManager) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	c, _ := cache.NewCache(cache.CacheConfig{})
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	t.Cleanup(wm.StopAll)
	skillSvc := gskill.NewSkillService(c, nil, wm, nop())
	sh := NewSkillItemHandlers(db, nil, wm, skillSvc, nop())
	return sh, wm
}

func TestHandleUseSkill_MalformedPayload(t *testing.T) {
	sh, _ := newSkillItemHandlersForTest(t)
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	s := newSession(1, 10)
	pkt := player.Packet{Seq: 1, Type: "player_skill", Payload: json.RawMessage(`{bad`)}
	raw, _ := json.Marshal(pkt)
	r.Dispatch(s, raw)
	// Should not panic
}

func TestHandleUseSkill_NilResources(t *testing.T) {
	sh, _ := newSkillItemHandlersForTest(t)
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	s := newSession(1, 10)
	raw := makePacket(t, 1, "player_skill", map[string]interface{}{
		"skill_id": 1, "target_id": int64(0), "target_type": "monster",
	})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error packet from UseSkill with nil resources")
	}
}

func TestHandleEquipItem_MalformedPayload(t *testing.T) {
	sh, _ := newSkillItemHandlersForTest(t)
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	s := newSession(1, 10)
	pkt := player.Packet{Seq: 1, Type: "equip_item", Payload: json.RawMessage(`{bad`)}
	raw, _ := json.Marshal(pkt)
	r.Dispatch(s, raw)
	// Should not panic
}

func TestHandleEquipItem_NotFound(t *testing.T) {
	sh, _ := newSkillItemHandlersForTest(t)
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	s := newSession(1, 100)
	raw := makePacket(t, 1, "equip_item", map[string]interface{}{"inv_id": int64(9999)})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "equip_result", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected equip_result packet from equip with nonexistent inv")
	}
}

func TestHandleUnequipItem_MalformedPayload(t *testing.T) {
	sh, _ := newSkillItemHandlersForTest(t)
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	s := newSession(1, 10)
	pkt := player.Packet{Seq: 1, Type: "unequip_item", Payload: json.RawMessage(`{bad`)}
	raw, _ := json.Marshal(pkt)
	r.Dispatch(s, raw)
	// Should not panic
}

func TestHandleUnequipItem_NotFound(t *testing.T) {
	sh, _ := newSkillItemHandlersForTest(t)
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	s := newSession(1, 100)
	raw := makePacket(t, 1, "unequip_item", map[string]interface{}{"inv_id": int64(9999)})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "equip_result", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected equip_result packet from unequip with nonexistent inv")
	}
}

func TestHandleUseItem_MalformedPayload(t *testing.T) {
	sh, _ := newSkillItemHandlersForTest(t)
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	s := newSession(1, 10)
	pkt := player.Packet{Seq: 1, Type: "use_item", Payload: json.RawMessage(`{bad`)}
	raw, _ := json.Marshal(pkt)
	r.Dispatch(s, raw)
	// Should not panic
}

func TestHandleUseItem_NotFound(t *testing.T) {
	sh, _ := newSkillItemHandlersForTest(t)
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	s := newSession(1, 100)
	raw := makePacket(t, 1, "use_item", map[string]interface{}{"inv_id": int64(9999)})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "error", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected error packet from use_item with nonexistent inv")
	}
}

func TestHandleUseSkill_Success(t *testing.T) {
	c, _ := cache.NewCache(cache.CacheConfig{})
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	// Build a minimal ResourceLoader with skill 1
	skills := make([]*resource.Skill, 2)
	skills[1] = &resource.Skill{ID: 1, Name: "Fire", MPCost: 5}
	res := &resource.ResourceLoader{Skills: skills}

	db := testutil.SetupTestDB(t)
	skillSvc := gskill.NewSkillService(c, res, wm, nop())
	sh := NewSkillItemHandlers(db, nil, wm, skillSvc, nop())
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1
	s.MP = 50

	raw := makePacket(t, 1, "player_skill", map[string]interface{}{
		"skill_id": 1, "target_id": int64(0), "target_type": "monster",
	})
	r.Dispatch(s, raw)
	// UseSkill succeeds → no sendError sent; give it a moment
	time.Sleep(50 * time.Millisecond)
}

func TestHandleEquipItem_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := cache.NewCache(cache.CacheConfig{})
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	skillSvc := gskill.NewSkillService(c, nil, wm, nop())
	sh := NewSkillItemHandlers(db, nil, wm, skillSvc, nop())
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	acc := &model.Account{Username: "equipper", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{AccountID: acc.ID, Name: "Equipper", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	// Kind=2 (weapon) equips to slot 0 without needing res
	inv := &model.Inventory{CharID: char.ID, ItemID: 1, Kind: 2, Qty: 1, Equipped: false, SlotIndex: -1}
	require.NoError(t, db.Create(inv).Error)

	s := newSession(acc.ID, char.ID)
	raw := makePacket(t, 1, "equip_item", map[string]interface{}{"inv_id": inv.ID})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "equip_result", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected equip_result on successful equip")
	}
}

func TestHandleUnequipItem_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := cache.NewCache(cache.CacheConfig{})
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	skillSvc := gskill.NewSkillService(c, nil, wm, nop())
	sh := NewSkillItemHandlers(db, nil, wm, skillSvc, nop())
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	acc := &model.Account{Username: "unequipper", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{AccountID: acc.ID, Name: "Unequipper", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	// Kind=2 (weapon), already equipped
	inv := &model.Inventory{CharID: char.ID, ItemID: 1, Kind: 2, Qty: 1, Equipped: true, SlotIndex: 0}
	require.NoError(t, db.Create(inv).Error)

	s := newSession(acc.ID, char.ID)
	raw := makePacket(t, 1, "unequip_item", map[string]interface{}{"inv_id": inv.ID})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "equip_result", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected equip_result on successful unequip")
	}
}

func TestHandleUseItem_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := cache.NewCache(cache.CacheConfig{})
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	skillSvc := gskill.NewSkillService(c, nil, wm, nop())
	sh := NewSkillItemHandlers(db, nil, wm, skillSvc, nop())
	r := NewRouter(nop())
	sh.RegisterHandlers(r)

	acc := &model.Account{Username: "itemuser", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{AccountID: acc.ID, Name: "ItemUser", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	// Kind=1 (consumable item)
	inv := &model.Inventory{CharID: char.ID, ItemID: 1, Kind: 1, Qty: 3}
	require.NoError(t, db.Create(inv).Error)

	s := newSession(acc.ID, char.ID)
	raw := makePacket(t, 1, "use_item", map[string]interface{}{"inv_id": inv.ID})
	r.Dispatch(s, raw)
	// Success: logger.Info called, no packet sent
	time.Sleep(50 * time.Millisecond)
}

// ---- BattleHandlers: monster hit and monster death ----

func newSlimeTemplate(hp int) *resource.Enemy {
	return &resource.Enemy{
		ID: 1, Name: "Slime",
		HP: hp, MP: 0,
		Atk: 1, Def: 0, Mat: 1, Mdf: 0, Agi: 1, Luk: 0,
		Exp: 10, Gold: 5,
		DropItems: []resource.EnemyDrop{},
	}
}

func TestHandleAttack_WithSkillID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	// Monster with high HP
	monster := world.NewMonster(newSlimeTemplate(1000), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	// Create ResourceLoader with skill 1
	skills := make([]*resource.Skill, 2)
	skills[1] = &resource.Skill{ID: 1, Name: "Fire", MPCost: 5, Damage: resource.SkillDamage{Type: 1}}
	res := &resource.ResourceLoader{Skills: skills}

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, res, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1
	room.AddPlayer(s)

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster", "skill_id": 1,
	})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "battle_result", pkt.Type)
	case <-time.After(300 * time.Millisecond):
		t.Error("expected battle_result with skill attack")
	}
}

func TestHandleAttack_MonsterHit(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	// Monster with high HP so it survives the hit
	monster := world.NewMonster(newSlimeTemplate(1000), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1
	room.AddPlayer(s) // required so room.Broadcast reaches this session

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster",
	})
	r.Dispatch(s, raw)

	// Should receive a battle_result broadcast
	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "battle_result", pkt.Type)
	case <-time.After(300 * time.Millisecond):
		t.Error("expected battle_result packet")
	}
}

func TestHandleAttack_MonsterDeath(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	// Monster with 1 HP – will die from one normal hit (atk=10)
	monster := world.NewMonster(newSlimeTemplate(1), 1, 5, 5)
	room.AddMonsterRuntime(monster)

	// Create a real character so awardExp goroutine can update it
	acc := &model.Account{Username: "deathtest", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{
		AccountID: acc.ID, Name: "Fighter", ClassID: 1,
		HP: 100, MaxHP: 100, Level: 1,
	}
	require.NoError(t, db.Create(char).Error)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(acc.ID, char.ID)
	s.MapID = 1

	raw := makePacket(t, 1, "attack", map[string]interface{}{
		"target_id": monster.InstID, "target_type": "monster",
	})
	r.Dispatch(s, raw)

	// Give the awardExp goroutine time to run
	time.Sleep(150 * time.Millisecond)

	// Drain the send channel
	for len(s.SendChan) > 0 {
		<-s.SendChan
	}
}

// ---- GameHandlers: HandleMove negative coordinates and passability ----

func TestHandleMove_NegativeDxDy(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	r := NewRouter(nop())
	gh := NewGameHandlers(nil, wm, nil, nil, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1
	s.SetPosition(5, 5, 4)

	// Move left: dx = 4-5 = -1 → if dx < 0 { dx = -dx }; total 1 tile ≤ 1.3, valid
	raw := makePacket(t, 0, "player_move", map[string]interface{}{
		"x": 4, "y": 5, "dir": 4, "seq": uint64(0),
	})
	r.Dispatch(s, raw)
	x, y, _ := s.Position()
	assert.Equal(t, 4, x)
	assert.Equal(t, 5, y)

	// Move up: dy = 4-5 = -1 → if dy < 0 { dy = -dy }; total 1 tile ≤ 1.3, valid
	raw2 := makePacket(t, 0, "player_move", map[string]interface{}{
		"x": 4, "y": 4, "dir": 8, "seq": uint64(0),
	})
	r.Dispatch(s, raw2)
	x, y, _ = s.Position()
	assert.Equal(t, 4, x)
	assert.Equal(t, 4, y)
}

func TestHandleMove_ImpassableTile(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	wm.GetOrCreate(1)

	// PassabilityMap with Width=0,Height=0 → CanPass always returns false (out-of-bounds)
	pm := &resource.PassabilityMap{Width: 0, Height: 0}
	res := &resource.ResourceLoader{
		Maps:        make(map[int]*resource.MapData),
		Passability: map[int]*resource.PassabilityMap{1: pm},
	}

	r := NewRouter(nop())
	gh := NewGameHandlers(nil, wm, nil, res, nop())
	gh.RegisterHandlers(r)

	s := newSession(1, 10)
	s.MapID = 1
	s.SetPosition(5, 5, 2)

	// 1-tile move (within speed limit) to a fully-blocked tile
	raw := makePacket(t, 1, "player_move", map[string]interface{}{
		"x": 5, "y": 6, "dir": 2, "seq": uint64(1),
	})
	r.Dispatch(s, raw)

	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "move_reject", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected move_reject for impassable tile")
	}
}

// ---- NPCHandlers: HandleInteract valid event ----

func TestHandleNPCInteract_ValidEvent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	// Use a res with empty Maps so ExecuteEventByID returns early without panic
	res := &resource.ResourceLoader{
		Maps:        make(map[int]*resource.MapData),
		Passability: make(map[int]*resource.PassabilityMap),
	}

	r := NewRouter(nop())
	wm2 := world.NewWorldManager(res, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	npcH := NewNPCHandlers(db, res, wm2, nop())
	npcH.RegisterHandlers(r)

	s := newSession(1, 1)
	s.MapID = 1
	raw := makePacket(t, 2, "npc_interact", map[string]interface{}{"event_id": 1})
	r.Dispatch(s, raw)
	// Let the goroutine run
	time.Sleep(50 * time.Millisecond)
	// No panic expected
}

// ---- TradeHandlers: online-target / online-initiator paths ----

func TestHandleTradeRequest_TargetOnline(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger, _ := zap.NewDevelopment()
	c, _ := cache.NewCache(cache.CacheConfig{})
	sm := player.NewSessionManager(logger)

	tradeSvc := trade.NewService(db, c, sm, logger)
	r := NewRouter(nop())
	th := NewTradeHandlers(db, tradeSvc, sm, nop())
	th.RegisterHandlers(r)

	// Register an online target with charID=200
	target := newSession(200, 200)
	sm.Register(target)

	s := newSession(1, 10)
	raw := makePacket(t, 1, "trade_request", map[string]interface{}{"target_char_id": int64(200)})
	r.Dispatch(s, raw)

	// RequestTrade sends "trade_request" packet to target
	select {
	case data := <-target.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "trade_request", pkt.Type)
	case <-time.After(200 * time.Millisecond):
		t.Error("expected trade_request packet forwarded to target")
	}
}

func TestHandleTradeAccept_InitiatorOnline(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger, _ := zap.NewDevelopment()
	c, _ := cache.NewCache(cache.CacheConfig{})
	sm := player.NewSessionManager(logger)

	tradeSvc := trade.NewService(db, c, sm, logger)
	r := NewRouter(nop())
	th := NewTradeHandlers(db, tradeSvc, sm, nop())
	th.RegisterHandlers(r)

	// Register an online initiator with charID=300
	initiator := newSession(300, 300)
	sm.Register(initiator)

	s := newSession(1, 10)
	raw := makePacket(t, 1, "trade_accept", map[string]interface{}{"from_char_id": int64(300)})
	r.Dispatch(s, raw)
	// AcceptTrade creates a trade session — no panic expected
	time.Sleep(20 * time.Millisecond)
}

func TestHandlePickup_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	wm := world.NewWorldManager(nil, world.NewGameState(nil), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()
	room := wm.GetOrCreate(1)

	// Add a drop at (1,1)
	room.AddDrop(1, 1, 1, 1, 1)

	// Create character in DB for the inventory insert
	acc := &model.Account{Username: "pickuptest", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{
		AccountID: acc.ID, Name: "Picker", ClassID: 1, HP: 100, MaxHP: 100,
	}
	require.NoError(t, db.Create(char).Error)

	r := NewRouter(nop())
	bh := NewBattleHandlers(db, wm, nil, nop())
	bh.RegisterHandlers(r)

	s := newSession(acc.ID, char.ID)
	s.MapID = 1
	s.SetPosition(1, 1, 2) // same tile as the drop
	room.AddPlayer(s)       // required so room.Broadcast reaches this session

	raw := makePacket(t, 1, "pickup_item", map[string]interface{}{"drop_id": int64(1)})
	r.Dispatch(s, raw)

	// Should receive drop_remove broadcast
	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		json.Unmarshal(data, &pkt)
		assert.Equal(t, "drop_remove", pkt.Type)
	case <-time.After(300 * time.Millisecond):
		t.Error("expected drop_remove packet")
	}
}
