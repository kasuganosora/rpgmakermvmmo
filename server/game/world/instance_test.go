package world

import (
	"sync"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestParseInstanceConfig(t *testing.T) {
	tests := []struct {
		name string
		note string
		want InstanceConfig
	}{
		{"empty", "", InstanceConfig{}},
		{"no instance tag", "<RandomPos>\n<server:global>", InstanceConfig{}},
		{"solo instance", "<instance>", InstanceConfig{Enabled: true}},
		{"party instance", "<instance:party>", InstanceConfig{Enabled: true, Party: true}},
		{"save instance", "<instance:save>", InstanceConfig{Enabled: true, Save: true}},
		{"party+save", "<instance:party:save>", InstanceConfig{Enabled: true, Party: true, Save: true}},
		{"mixed tags", "<RandomPos>\n<instance:party>\n<server:global>", InstanceConfig{Enabled: true, Party: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInstanceConfig(tt.note)
			if got != tt.want {
				t.Errorf("ParseInstanceConfig(%q) = %+v, want %+v", tt.note, got, tt.want)
			}
		})
	}
}

func TestInstanceRoomLifecycle(t *testing.T) {
	logger := zap.NewNop()
	wm := NewWorldManager(nil, NewGameState(nil, logger), nil, nil, logger)

	// Create instance for player 100 on map 5.
	room1, instID1 := wm.GetOrCreateInstance(5, 100)
	if room1 == nil || instID1 == 0 {
		t.Fatal("expected instance room to be created")
	}
	if !room1.IsInstance || room1.InstanceID != instID1 {
		t.Error("room should be marked as instance")
	}

	// Same owner+map should return the same instance.
	room1b, instID1b := wm.GetOrCreateInstance(5, 100)
	if room1b != room1 || instID1b != instID1 {
		t.Error("expected same instance for same owner+map")
	}

	// Different owner should get a different instance.
	room2, instID2 := wm.GetOrCreateInstance(5, 200)
	if room2 == room1 || instID2 == instID1 {
		t.Error("expected different instance for different owner")
	}

	// GetInstance should work.
	if wm.GetInstance(instID1) != room1 {
		t.Error("GetInstance should return the correct room")
	}

	// ActiveInstanceCount.
	if wm.ActiveInstanceCount() != 2 {
		t.Errorf("expected 2 instances, got %d", wm.ActiveInstanceCount())
	}

	// DestroyInstance when empty.
	wm.DestroyInstance(instID1)
	if wm.GetInstance(instID1) != nil {
		t.Error("instance should be destroyed")
	}
	if wm.ActiveInstanceCount() != 1 {
		t.Errorf("expected 1 instance, got %d", wm.ActiveInstanceCount())
	}

	// DestroyInstance with players should be a no-op.
	sess := &player.PlayerSession{CharID: 200}
	room2.AddPlayer(sess)
	wm.DestroyInstance(instID2)
	if wm.GetInstance(instID2) == nil {
		t.Error("instance with players should not be destroyed")
	}

	// Remove player and destroy.
	room2.RemovePlayer(200)
	wm.DestroyInstance(instID2)
	if wm.GetInstance(instID2) != nil {
		t.Error("empty instance should now be destroyed")
	}
}

func TestGetPlayerRoom(t *testing.T) {
	logger := zap.NewNop()
	wm := NewWorldManager(nil, NewGameState(nil, logger), nil, nil, logger)

	// Create a shared room.
	sharedRoom := wm.GetOrCreate(10)

	// Player in shared room.
	s1 := &player.PlayerSession{CharID: 1, MapID: 10, InstanceID: 0}
	if got := wm.GetPlayerRoom(s1); got != sharedRoom {
		t.Error("expected shared room for player with InstanceID=0")
	}

	// Create instance and assign player.
	instRoom, instID := wm.GetOrCreateInstance(10, 2)
	s2 := &player.PlayerSession{CharID: 2, MapID: 10, InstanceID: instID}
	if got := wm.GetPlayerRoom(s2); got != instRoom {
		t.Error("expected instance room for player with InstanceID > 0")
	}

	// Player with stale InstanceID falls back to shared.
	wm.DestroyInstance(instID)
	if got := wm.GetPlayerRoom(s2); got != sharedRoom {
		t.Error("expected fallback to shared room when instance is gone")
	}
}

// TestInstanceRoomHasIndependentNPCs verifies that two rooms for the same mapID
// have separate NPC instances. Modifying an NPC in the instance room must not
// affect the shared room's NPC list.
func TestInstanceRoomHasIndependentNPCs(t *testing.T) {
	logger := zap.NewNop()
	wm := NewWorldManager(nil, NewGameState(nil, logger), nil, nil, logger)

	// Shared room for map 7.
	sharedRoom := wm.GetOrCreate(7)
	sharedNPCsBefore := sharedRoom.AllNPCs()

	// Instance room for same map 7.
	instRoom, instID := wm.GetOrCreateInstance(7, 500)
	require.NotNil(t, instRoom)
	assert.NotZero(t, instID)

	// Both rooms start with the same (empty) NPC list when no resource loader
	// is provided, but they must be separate slices.
	instNPCs := instRoom.AllNPCs()
	assert.Equal(t, len(sharedNPCsBefore), len(instNPCs),
		"instance and shared rooms should start with same NPC count")

	// Add an NPC to the instance room.
	instRoom.AddNPC(&NPCRuntime{EventID: 42, X: 3, Y: 4})
	assert.Equal(t, len(sharedNPCsBefore), len(sharedRoom.AllNPCs()),
		"adding NPC to instance must not affect shared room")
	assert.Equal(t, len(instNPCs)+1, len(instRoom.AllNPCs()),
		"instance room should have the added NPC")
}

// TestInstanceDifferentMaps verifies that instances for different mapIDs are
// fully independent — different maps, different instance IDs, different rooms.
func TestInstanceDifferentMaps(t *testing.T) {
	logger := zap.NewNop()
	wm := NewWorldManager(nil, NewGameState(nil, logger), nil, nil, logger)

	room5, id5 := wm.GetOrCreateInstance(5, 100)
	room8, id8 := wm.GetOrCreateInstance(8, 100) // same owner, different map

	require.NotNil(t, room5)
	require.NotNil(t, room8)
	assert.NotEqual(t, id5, id8, "different maps should yield different instance IDs")
	assert.NotEqual(t, room5, room8, "different maps should yield different rooms")
	assert.Equal(t, 5, room5.MapID)
	assert.Equal(t, 8, room8.MapID)

	// Each instance is independently retrievable.
	assert.Equal(t, room5, wm.GetInstance(id5))
	assert.Equal(t, room8, wm.GetInstance(id8))

	// Destroying one does not affect the other.
	wm.DestroyInstance(id5)
	assert.Nil(t, wm.GetInstance(id5))
	assert.NotNil(t, wm.GetInstance(id8))
}

// TestStopAllCleansInstances verifies StopAll destroys all shared and instance rooms.
func TestStopAllCleansInstances(t *testing.T) {
	logger := zap.NewNop()
	wm := NewWorldManager(nil, NewGameState(nil, logger), nil, nil, logger)

	wm.GetOrCreate(1)
	wm.GetOrCreate(2)
	wm.GetOrCreateInstance(1, 100)
	wm.GetOrCreateInstance(2, 200)
	wm.GetOrCreateInstance(1, 300)

	assert.Equal(t, 2, wm.ActiveRoomCount())
	assert.Equal(t, 3, wm.ActiveInstanceCount())

	wm.StopAll()

	assert.Equal(t, 0, wm.ActiveRoomCount(), "StopAll should clear all shared rooms")
	assert.Equal(t, 0, wm.ActiveInstanceCount(), "StopAll should clear all instances")
}

// TestGetOrCreateInstance_ConcurrentSafe verifies that concurrent calls to
// GetOrCreateInstance for different owners do not race or corrupt state.
func TestGetOrCreateInstance_ConcurrentSafe(t *testing.T) {
	logger := zap.NewNop()
	wm := NewWorldManager(nil, NewGameState(nil, logger), nil, nil, logger)

	const n = 50
	var wg sync.WaitGroup
	rooms := make([]*MapRoom, n)
	ids := make([]int64, n)

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			room, id := wm.GetOrCreateInstance(10, int64(idx+1))
			rooms[idx] = room
			ids[idx] = id
		}(i)
	}
	wg.Wait()

	// All instances should be unique.
	idSet := make(map[int64]bool, n)
	roomSet := make(map[*MapRoom]bool, n)
	for i := 0; i < n; i++ {
		require.NotNil(t, rooms[i], "room %d should not be nil", i)
		require.NotZero(t, ids[i], "id %d should not be zero", i)
		assert.False(t, idSet[ids[i]], "duplicate instance ID %d", ids[i])
		assert.False(t, roomSet[rooms[i]], "duplicate room pointer for index %d", i)
		idSet[ids[i]] = true
		roomSet[rooms[i]] = true
	}

	assert.Equal(t, n, wm.ActiveInstanceCount())
}
