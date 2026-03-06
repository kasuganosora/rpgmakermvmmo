package ws

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================================================================
// EnterInstanceMidEvent tests
// ========================================================================

// TestEnterInstanceMidEvent_SwitchesRoom verifies that a player in a shared
// room is moved to a new instance room for the same map.
func TestEnterInstanceMidEvent_SwitchesRoom(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nop()), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	gh := NewGameHandlers(nil, wm, nil, nil, nop())

	// Player starts in shared room on map 5.
	s := newSession(1, 100)
	s.MapID = 5
	sharedRoom := wm.GetOrCreate(5)
	sharedRoom.AddPlayer(s)
	assert.Equal(t, 1, sharedRoom.PlayerCount())

	// Enter instance.
	gh.EnterInstanceMidEvent(s)

	// Player should now be in an instance room.
	assert.NotZero(t, s.InstanceID, "session should have a non-zero InstanceID")
	instRoom := wm.GetInstance(s.InstanceID)
	require.NotNil(t, instRoom, "instance room should exist")
	assert.True(t, instRoom.IsInstance)
	assert.Equal(t, 5, instRoom.MapID)
	assert.Equal(t, 1, instRoom.PlayerCount(), "player should be in instance room")

	// Player should have been removed from shared room.
	assert.Equal(t, 0, sharedRoom.PlayerCount(), "shared room should be empty")

	// Client should receive instance_enter packet.
	pkt := recvPkt(t, s)
	assert.Equal(t, "instance_enter", pkt.Type)
}

// TestEnterInstanceMidEvent_AlreadyInInstance verifies that calling
// EnterInstanceMidEvent when already in an instance is a no-op.
func TestEnterInstanceMidEvent_AlreadyInInstance(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nop()), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	gh := NewGameHandlers(nil, wm, nil, nil, nop())

	s := newSession(1, 100)
	s.MapID = 5

	// Set up: player already in an instance room.
	instRoom, instID := wm.GetOrCreateInstance(5, 100)
	instRoom.AddPlayer(s)
	s.InstanceID = instID
	origCount := wm.ActiveInstanceCount()

	// Call again — should be a no-op.
	gh.EnterInstanceMidEvent(s)

	assert.Equal(t, instID, s.InstanceID, "InstanceID should remain unchanged")
	assert.Equal(t, origCount, wm.ActiveInstanceCount(), "no new instance should be created")

	// No packet should be sent (channel should be empty).
	select {
	case <-s.SendChan:
		t.Error("no packet should be sent when already in instance")
	default:
		// expected
	}
}

// ========================================================================
// LeaveInstanceMidEvent tests
// ========================================================================

// TestLeaveInstanceMidEvent_ReturnsToShared verifies that a player in an
// instance room is moved back to the shared room.
func TestLeaveInstanceMidEvent_ReturnsToShared(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nop()), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	gh := NewGameHandlers(nil, wm, nil, nil, nop())

	s := newSession(1, 100)
	s.MapID = 5
	s.SetPosition(10, 20, 2)

	// Set up: player in instance room.
	instRoom, instID := wm.GetOrCreateInstance(5, 100)
	instRoom.AddPlayer(s)
	s.InstanceID = instID

	// Ensure shared room exists.
	wm.GetOrCreate(5)

	// Leave instance.
	gh.LeaveInstanceMidEvent(s)

	assert.Equal(t, int64(0), s.InstanceID, "InstanceID should be reset to 0")

	sharedRoom := wm.GetOrCreate(5)
	assert.Equal(t, 1, sharedRoom.PlayerCount(), "player should be in shared room")

	// Client should receive instance_leave packet.
	pkt := recvPkt(t, s)
	assert.Equal(t, "instance_leave", pkt.Type)

	// The instance_leave payload should contain npcs and players.
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(pkt.Payload, &payload))
	assert.Contains(t, payload, "npcs")
	assert.Contains(t, payload, "players")
}

// TestLeaveInstanceMidEvent_NotInInstance verifies that calling
// LeaveInstanceMidEvent when not in an instance is a no-op.
func TestLeaveInstanceMidEvent_NotInInstance(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nop()), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	gh := NewGameHandlers(nil, wm, nil, nil, nop())

	s := newSession(1, 100)
	s.MapID = 5
	s.InstanceID = 0 // not in an instance

	sharedRoom := wm.GetOrCreate(5)
	sharedRoom.AddPlayer(s)

	// Call leave — should be a no-op.
	gh.LeaveInstanceMidEvent(s)

	assert.Equal(t, int64(0), s.InstanceID)
	assert.Equal(t, 1, sharedRoom.PlayerCount(), "player should remain in shared room")

	// No packet should be sent.
	select {
	case <-s.SendChan:
		t.Error("no packet should be sent when not in instance")
	default:
		// expected
	}
}

// TestLeaveInstanceMidEvent_DestroysEmptyInstance verifies that after the last
// player leaves an instance room, the instance is destroyed.
func TestLeaveInstanceMidEvent_DestroysEmptyInstance(t *testing.T) {
	wm := world.NewWorldManager(nil, world.NewGameState(nil, nop()), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	gh := NewGameHandlers(nil, wm, nil, nil, nop())

	s := newSession(1, 100)
	s.MapID = 5

	// Create instance and add player.
	_, instID := wm.GetOrCreateInstance(5, 100)
	instRoom := wm.GetInstance(instID)
	instRoom.AddPlayer(s)
	s.InstanceID = instID

	assert.Equal(t, 1, wm.ActiveInstanceCount())

	// Leave instance.
	gh.LeaveInstanceMidEvent(s)

	// Instance should be destroyed since it's now empty.
	assert.Nil(t, wm.GetInstance(instID), "empty instance should be destroyed after leave")
	assert.Equal(t, 0, wm.ActiveInstanceCount())

	// Drain the packet.
	recvPkt(t, s)
}

// ========================================================================
// EnterMapRoom with instance tag tests
// ========================================================================

// TestEnterMapRoom_InstanceTag verifies that entering a map tagged with
// <instance> in its Note creates an instance room and sets the session
// InstanceID accordingly.
func TestEnterMapRoom_InstanceTag(t *testing.T) {
	db := testutil.SetupTestDB(t)

	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			42: {ID: 42, Note: "<instance>", Width: 10, Height: 10},
		},
		Passability: make(map[int]*resource.PassabilityMap),
	}

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nop()), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	gh := NewGameHandlers(db, wm, nil, res, nop())

	// Create a character in the DB so EnterMapRoom can load it.
	acc := &model.Account{Username: "inst_user", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{
		AccountID: acc.ID, Name: "InstanceHero", ClassID: 1,
		HP: 100, MaxHP: 100, MapID: 42,
	}
	require.NoError(t, db.Create(char).Error)

	s := newSession(acc.ID, char.ID)

	gh.EnterMapRoom(s, 42, 5, 5, 2)

	// Session should be in an instance.
	assert.NotZero(t, s.InstanceID, "InstanceID should be set for instance map")
	assert.Equal(t, 42, s.MapID)

	instRoom := wm.GetInstance(s.InstanceID)
	require.NotNil(t, instRoom)
	assert.True(t, instRoom.IsInstance)
	assert.Equal(t, 42, instRoom.MapID)
	assert.Equal(t, 1, instRoom.PlayerCount())

	// Should receive map_init.
	pkt := recvPkt(t, s)
	assert.Equal(t, "map_init", pkt.Type)
}

// TestEnterMapRoom_NoInstanceTag verifies that entering a normal map (no
// <instance> tag) uses a shared room and leaves InstanceID at 0.
func TestEnterMapRoom_NoInstanceTag(t *testing.T) {
	db := testutil.SetupTestDB(t)

	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			10: {ID: 10, Note: "<RandomPos>", Width: 10, Height: 10},
		},
		Passability: make(map[int]*resource.PassabilityMap),
	}

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nop()), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	gh := NewGameHandlers(db, wm, nil, res, nop())

	acc := &model.Account{Username: "normal_user", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{
		AccountID: acc.ID, Name: "NormalHero", ClassID: 1,
		HP: 100, MaxHP: 100, MapID: 10,
	}
	require.NoError(t, db.Create(char).Error)

	s := newSession(acc.ID, char.ID)

	gh.EnterMapRoom(s, 10, 3, 3, 2)

	assert.Equal(t, int64(0), s.InstanceID, "InstanceID should be 0 for normal map")
	assert.Equal(t, 10, s.MapID)

	// Should be in the shared room.
	sharedRoom := wm.Get(10)
	require.NotNil(t, sharedRoom)
	assert.False(t, sharedRoom.IsInstance)
	assert.Equal(t, 1, sharedRoom.PlayerCount())

	// Should receive map_init.
	pkt := recvPkt(t, s)
	assert.Equal(t, "map_init", pkt.Type)
}

// TestEnterMapRoom_InstanceTag_LeavePreviousInstance verifies that when a
// player in an instance enters a new instance map, they leave the old instance
// first.
func TestEnterMapRoom_InstanceTag_LeavePreviousInstance(t *testing.T) {
	db := testutil.SetupTestDB(t)

	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			42: {ID: 42, Note: "<instance>", Width: 10, Height: 10},
			43: {ID: 43, Note: "<instance>", Width: 10, Height: 10},
		},
		Passability: make(map[int]*resource.PassabilityMap),
	}

	wm := world.NewWorldManager(nil, world.NewGameState(nil, nop()), world.NewGlobalWhitelist(), nil, nop())
	defer wm.StopAll()

	gh := NewGameHandlers(db, wm, nil, res, nop())

	acc := &model.Account{Username: "multi_inst", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)
	char := &model.Character{
		AccountID: acc.ID, Name: "MultiInst", ClassID: 1,
		HP: 100, MaxHP: 100, MapID: 42,
	}
	require.NoError(t, db.Create(char).Error)

	s := newSession(acc.ID, char.ID)

	// Enter first instance map.
	gh.EnterMapRoom(s, 42, 5, 5, 2)
	firstInstID := s.InstanceID
	require.NotZero(t, firstInstID)
	drainChan(s)

	// Enter second instance map.
	gh.EnterMapRoom(s, 43, 3, 3, 2)
	secondInstID := s.InstanceID
	require.NotZero(t, secondInstID)
	assert.NotEqual(t, firstInstID, secondInstID)

	// First instance should be destroyed (player left it, it's empty).
	assert.Nil(t, wm.GetInstance(firstInstID), "old instance should be destroyed")

	// Second instance should exist.
	assert.NotNil(t, wm.GetInstance(secondInstID), "new instance should exist")
}

// ========================================================================
// helpers
// ========================================================================

// recvPkt reads a single packet from the session's SendChan with a timeout.
func recvPkt(t *testing.T, s *player.PlayerSession) *player.Packet {
	t.Helper()
	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		require.NoError(t, json.Unmarshal(data, &pkt))
		return &pkt
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for packet")
		return nil
	}
}

// drainChan drains all pending data from the session's SendChan.
func drainChan(s *player.PlayerSession) {
	for {
		select {
		case <-s.SendChan:
		default:
			return
		}
	}
}
