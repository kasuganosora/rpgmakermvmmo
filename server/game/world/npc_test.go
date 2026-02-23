package world

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---- Helpers ----

// newTestRoom creates a MapRoom for testing with a passability map.
func newTestRoom(t *testing.T, mapID, width, height int) *MapRoom {
	t.Helper()
	pm := resource.NewPassabilityMap(width, height)
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			mapID: {ID: mapID, Width: width, Height: height},
		},
		Passability: map[int]*resource.PassabilityMap{
			mapID: pm,
		},
	}
	state := NewGameState(nil)
	return &MapRoom{
		MapID:           mapID,
		mapWidth:        width,
		mapHeight:       height,
		passMap:         pm,
		res:             res,
		state:           state,
		npcs:            []*NPCRuntime{},
		players:         make(map[int64]*player.PlayerSession),
		runtimeMonsters: make(map[int64]*MonsterRuntime),
		drops:           make(map[int64]*DropRuntimeEntry),
		broadcastQ:      make(chan []byte, 16),
		stopCh:          make(chan struct{}),
		logger:          zap.NewNop(),
	}
}

// newNPC creates an NPCRuntime with sensible defaults for testing.
func newNPC(eventID, x, y int, page *resource.EventPage) *NPCRuntime {
	return &NPCRuntime{
		EventID:    eventID,
		Name:       "TestNPC",
		X:          x,
		Y:          y,
		Dir:        2,
		ActivePage: page,
		MapEvent: &resource.MapEvent{
			ID:    eventID,
			Name:  "TestNPC",
			X:     x,
			Y:     y,
			Pages: []*resource.EventPage{page},
		},
	}
}

// ========================================================================
// Tests for GetTransferAt — the "green arrow door" bug
// ========================================================================

// Scenario: Event 20 on Map 5 (自室/My Room)
// - Page 1 (no conditions, ACTIVE): trigger=1, commands=ShowText("too tired")
// - Page 2 (Switch 306): trigger=1, commands=Transfer(201) to Map 1
// When the player steps on Event 20, GetTransferAt should NOT auto-transfer
// because the ACTIVE page (page 1) has dialog, not a transfer.

func TestGetTransferAt_ActivePage_HasDialogNoTransfer_ReturnsNil(t *testing.T) {
	room := newTestRoom(t, 5, 30, 36)

	// Active page: trigger=1, dialog only (no transfer command)
	dialogPage := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 101, Parameters: []interface{}{"", 0, 0, 2}},       // ShowText header
			{Code: 401, Parameters: []interface{}{"Too tired to go out."}}, // text line
			{Code: 0},
		},
	}
	// Inactive page: trigger=1, has transfer
	transferPage := &resource.EventPage{
		Trigger: 1,
		Conditions: resource.EventPageConditions{
			Switch1Valid: true,
			Switch1ID:    306,
		},
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(1), float64(10), float64(15), float64(2)}},
			{Code: 0},
		},
	}

	npc := &NPCRuntime{
		EventID:    20,
		Name:       "Door",
		X:          15,
		Y:          30,
		Dir:        2,
		ActivePage: dialogPage, // Page 1 is active (no conditions)
		MapEvent: &resource.MapEvent{
			ID:    20,
			Name:  "Door",
			X:     15,
			Y:     30,
			Pages: []*resource.EventPage{dialogPage, transferPage},
		},
	}
	room.npcs = append(room.npcs, npc)

	// Player walks onto the door → should NOT auto-transfer
	td := room.GetTransferAt(15, 30)
	assert.Nil(t, td, "GetTransferAt should return nil when active page has dialog but no transfer")
}

func TestGetTransferAt_ActivePage_HasTransfer_ReturnsTransfer(t *testing.T) {
	room := newTestRoom(t, 5, 30, 36)

	// Active page: trigger=1, has transfer command
	transferPage := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(1), float64(10), float64(15), float64(2)}},
			{Code: 0},
		},
	}

	npc := &NPCRuntime{
		EventID:    20,
		Name:       "Door",
		X:          15,
		Y:          30,
		Dir:        2,
		ActivePage: transferPage,
		MapEvent: &resource.MapEvent{
			ID:    20,
			Name:  "Door",
			X:     15,
			Y:     30,
			Pages: []*resource.EventPage{transferPage},
		},
	}
	room.npcs = append(room.npcs, npc)

	td := room.GetTransferAt(15, 30)
	require.NotNil(t, td, "GetTransferAt should return transfer when active page has transfer command")
	assert.Equal(t, 1, td.MapID)
	assert.Equal(t, 10, td.X)
	assert.Equal(t, 15, td.Y)
	assert.Equal(t, 2, td.Dir)
}

func TestGetTransferAt_NoActivePage_FallbackFindsTransfer(t *testing.T) {
	room := newTestRoom(t, 5, 30, 36)

	// Page with trigger=1 and a transfer command
	transferPage := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(3), float64(5), float64(8), float64(4)}},
			{Code: 0},
		},
	}

	npc := &NPCRuntime{
		EventID:    25,
		Name:       "Portal",
		X:          10,
		Y:          5,
		Dir:        2,
		ActivePage: nil, // No active page (conditions not met)
		MapEvent: &resource.MapEvent{
			ID:    25,
			Name:  "Portal",
			X:     10,
			Y:     5,
			Pages: []*resource.EventPage{transferPage},
		},
	}
	room.npcs = append(room.npcs, npc)

	td := room.GetTransferAt(10, 5)
	require.NotNil(t, td, "GetTransferAt should fall back to scanning all pages when no active page")
	assert.Equal(t, 3, td.MapID)
}

func TestGetTransferAt_ActionButtonTrigger_Skipped(t *testing.T) {
	room := newTestRoom(t, 5, 30, 36)

	// Active page with trigger=0 (Action Button) and a transfer — should be skipped
	// because GetTransferAt only handles trigger 1/2 (auto-trigger on walk).
	page := &resource.EventPage{
		Trigger: 0,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(2), float64(1), float64(1), float64(2)}},
			{Code: 0},
		},
	}

	npc := newNPC(30, 5, 5, page)
	room.npcs = append(room.npcs, npc)

	td := room.GetTransferAt(5, 5)
	assert.Nil(t, td, "GetTransferAt should skip events with trigger=0 (action button)")
}

func TestGetTransferAt_PositionMismatch_ReturnsNil(t *testing.T) {
	room := newTestRoom(t, 5, 30, 36)

	page := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(1), float64(0), float64(0), float64(2)}},
			{Code: 0},
		},
	}
	npc := newNPC(10, 5, 5, page)
	room.npcs = append(room.npcs, npc)

	// Query a different position
	td := room.GetTransferAt(6, 5)
	assert.Nil(t, td, "GetTransferAt should return nil for non-matching position")
}

func TestGetTransferAt_VariableBased_Skipped(t *testing.T) {
	room := newTestRoom(t, 5, 30, 36)

	// Transfer with mode=1 (variable-based) should be skipped by findTransferInPage
	page := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(1), float64(5), float64(6), float64(7), float64(2)}},
			{Code: 0},
		},
	}
	npc := newNPC(10, 5, 5, page)
	room.npcs = append(room.npcs, npc)

	td := room.GetTransferAt(5, 5)
	assert.Nil(t, td, "GetTransferAt should skip variable-based transfers (mode=1)")
}

// ========================================================================
// Tests for tryMoveNPC — NPC passability (the "cat walks through walls" bug)
// ========================================================================

func TestTryMoveNPC_PassableTile_Moves(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 5, 5, page)
	room.npcs = append(room.npcs, npc)

	// All tiles are passable by default
	ok := room.tryMoveNPC(npc, dirDown)
	assert.True(t, ok, "NPC should move to passable tile")
	assert.Equal(t, 5, npc.X)
	assert.Equal(t, 6, npc.Y)
	assert.Equal(t, dirDown, npc.Dir)
	assert.True(t, npc.dirty)
}

func TestTryMoveNPC_OutOfBounds_Blocked(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{PriorityType: 1}

	tests := []struct {
		name     string
		startX   int
		startY   int
		dir      int
	}{
		{"left edge", 0, 5, dirLeft},
		{"right edge", 9, 5, dirRight},
		{"top edge", 5, 0, dirUp},
		{"bottom edge", 5, 9, dirDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npc := newNPC(1, tt.startX, tt.startY, page)
			room.npcs = []*NPCRuntime{npc}

			ok := room.tryMoveNPC(npc, tt.dir)
			assert.False(t, ok, "NPC should not move out of bounds")
			assert.Equal(t, tt.startX, npc.X)
			assert.Equal(t, tt.startY, npc.Y)
		})
	}
}

func TestTryMoveNPC_ImpassableSource_Blocked(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	// Block movement DOWN from (5,5) — source tile check
	room.passMap.SetPass(5, 5, 2, false) // can't leave (5,5) going down

	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 5, 5, page)
	room.npcs = []*NPCRuntime{npc}

	ok := room.tryMoveNPC(npc, dirDown)
	assert.False(t, ok, "NPC should not move when source tile blocks the direction")
	assert.Equal(t, 5, npc.X)
	assert.Equal(t, 5, npc.Y)
}

func TestTryMoveNPC_ImpassableDestination_Blocked(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	// Block movement INTO (5,6) from above — destination tile check
	// To enter (5,6) from above, we move DOWN, and the reverse direction is UP(8)
	room.passMap.SetPass(5, 6, 8, false) // can't enter (5,6) from up

	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 5, 5, page)
	room.npcs = []*NPCRuntime{npc}

	ok := room.tryMoveNPC(npc, dirDown)
	assert.False(t, ok, "NPC should not move when destination tile blocks entry from reverse direction")
	assert.Equal(t, 5, npc.X)
	assert.Equal(t, 5, npc.Y)
}

func TestTryMoveNPC_Through_IgnoresPassability(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	// Block ALL directions at (5,5)
	for _, dir := range []int{2, 4, 6, 8} {
		room.passMap.SetPass(5, 5, dir, false)
	}
	// Block ALL directions at (5,6)
	for _, dir := range []int{2, 4, 6, 8} {
		room.passMap.SetPass(5, 6, dir, false)
	}

	page := &resource.EventPage{PriorityType: 1, Through: true}
	npc := newNPC(1, 5, 5, page)
	room.npcs = []*NPCRuntime{npc}

	ok := room.tryMoveNPC(npc, dirDown)
	assert.True(t, ok, "NPC with Through=true should move through impassable tiles")
	assert.Equal(t, 5, npc.X)
	assert.Equal(t, 6, npc.Y)
}

func TestTryMoveNPC_Through_StillBlockedByBounds(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	// Through NPCs should still be blocked at map boundaries
	page := &resource.EventPage{PriorityType: 1, Through: true}
	npc := newNPC(1, 0, 5, page)
	room.npcs = []*NPCRuntime{npc}

	ok := room.tryMoveNPC(npc, dirLeft)
	assert.False(t, ok, "NPC with Through=true should still be blocked at map boundary")
}

func TestTryMoveNPC_NPCCollision_Blocked(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	page1 := &resource.EventPage{PriorityType: 1}
	page2 := &resource.EventPage{PriorityType: 1}

	npc1 := newNPC(1, 5, 5, page1)
	npc2 := newNPC(2, 5, 6, page2) // blocking the tile below npc1
	room.npcs = []*NPCRuntime{npc1, npc2}

	ok := room.tryMoveNPC(npc1, dirDown)
	assert.False(t, ok, "NPC should not move to a tile occupied by another same-priority NPC")
	assert.Equal(t, 5, npc1.X)
	assert.Equal(t, 5, npc1.Y)
}

func TestTryMoveNPC_NPCCollision_DifferentPriority_Allowed(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	page1 := &resource.EventPage{PriorityType: 1} // same as characters
	page2 := &resource.EventPage{PriorityType: 0} // below characters

	npc1 := newNPC(1, 5, 5, page1)
	npc2 := newNPC(2, 5, 6, page2) // different priority → no collision
	room.npcs = []*NPCRuntime{npc1, npc2}

	ok := room.tryMoveNPC(npc1, dirDown)
	assert.True(t, ok, "NPC should move to tile with NPC of different priority")
}

func TestTryMoveNPC_NPCCollision_OtherThrough_Allowed(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	page1 := &resource.EventPage{PriorityType: 1}
	page2 := &resource.EventPage{PriorityType: 1, Through: true}

	npc1 := newNPC(1, 5, 5, page1)
	npc2 := newNPC(2, 5, 6, page2) // Through → doesn't block
	room.npcs = []*NPCRuntime{npc1, npc2}

	ok := room.tryMoveNPC(npc1, dirDown)
	assert.True(t, ok, "NPC should move to tile with Through NPC")
}

func TestTryMoveNPC_RegionRestriction_Blocked(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	// Set up region restrictions
	room.res.RegionRestr = &resource.RegionRestrictions{
		EventRestrict: []int{255},
	}
	// Set region 255 at destination tile
	room.passMap.SetRegion(5, 6, 255)

	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 5, 5, page)
	room.npcs = []*NPCRuntime{npc}

	ok := room.tryMoveNPC(npc, dirDown)
	assert.False(t, ok, "NPC should be blocked by region restriction")
}

func TestTryMoveNPC_RegionAllow_SkipsPassability(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	// Block the source tile's down direction
	room.passMap.SetPass(5, 5, 2, false)

	// BUT set region that always allows NPC movement
	room.res.RegionRestr = &resource.RegionRestrictions{
		EventAllow: []int{100},
	}
	room.passMap.SetRegion(5, 6, 100) // destination has allow region

	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 5, 5, page)
	room.npcs = []*NPCRuntime{npc}

	ok := room.tryMoveNPC(npc, dirDown)
	assert.True(t, ok, "NPC should move to region-allowed tile even if tile passability says blocked")
}

func TestTryMoveNPC_NilActivePage_NoMove(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := newNPC(1, 5, 5, nil) // no active page
	room.npcs = []*NPCRuntime{npc}

	// tryMoveNPC checks Through via ActivePage — nil page means Through=false
	// Passability should work normally
	ok := room.tryMoveNPC(npc, dirDown)
	assert.True(t, ok, "NPC without active page should still be able to move on passable tiles")
}

func TestTryMoveNPC_NilPassMap_UsesMapDimensions(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	room.passMap = nil // no passability data

	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 5, 5, page)
	room.npcs = []*NPCRuntime{npc}

	ok := room.tryMoveNPC(npc, dirDown)
	assert.True(t, ok, "NPC should move when no passMap (passability checks skipped)")
	assert.Equal(t, 6, npc.Y)
}

func TestTryMoveNPC_NilPassMap_BoundsStillChecked(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	room.passMap = nil

	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 0, 5, page)
	room.npcs = []*NPCRuntime{npc}

	ok := room.tryMoveNPC(npc, dirLeft)
	assert.False(t, ok, "NPC should not move out of map bounds even without passMap")
}

// ========================================================================
// Tests for selectPage — condition matching
// ========================================================================

func TestSelectPage_NoState_ReturnsFirstPage(t *testing.T) {
	page0 := &resource.EventPage{Trigger: 0}
	page1 := &resource.EventPage{Trigger: 0}
	ev := &resource.MapEvent{
		ID:    1,
		Pages: []*resource.EventPage{page0, page1},
	}

	result := selectPage(ev, 1, nil)
	assert.Equal(t, page0, result, "With nil state, should return first page")
}

func TestSelectPage_WithState_HighestMatchingPage(t *testing.T) {
	state := NewGameState(nil)
	state.SetSwitch(306, true)

	page0 := &resource.EventPage{Trigger: 0} // no conditions
	page1 := &resource.EventPage{
		Trigger: 1,
		Conditions: resource.EventPageConditions{
			Switch1Valid: true,
			Switch1ID:    306,
		},
	}

	ev := &resource.MapEvent{
		ID:    1,
		Pages: []*resource.EventPage{page0, page1},
	}

	// With switch 306 ON, page1 (highest index with met conditions) wins
	result := selectPage(ev, 1, state)
	assert.Equal(t, page1, result, "Should return highest-index page with conditions met")
}

func TestSelectPage_ConditionsNotMet_FallsBack(t *testing.T) {
	state := NewGameState(nil)
	// Switch 306 is OFF

	page0 := &resource.EventPage{Trigger: 0} // no conditions
	page1 := &resource.EventPage{
		Trigger: 1,
		Conditions: resource.EventPageConditions{
			Switch1Valid: true,
			Switch1ID:    306,
		},
	}

	ev := &resource.MapEvent{
		ID:    1,
		Pages: []*resource.EventPage{page0, page1},
	}

	result := selectPage(ev, 1, state)
	assert.Equal(t, page0, result, "Should fall back to page0 when page1 conditions not met")
}

func TestSelectPage_SelfSwitch_Works(t *testing.T) {
	state := NewGameState(nil)
	state.SetSelfSwitch(5, 20, "A", true)

	page0 := &resource.EventPage{Trigger: 0}
	page1 := &resource.EventPage{
		Trigger: 0,
		Conditions: resource.EventPageConditions{
			SelfSwitchValid: true,
			SelfSwitchCh:    "A",
		},
	}

	ev := &resource.MapEvent{
		ID:    20,
		Pages: []*resource.EventPage{page0, page1},
	}

	result := selectPage(ev, 5, state)
	assert.Equal(t, page1, result, "Should match self-switch condition")
}

func TestSelectPage_Variable_Condition(t *testing.T) {
	state := NewGameState(nil)
	state.SetVariable(206, 6)

	page0 := &resource.EventPage{Trigger: 0}
	page1 := &resource.EventPage{
		Trigger: 0,
		Conditions: resource.EventPageConditions{
			VariableValid: true,
			VariableID:    206,
			VariableValue: 6,
		},
	}

	ev := &resource.MapEvent{
		ID:    20,
		Pages: []*resource.EventPage{page0, page1},
	}

	// Variable 206 = 6 >= 6 → condition met
	result := selectPage(ev, 5, state)
	assert.Equal(t, page1, result)

	// Variable 206 = 5 < 6 → condition NOT met
	state.SetVariable(206, 5)
	result = selectPage(ev, 5, state)
	assert.Equal(t, page0, result)
}

func TestSelectPage_MultipleConditions(t *testing.T) {
	state := NewGameState(nil)

	page0 := &resource.EventPage{Trigger: 0}
	page1 := &resource.EventPage{
		Trigger: 0,
		Conditions: resource.EventPageConditions{
			Switch1Valid: true,
			Switch1ID:    10,
			Switch2Valid: true,
			Switch2ID:    20,
		},
	}

	ev := &resource.MapEvent{
		ID:    1,
		Pages: []*resource.EventPage{page0, page1},
	}

	// Only switch 10 ON → page1 not selected
	state.SetSwitch(10, true)
	result := selectPage(ev, 1, state)
	assert.Equal(t, page0, result)

	// Both switches ON → page1 selected
	state.SetSwitch(20, true)
	result = selectPage(ev, 1, state)
	assert.Equal(t, page1, result)
}

// ========================================================================
// Tests for populateNPCs
// ========================================================================

func TestPopulateNPCs_CreatesRuntimeForAllEvents(t *testing.T) {
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			5: {
				ID: 5, Width: 10, Height: 10,
				Events: []*resource.MapEvent{
					nil, // event ID 0 (unused in RMMV)
					{
						ID: 1, Name: "NPC1", X: 3, Y: 4,
						Pages: []*resource.EventPage{
							{Trigger: 0, Image: resource.EventImage{Direction: 4}},
						},
					},
					{
						ID: 2, Name: "NPC2", X: 7, Y: 8,
						Pages: []*resource.EventPage{
							{Trigger: 1, Image: resource.EventImage{Direction: 6}},
						},
					},
				},
			},
		},
		Passability: make(map[int]*resource.PassabilityMap),
	}
	state := NewGameState(nil)

	room := &MapRoom{
		MapID:           5,
		npcs:            []*NPCRuntime{},
		players:         make(map[int64]*player.PlayerSession),
		runtimeMonsters: make(map[int64]*MonsterRuntime),
		drops:           make(map[int64]*DropRuntimeEntry),
		broadcastQ:      make(chan []byte, 16),
		stopCh:          make(chan struct{}),
		res:             res,
		state:           state,
		logger:          zap.NewNop(),
	}
	room.populateNPCs()

	require.Len(t, room.npcs, 2)
	assert.Equal(t, 1, room.npcs[0].EventID)
	assert.Equal(t, 3, room.npcs[0].X)
	assert.Equal(t, 4, room.npcs[0].Y)
	assert.Equal(t, 4, room.npcs[0].Dir) // from image direction

	assert.Equal(t, 2, room.npcs[1].EventID)
	assert.Equal(t, 7, room.npcs[1].X)
	assert.Equal(t, 8, room.npcs[1].Y)
	assert.Equal(t, 6, room.npcs[1].Dir)
}

func TestPopulateNPCs_SkipsNilAndEmptyEvents(t *testing.T) {
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			1: {
				ID: 1, Width: 10, Height: 10,
				Events: []*resource.MapEvent{
					nil,                           // nil event
					{ID: 1, Pages: nil},           // no pages
					{ID: 2, Pages: []*resource.EventPage{}}, // empty pages
					{
						ID: 3, X: 1, Y: 1,
						Pages: []*resource.EventPage{{Trigger: 0}},
					},
				},
			},
		},
		Passability: make(map[int]*resource.PassabilityMap),
	}
	state := NewGameState(nil)

	room := &MapRoom{
		MapID:           1,
		npcs:            []*NPCRuntime{},
		players:         make(map[int64]*player.PlayerSession),
		runtimeMonsters: make(map[int64]*MonsterRuntime),
		drops:           make(map[int64]*DropRuntimeEntry),
		broadcastQ:      make(chan []byte, 16),
		stopCh:          make(chan struct{}),
		res:             res,
		state:           state,
		logger:          zap.NewNop(),
	}
	room.populateNPCs()

	require.Len(t, room.npcs, 1)
	assert.Equal(t, 3, room.npcs[0].EventID)
}

// ========================================================================
// Tests for NPCSnapshot
// ========================================================================

func TestNPCSnapshot_IncludesAllFields(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	page := &resource.EventPage{
		PriorityType: 1,
		MoveType:     1,
		StepAnime:    true,
		DirectionFix: false,
		Through:      false,
		WalkAnime:    true,
		Image: resource.EventImage{
			CharacterName:  "Actor1",
			CharacterIndex: 2,
		},
	}
	npc := newNPC(5, 3, 4, page)
	room.npcs = []*NPCRuntime{npc}

	snap := room.NPCSnapshot()
	require.Len(t, snap, 1)
	s := snap[0]
	assert.Equal(t, 5, s["event_id"])
	assert.Equal(t, "TestNPC", s["name"])
	assert.Equal(t, 3, s["x"])
	assert.Equal(t, 4, s["y"])
	assert.Equal(t, 2, s["dir"])
	assert.Equal(t, "Actor1", s["walk_name"])
	assert.Equal(t, 2, s["walk_index"])
	assert.Equal(t, 1, s["priority_type"])
	assert.Equal(t, 1, s["move_type"])
	assert.Equal(t, true, s["step_anime"])
	assert.Equal(t, false, s["direction_fix"])
	assert.Equal(t, false, s["through"])
	assert.Equal(t, true, s["walk_anime"])
}

func TestNPCSnapshot_NilActivePage_InvisibleDefaults(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	npc := newNPC(5, 3, 4, nil)
	room.npcs = []*NPCRuntime{npc}

	snap := room.NPCSnapshot()
	require.Len(t, snap, 1)
	s := snap[0]
	assert.Equal(t, "", s["walk_name"])
	assert.Equal(t, 0, s["walk_index"])
	assert.Equal(t, 0, s["priority_type"])
	assert.Equal(t, false, s["step_anime"])
}

// ========================================================================
// Tests for RefreshNPCPages
// ========================================================================

func TestRefreshNPCPages_DetectsPageChange(t *testing.T) {
	state := NewGameState(nil)

	page0 := &resource.EventPage{Trigger: 0}
	page1 := &resource.EventPage{
		Trigger: 0,
		Conditions: resource.EventPageConditions{
			SelfSwitchValid: true,
			SelfSwitchCh:    "A",
		},
		Image: resource.EventImage{Direction: 6},
	}

	ev := &resource.MapEvent{
		ID:    10,
		Pages: []*resource.EventPage{page0, page1},
	}

	room := newTestRoom(t, 5, 10, 10)
	room.state = state
	room.npcs = []*NPCRuntime{{
		EventID:    10,
		X:          3,
		Y:          4,
		Dir:        2,
		ActivePage: page0,
		MapEvent:   ev,
	}}

	// Initially page0 is active; no change expected.
	changed := room.RefreshNPCPages()
	assert.Empty(t, changed, "No page change expected initially")

	// Set self-switch A → page1 should become active.
	state.SetSelfSwitch(5, 10, "A", true)
	changed = room.RefreshNPCPages()
	require.Len(t, changed, 1)
	assert.Equal(t, 10, changed[0])
	assert.Equal(t, page1, room.npcs[0].ActivePage)
	assert.Equal(t, 6, room.npcs[0].Dir, "Direction should update from new page image")
}

// ========================================================================
// Tests for moveNPCRandom — basic movement tick behavior
// ========================================================================

func TestMoveNPCRandom_MovesToRandomDirection(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 5, 5, page)
	room.npcs = []*NPCRuntime{npc}

	// Call multiple times to ensure at least one move happens
	moved := false
	for i := 0; i < 100; i++ {
		npc.X = 5
		npc.Y = 5
		npc.dirty = false
		room.moveNPCRandom(npc)
		if npc.X != 5 || npc.Y != 5 {
			moved = true
			break
		}
	}
	assert.True(t, moved, "NPC should eventually move with random movement")
}

// ========================================================================
// Tests for moveNPCApproach — approach nearest player
// ========================================================================

func TestMoveNPCApproach_MovesTowardPlayer(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 5, 5, page)
	room.npcs = []*NPCRuntime{npc}

	// Add a player at (5, 8) — directly below the NPC
	sess := &player.PlayerSession{CharID: 1, X: 5, Y: 8}
	room.players[1] = sess

	room.moveNPCApproach(npc)
	assert.Equal(t, 5, npc.X)
	assert.Equal(t, 6, npc.Y, "NPC should move toward player (downward)")
	assert.Equal(t, dirDown, npc.Dir)
}

func TestMoveNPCApproach_NoPlayers_NoMove(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{PriorityType: 1}
	npc := newNPC(1, 5, 5, page)
	room.npcs = []*NPCRuntime{npc}

	room.moveNPCApproach(npc)
	assert.Equal(t, 5, npc.X)
	assert.Equal(t, 5, npc.Y, "NPC should not move when no players")
}

// ========================================================================
// Tests for GetAutorunNPCs
// ========================================================================

func TestGetAutorunNPCs_ReturnsOnlyTrigger3(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	actionPage := &resource.EventPage{Trigger: 0, List: []*resource.EventCommand{{Code: 101}, {Code: 0}}}
	touchPage := &resource.EventPage{Trigger: 1, List: []*resource.EventCommand{{Code: 101}, {Code: 0}}}
	autorunPage := &resource.EventPage{Trigger: 3, List: []*resource.EventCommand{{Code: 101}, {Code: 0}}}
	parallelPage := &resource.EventPage{Trigger: 4, List: []*resource.EventCommand{{Code: 101}, {Code: 0}}}

	room.npcs = []*NPCRuntime{
		newNPC(1, 1, 1, actionPage),
		newNPC(2, 2, 2, touchPage),
		newNPC(3, 3, 3, autorunPage),
		newNPC(4, 4, 4, parallelPage),
	}

	autoruns := room.GetAutorunNPCs()
	require.Len(t, autoruns, 1)
	assert.Equal(t, 3, autoruns[0].EventID)
}

func TestGetAutorunNPCs_SkipsEmptyCommandLists(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	// Autorun with only end marker (1 command) → skip
	emptyAutorun := &resource.EventPage{Trigger: 3, List: []*resource.EventCommand{{Code: 0}}}
	// Autorun with real commands → include
	realAutorun := &resource.EventPage{Trigger: 3, List: []*resource.EventCommand{{Code: 101}, {Code: 0}}}

	room.npcs = []*NPCRuntime{
		newNPC(1, 1, 1, emptyAutorun),
		newNPC(2, 2, 2, realAutorun),
	}

	autoruns := room.GetAutorunNPCs()
	require.Len(t, autoruns, 1)
	assert.Equal(t, 2, autoruns[0].EventID)
}

// ========================================================================
// Tests for GetEntryPoints
// ========================================================================

func TestGetEntryPoints_FindsTransferEvents(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	transferPage := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(2), float64(5), float64(5), float64(2)}},
			{Code: 0},
		},
	}
	nonTransferPage := &resource.EventPage{
		Trigger: 0,
		List: []*resource.EventCommand{
			{Code: 101}, {Code: 0},
		},
	}

	room.npcs = []*NPCRuntime{
		{
			EventID:  1,
			X:        3,
			Y:        9,
			MapEvent: &resource.MapEvent{ID: 1, X: 3, Y: 9, Pages: []*resource.EventPage{transferPage}},
		},
		{
			EventID:  2,
			X:        7,
			Y:        0,
			MapEvent: &resource.MapEvent{ID: 2, X: 7, Y: 0, Pages: []*resource.EventPage{nonTransferPage}},
		},
	}

	points := room.GetEntryPoints()
	require.Len(t, points, 1)
	assert.Equal(t, 3, points[0].X)
	assert.Equal(t, 9, points[0].Y)
}

// ========================================================================
// Integration: tickNPCs with dirty flag broadcasting
// ========================================================================

func TestTickNPCs_FixedMoveType_NeverMoves(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	page := &resource.EventPage{
		MoveType:      0, // fixed
		MoveFrequency: 3,
		PriorityType:  1,
	}
	npc := newNPC(1, 5, 5, page)
	npc.moveTimer = 0 // timer expired
	room.npcs = []*NPCRuntime{npc}

	// Tick multiple times
	for i := 0; i < 10; i++ {
		room.tickNPCs()
	}

	assert.Equal(t, 5, npc.X, "Fixed NPC should not move")
	assert.Equal(t, 5, npc.Y, "Fixed NPC should not move")
}

func TestTickNPCs_RandomMoveType_EventuallyMoves(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	page := &resource.EventPage{
		MoveType:      1, // random
		MoveFrequency: 5, // highest frequency
		PriorityType:  1,
	}
	npc := newNPC(1, 5, 5, page)
	npc.moveTimer = 0 // force immediate move
	room.npcs = []*NPCRuntime{npc}

	startX, startY := npc.X, npc.Y
	moved := false
	for i := 0; i < 100; i++ {
		npc.moveTimer = 0 // keep forcing moves
		room.tickNPCs()
		if npc.X != startX || npc.Y != startY {
			moved = true
			break
		}
	}
	assert.True(t, moved, "Random NPC should eventually move")
}
