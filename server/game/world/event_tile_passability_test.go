package world

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/ai"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---- Event Tile Passability Tests ----

// newTestRoomWithTileset creates a MapRoom with a tileset for testing event tile passability.
func newTestRoomWithTileset(t *testing.T, mapID, width, height int, tilesetFlags []int) *MapRoom {
	t.Helper()
	pm := resource.NewPassabilityMap(width, height)
	md := &resource.MapData{ID: mapID, Width: width, Height: height, TilesetID: 1}
	res := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{mapID: md},
		Passability: map[int]*resource.PassabilityMap{mapID: pm},
		Tilesets: []*resource.Tileset{{ID: 1, Flags: tilesetFlags}},
	}
	state := NewGameState(nil, nil)
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

func TestCheckPassage_NoEventTiles_FallsThrough(t *testing.T) {
	flags := []int{0x10, 0x00, 0x0F} // 0=star, 1=passable, 2=blocked
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)
	// No NPCs — should fall through to static passability.
	// Static map is all passable by default (NewPassabilityMap).
	assert.True(t, room.CheckPassage(5, 5, 2))
	assert.True(t, room.CheckPassage(5, 5, 4))
}

func TestCheckPassage_EventTile_Blocks(t *testing.T) {
	// Tile ID 2 is blocked (flag=0x0F, all directions blocked).
	flags := make([]int, 16)
	flags[2] = 0x0F // blocked all directions
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)

	// Add an NPC tile event at (5, 5) with tileId=2, priorityType=0 (below chars).
	npc := &NPCRuntime{
		EventID: 1,
		X:       5,
		Y:       5,
		Dir:     2,
		ActivePage: &resource.EventPage{
			PriorityType: 0, // below characters = tile event
			Image:        resource.EventImage{TileID: 2},
		},
	}
	room.npcs = append(room.npcs, npc)

	// Event tile should block movement.
	assert.False(t, room.CheckPassage(5, 5, 2), "event tile should block down")
	assert.False(t, room.CheckPassage(5, 5, 4), "event tile should block left")
	assert.False(t, room.CheckPassage(5, 5, 6), "event tile should block right")
	assert.False(t, room.CheckPassage(5, 5, 8), "event tile should block up")

	// Different position should not be affected.
	assert.True(t, room.CheckPassage(3, 3, 2), "other position unaffected")
}

func TestCheckPassage_EventTile_Passable(t *testing.T) {
	flags := make([]int, 16)
	flags[3] = 0x00 // passable all directions
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)

	// Block the static passability at (5, 5).
	room.passMap.SetPass(5, 5, 2, false)
	room.passMap.SetPass(5, 5, 4, false)

	// Add a passable tile event at (5, 5).
	npc := &NPCRuntime{
		EventID: 1,
		X:       5,
		Y:       5,
		Dir:     2,
		ActivePage: &resource.EventPage{
			PriorityType: 0,
			Image:        resource.EventImage{TileID: 3},
		},
	}
	room.npcs = append(room.npcs, npc)

	// Event tile should make it passable even though static map says blocked.
	assert.True(t, room.CheckPassage(5, 5, 2), "event tile overrides static → passable")
	assert.True(t, room.CheckPassage(5, 5, 4), "event tile overrides static → passable")
}

func TestCheckPassage_EventTile_StarSkipped(t *testing.T) {
	flags := make([]int, 16)
	flags[4] = 0x10 // star tile — skipped in base RMMV
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)

	// Block the static passability at (5, 5).
	room.passMap.SetPass(5, 5, 2, false)

	// Add a star tile event at (5, 5).
	npc := &NPCRuntime{
		EventID: 1,
		X:       5,
		Y:       5,
		Dir:     2,
		ActivePage: &resource.EventPage{
			PriorityType: 0,
			Image:        resource.EventImage{TileID: 4},
		},
	}
	room.npcs = append(room.npcs, npc)

	// Star tile is skipped — falls through to static map (blocked).
	assert.False(t, room.CheckPassage(5, 5, 2), "star event tile skipped, static blocks")
}

func TestCheckPassage_EventTile_StarBlocksWithCPFix(t *testing.T) {
	flags := make([]int, 16)
	flags[5] = 0x10 | 0x01 // star + blocked-down
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)
	room.res.CPStarPassFix = true

	npc := &NPCRuntime{
		EventID: 1,
		X:       5,
		Y:       5,
		Dir:     2,
		ActivePage: &resource.EventPage{
			PriorityType: 0,
			Image:        resource.EventImage{TileID: 5},
		},
	}
	room.npcs = append(room.npcs, npc)

	// With CP fix, star tile with blocked-down bit blocks movement down.
	assert.False(t, room.CheckPassage(5, 5, 2), "CP fix: star blocks down")
	// Other directions not blocked by this star tile → fall through to static (passable).
	assert.True(t, room.CheckPassage(5, 5, 4), "CP fix: star doesn't block left")
}

func TestCheckPassage_EventTile_PriorityType1_Ignored(t *testing.T) {
	flags := make([]int, 16)
	flags[2] = 0x0F // blocked
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)

	// NPC with tileId > 0 but priorityType=1 (same as characters) — NOT a tile event.
	npc := &NPCRuntime{
		EventID: 1,
		X:       5,
		Y:       5,
		Dir:     2,
		ActivePage: &resource.EventPage{
			PriorityType: 1, // same level = NOT a tile event
			Image:        resource.EventImage{TileID: 2},
		},
	}
	room.npcs = append(room.npcs, npc)

	// Should be passable (falls through to static which is all-passable by default).
	assert.True(t, room.CheckPassage(5, 5, 2))
}

func TestCheckPassage_EventTile_NoActivePage(t *testing.T) {
	flags := make([]int, 16)
	flags[2] = 0x0F
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)

	// NPC with no active page — should be ignored.
	npc := &NPCRuntime{
		EventID: 1,
		X:       5,
		Y:       5,
		Dir:     2,
	}
	room.npcs = append(room.npcs, npc)

	assert.True(t, room.CheckPassage(5, 5, 2))
}

func TestCheckPassage_DirectionalBlocking(t *testing.T) {
	// Tile that blocks only down (bit 0x01).
	flags := make([]int, 16)
	flags[6] = 0x01 // only bit0 set = blocks down
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)

	npc := &NPCRuntime{
		EventID: 1,
		X:       5,
		Y:       5,
		Dir:     2,
		ActivePage: &resource.EventPage{
			PriorityType: 0,
			Image:        resource.EventImage{TileID: 6},
		},
	}
	room.npcs = append(room.npcs, npc)

	assert.False(t, room.CheckPassage(5, 5, 2), "blocks down")
	assert.True(t, room.CheckPassage(5, 5, 4), "passable left")
	assert.True(t, room.CheckPassage(5, 5, 6), "passable right")
	assert.True(t, room.CheckPassage(5, 5, 8), "passable up")
}

func TestIsPassable_AcquiresLock(t *testing.T) {
	flags := make([]int, 16)
	flags[2] = 0x0F
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)

	npc := &NPCRuntime{
		EventID: 1,
		X:       5,
		Y:       5,
		Dir:     2,
		ActivePage: &resource.EventPage{
			PriorityType: 0,
			Image:        resource.EventImage{TileID: 2},
		},
	}
	room.npcs = append(room.npcs, npc)

	// IsPassable acquires the lock — should work without deadlock.
	assert.False(t, room.IsPassable(5, 5, 2))
	assert.True(t, room.IsPassable(3, 3, 2))
}

func TestDirToBit(t *testing.T) {
	assert.Equal(t, 0x01, dirToBit(2))
	assert.Equal(t, 0x02, dirToBit(4))
	assert.Equal(t, 0x04, dirToBit(6))
	assert.Equal(t, 0x08, dirToBit(8))
	assert.Equal(t, 0, dirToBit(0))
	assert.Equal(t, 0, dirToBit(5))
}

// ---- tryMoveNPC with event tile passability ----

func TestTryMoveNPC_EventTileBlocks(t *testing.T) {
	flags := make([]int, 16)
	flags[7] = 0x0F // blocked
	room := newTestRoomWithTileset(t, 1, 10, 10, flags)

	// Place a blocking tile event at (5, 6) — the destination when moving down from (5, 5).
	tileEvent := &NPCRuntime{
		EventID: 99,
		X:       5,
		Y:       6,
		Dir:     2,
		ActivePage: &resource.EventPage{
			PriorityType: 0,
			Image:        resource.EventImage{TileID: 7},
		},
	}
	room.npcs = append(room.npcs, tileEvent)

	// NPC trying to move down from (5, 5) to (5, 6).
	mover := &NPCRuntime{
		EventID: 1,
		X:       5,
		Y:       5,
		Dir:     2,
		ActivePage: &resource.EventPage{PriorityType: 1},
	}
	room.npcs = append(room.npcs, mover)

	ok := room.tryMoveNPC(mover, dirDown)
	assert.False(t, ok, "should be blocked by event tile at destination")
	assert.Equal(t, 5, mover.X)
	assert.Equal(t, 5, mover.Y)
}

// ---- Map wrapping in movement ----

// makeLoopPassMap creates a fully passable looping map via buildPassability.
func makeLoopPassMap(t *testing.T, w, h, scrollType int) (*resource.PassabilityMap, *resource.ResourceLoader) {
	t.Helper()
	// Fill all tiles with tile ID 1 (passable).
	data := make([]int, w*h) // 1 layer
	for i := range data {
		data[i] = 1 // passable tile
	}
	md := &resource.MapData{ID: 1, Width: w, Height: h, TilesetID: 1, ScrollType: scrollType, Data: data}
	flags := make([]int, 8)
	flags[0] = 0x10 // star
	flags[1] = 0x00 // passable
	rl := resource.NewLoader("", "")
	rl.Tilesets = []*resource.Tileset{{ID: 1, Flags: flags}}
	rl.Maps = map[int]*resource.MapData{1: md}
	rl.Passability = make(map[int]*resource.PassabilityMap)
	rl.BuildPassabilityExported()
	pm := rl.Passability[1]
	require.NotNil(t, pm)
	return pm, rl
}

func TestTryMoveNPC_WrapsHorizontally(t *testing.T) {
	w, h := 10, 10
	pm, rl := makeLoopPassMap(t, w, h, 2) // horizontal loop
	require.True(t, pm.IsLoopH())

	state := NewGameState(nil, nil)
	room := &MapRoom{
		MapID:           1,
		mapWidth:        w,
		mapHeight:       h,
		passMap:         pm,
		res:             rl,
		state:           state,
		npcs:            []*NPCRuntime{},
		players:         make(map[int64]*player.PlayerSession),
		runtimeMonsters: make(map[int64]*MonsterRuntime),
		drops:           make(map[int64]*DropRuntimeEntry),
		broadcastQ:      make(chan []byte, 16),
		stopCh:          make(chan struct{}),
		logger:          zap.NewNop(),
	}

	npc := &NPCRuntime{
		EventID: 1,
		X:       0,
		Y:       5,
		Dir:     4,
		ActivePage: &resource.EventPage{PriorityType: 1},
	}
	room.npcs = append(room.npcs, npc)

	// Move left from x=0 should wrap to x=9.
	ok := room.tryMoveNPC(npc, dirLeft)
	assert.True(t, ok, "should wrap left")
	assert.Equal(t, 9, npc.X, "x should wrap to 9")
	assert.Equal(t, 5, npc.Y, "y unchanged")
}

func TestTryMoveMonster_WrapsVertically(t *testing.T) {
	w, h := 10, 10
	pm, rl := makeLoopPassMap(t, w, h, 1) // vertical loop
	require.True(t, pm.IsLoopV())

	room := &MapRoom{
		MapID:           1,
		mapWidth:        w,
		mapHeight:       h,
		passMap:         pm,
		res:             rl,
		npcs:            []*NPCRuntime{},
		players:         make(map[int64]*player.PlayerSession),
		runtimeMonsters: make(map[int64]*MonsterRuntime),
		drops:           make(map[int64]*DropRuntimeEntry),
		broadcastQ:      make(chan []byte, 16),
		stopCh:          make(chan struct{}),
		logger:          zap.NewNop(),
	}

	m := &MonsterRuntime{
		InstID:   1,
		X:        5,
		Y:        0,
		Dir:      8,
		HP:       100,
		MaxHP:    100,
		State:    ai.StateIdle,
		Threat:   ai.NewThreatTable(),
		Template: &resource.Enemy{ID: 1, Name: "Slime", HP: 100},
	}

	// Move up from y=0 should wrap to y=9.
	ok := room.TryMoveMonster(m, dirUp)
	assert.True(t, ok, "should wrap up")
	assert.Equal(t, 5, m.X)
	assert.Equal(t, 9, m.Y, "y should wrap to 9")
}

// ---- A* pathfinding with wrapping ----

func TestAStar_WrappingShortcut(t *testing.T) {
	// Create a 10x1 horizontal loop map. Path from (0,0) to (9,0) should be 1 step (wrap left).
	w, h := 10, 1
	pm, _ := makeLoopPassMap(t, w, h, 2) // horizontal loop
	require.True(t, pm.IsLoopH())

	path := ai.AStar(pm, ai.Point{0, 0}, ai.Point{9, 0})
	require.NotNil(t, path, "path should exist via wrapping")
	assert.Equal(t, 1, len(path), "wrapping path should be 1 step")
	assert.Equal(t, ai.Point{9, 0}, path[0])
}
