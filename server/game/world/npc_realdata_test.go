package world

import (
	"os"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// This file contains integration tests using actual game data files.
// Tests are skipped if the game data directory is not available.

const realDataPath = "../../../../projectb/www/data"

func skipIfNoData(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(realDataPath); os.IsNotExist(err) {
		t.Skip("game data not available at", realDataPath)
	}
}

func loadRealResources(t *testing.T) *resource.ResourceLoader {
	t.Helper()
	skipIfNoData(t)
	rl := resource.NewLoader(realDataPath, "")
	require.NoError(t, rl.Load(), "Failed to load game resources")
	return rl
}

// ========================================================================
// Test: Map 5 NPC Population
// ========================================================================

func TestRealData_Map5_NPCPopulation(t *testing.T) {
	rl := loadRealResources(t)
	state := NewGameState(nil, nil)

	md, ok := rl.Maps[5]
	require.True(t, ok, "Map 5 should exist")
	t.Logf("Map 5: %dx%d, tilesetID=%d, %d events", md.Width, md.Height, md.TilesetID, len(md.Events))

	room := newMapRoom(5, rl, state, zap.NewNop())

	t.Logf("NPC count: %d", len(room.npcs))
	assert.Greater(t, len(room.npcs), 0, "Map 5 should have at least some NPCs")

	// Log all NPCs for debugging
	for _, npc := range room.npcs {
		pageTrigger := -1
		pageThrough := false
		pageMoveType := -1
		walkName := ""
		if npc.ActivePage != nil {
			pageTrigger = npc.ActivePage.Trigger
			pageThrough = npc.ActivePage.Through
			pageMoveType = npc.ActivePage.MoveType
			walkName = npc.ActivePage.Image.CharacterName
		}
		t.Logf("  NPC[%d] %q at (%d,%d) dir=%d trigger=%d through=%v moveType=%d walk=%q",
			npc.EventID, npc.Name, npc.X, npc.Y, npc.Dir,
			pageTrigger, pageThrough, pageMoveType, walkName)
	}
}

// ========================================================================
// Test: Map 5 Passability at Door (Event 20)
// ========================================================================

func TestRealData_Map5_DoorPassability(t *testing.T) {
	rl := loadRealResources(t)

	pm := rl.Passability[5]
	require.NotNil(t, pm, "Map 5 should have passability data")

	md := rl.Maps[5]
	require.NotNil(t, md)

	// Find Event 20 (the green arrow door)
	var doorEvent *resource.MapEvent
	for _, ev := range md.Events {
		if ev != nil && ev.ID == 20 {
			doorEvent = ev
			break
		}
	}
	require.NotNil(t, doorEvent, "Event 20 should exist on Map 5")
	t.Logf("Event 20 (Door): position=(%d,%d), pages=%d", doorEvent.X, doorEvent.Y, len(doorEvent.Pages))

	doorX, doorY := doorEvent.X, doorEvent.Y

	// Check passability at and around the door tile
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			x, y := doorX+dx, doorY+dy
			if x < 0 || x >= pm.Width || y < 0 || y >= pm.Height {
				continue
			}
			region := pm.RegionAt(x, y)
			t.Logf("  Tile (%d,%d): down=%v left=%v right=%v up=%v region=%d",
				x, y,
				pm.CanPass(x, y, 2),
				pm.CanPass(x, y, 4),
				pm.CanPass(x, y, 6),
				pm.CanPass(x, y, 8),
				region)
		}
	}

	// The tile directly above the door should be passable going down
	// (player walks from above to the door tile)
	aboveX, aboveY := doorX, doorY-1
	if aboveY >= 0 {
		canLeaveDown := pm.CanPass(aboveX, aboveY, 2)
		canEnterFromUp := pm.CanPass(doorX, doorY, 8)
		t.Logf("  Can leave (%d,%d) going down: %v", aboveX, aboveY, canLeaveDown)
		t.Logf("  Can enter (%d,%d) from up: %v", doorX, doorY, canEnterFromUp)

		if !canLeaveDown || !canEnterFromUp {
			t.Logf("  WARNING: Door tile is NOT reachable from above! This blocks player-touch trigger.")
		}
	}
}

// ========================================================================
// Test: Map 5 GetTransferAt for Door (Event 20) — the actual bug
// ========================================================================

func TestRealData_Map5_DoorTransfer(t *testing.T) {
	rl := loadRealResources(t)
	state := NewGameState(nil, nil)
	room := newMapRoom(5, rl, state, zap.NewNop())

	// Find Event 20's position
	var doorX, doorY int
	for _, npc := range room.npcs {
		if npc.EventID == 20 {
			doorX = npc.X
			doorY = npc.Y
			t.Logf("Door NPC (Event 20): pos=(%d,%d), activePage=%v", doorX, doorY, npc.ActivePage != nil)
			if npc.ActivePage != nil {
				t.Logf("  Active page trigger=%d, through=%v, moveType=%d",
					npc.ActivePage.Trigger, npc.ActivePage.Through, npc.ActivePage.MoveType)
				for i, cmd := range npc.ActivePage.List {
					if cmd != nil {
						t.Logf("    Cmd[%d]: code=%d indent=%d", i, cmd.Code, cmd.Indent)
					}
				}
			}
			break
		}
	}

	// With fresh state (no switches set), GetTransferAt should return nil
	// because the active page should be page 1 (dialog "too tired")
	td := room.GetTransferAt(doorX, doorY)
	if td != nil {
		t.Errorf("GetTransferAt returned transfer to map %d — should be nil (active page has dialog, not transfer)", td.MapID)
	} else {
		t.Log("GetTransferAt correctly returns nil for door with dialog page active")
	}

	// Now enable Switch 306 → page 2 (transfer page) should become active
	state.SetSwitch(306, true)
	changed := room.RefreshNPCPages()
	t.Logf("After setting switch 306: %d pages changed", len(changed))

	td = room.GetTransferAt(doorX, doorY)
	if td == nil {
		t.Error("GetTransferAt should return transfer when switch 306 is ON (page 2 active)")
	} else {
		t.Logf("GetTransferAt correctly returns transfer to map %d at (%d,%d)", td.MapID, td.X, td.Y)
	}
}

// ========================================================================
// Test: Map 5 EV10 bathroom door — TE_CALL_ORIGIN_EVENT transfer detection
// ========================================================================

func TestRealData_Map5_EV10_BathroomDoor(t *testing.T) {
	rl := loadRealResources(t)
	state := NewGameState(nil, nil)
	room := newMapRoom(5, rl, state, zap.NewNop())

	// Find NPC for Event 10 (bathroom door at 19,13).
	var doorNPC *NPCRuntime
	for _, npc := range room.npcs {
		if npc.EventID == 10 {
			doorNPC = npc
			break
		}
	}
	require.NotNil(t, doorNPC, "Event 10 should exist on Map 5")

	t.Logf("EV10: pos=(%d,%d), name=%q", doorNPC.X, doorNPC.Y, doorNPC.Name)
	assert.Equal(t, 19, doorNPC.X)
	assert.Equal(t, 13, doorNPC.Y)

	// Check template resolution: OriginalPages should be set.
	me := doorNPC.MapEvent
	require.NotNil(t, me, "MapEvent should be set")
	t.Logf("EV10 MapEvent: note=%q, pages=%d, originalPages=%d",
		me.Note, len(me.Pages), len(me.OriginalPages))
	require.NotNil(t, me.OriginalPages, "OriginalPages should be set (template resolved)")
	require.True(t, len(me.OriginalPages) > 0, "OriginalPages should have at least 1 page")

	// Check original page has transfer command (code 201).
	origPage := me.OriginalPages[0]
	require.NotNil(t, origPage)
	hasTransfer := false
	for _, cmd := range origPage.List {
		if cmd != nil && cmd.Code == 201 {
			hasTransfer = true
			t.Logf("  Original page has Transfer to map=%v x=%v y=%v",
				cmd.Parameters[1], cmd.Parameters[2], cmd.Parameters[3])
		}
	}
	assert.True(t, hasTransfer, "Original page should have Transfer Player (code 201)")

	// Check active page (template page).
	require.NotNil(t, doorNPC.ActivePage, "Should have an active page")
	t.Logf("EV10 active page: trigger=%d, cmd_count=%d",
		doorNPC.ActivePage.Trigger, len(doorNPC.ActivePage.List))
	assert.Equal(t, 1, doorNPC.ActivePage.Trigger, "Template trigger should be 1 (player touch)")

	// Check active page has TE_CALL_ORIGIN_EVENT.
	assert.True(t, hasCallOriginEvent(doorNPC.ActivePage),
		"Active page should contain TE固有イベント呼び出し plugin command")

	// GetTransferAt should find the transfer via OriginalPages.
	td := room.GetTransferAt(19, 13)
	require.NotNil(t, td, "GetTransferAt(19,13) should find transfer via TE_CALL_ORIGIN_EVENT → OriginalPages")
	assert.Equal(t, 50, td.MapID, "Should transfer to map 50 (bathroom)")
	t.Logf("GetTransferAt result: map=%d x=%d y=%d dir=%d", td.MapID, td.X, td.Y, td.Dir)
}

// ========================================================================
// Test: Map 5 Cat NPC — Through flag and movement
// ========================================================================

func TestRealData_Map5_NPCMovement(t *testing.T) {
	rl := loadRealResources(t)
	state := NewGameState(nil, nil)
	room := newMapRoom(5, rl, state, zap.NewNop())

	// Find all NPCs with MoveType != 0 (moving NPCs)
	for _, npc := range room.npcs {
		if npc.ActivePage == nil || npc.ActivePage.MoveType == 0 {
			continue
		}
		t.Logf("Moving NPC[%d] %q at (%d,%d): moveType=%d through=%v priority=%d walk=%q",
			npc.EventID, npc.Name, npc.X, npc.Y,
			npc.ActivePage.MoveType, npc.ActivePage.Through,
			npc.ActivePage.PriorityType, npc.ActivePage.Image.CharacterName)

		if npc.ActivePage.Through {
			t.Logf("  WARNING: NPC %d has Through=true — will walk through walls (intended by map maker?)", npc.EventID)
		}

		// Verify that tryMoveNPC respects walls for non-Through NPCs
		if !npc.ActivePage.Through {
			origX, origY := npc.X, npc.Y
			// Try moving in all 4 directions
			for _, dir := range []int{2, 4, 6, 8} {
				npc.X = origX
				npc.Y = origY
				npc.dirty = false
				moved := room.tryMoveNPC(npc, dir)
				if moved {
					// Verify the destination was actually passable
					dx, dy := dirDelta(dir)
					destX, destY := origX+dx, origY+dy
					if room.passMap != nil {
						srcCanLeave := room.passMap.CanPass(origX, origY, dir)
						dstCanEnter := room.passMap.CanPass(destX, destY, reverseDir(dir))
						if !srcCanLeave || !dstCanEnter {
							t.Errorf("  NPC[%d] moved to (%d,%d) dir=%d but tile should be impassable! srcPass=%v dstPass=%v",
								npc.EventID, destX, destY, dir, srcCanLeave, dstCanEnter)
						}
					}
				}
			}
			// Restore position
			npc.X = origX
			npc.Y = origY
		}
	}
}

// ========================================================================
// Test: Map 5 HandleMove passability check matches client
// ========================================================================

func TestRealData_Map5_PlayerPassability(t *testing.T) {
	rl := loadRealResources(t)

	pm := rl.Passability[5]
	require.NotNil(t, pm)

	// Count passable vs impassable tiles
	totalTiles := pm.Width * pm.Height
	passable := 0
	blocked := 0
	for y := 0; y < pm.Height; y++ {
		for x := 0; x < pm.Width; x++ {
			anyPass := false
			for _, dir := range []int{2, 4, 6, 8} {
				if pm.CanPass(x, y, dir) {
					anyPass = true
					break
				}
			}
			if anyPass {
				passable++
			} else {
				blocked++
			}
		}
	}

	t.Logf("Map 5 passability: %d total tiles, %d passable, %d fully blocked (%.1f%% passable)",
		totalTiles, passable, blocked, float64(passable)/float64(totalTiles)*100)

	assert.Greater(t, passable, 0, "Map 5 should have passable tiles")
	assert.Greater(t, blocked, 0, "Map 5 should have blocked tiles (walls)")
}

// ========================================================================
// Test: Verify HandleMove two-way passability check
// ========================================================================

func TestRealData_Map5_TwoWayPassability(t *testing.T) {
	rl := loadRealResources(t)

	pm := rl.Passability[5]
	require.NotNil(t, pm)

	// For each tile, verify that if we can leave in a direction,
	// we can also enter the destination from the reverse direction.
	// Mismatches would indicate one-way passages (rare but possible).
	mismatches := 0
	for y := 0; y < pm.Height; y++ {
		for x := 0; x < pm.Width; x++ {
			for _, dir := range []int{2, 4, 6, 8} {
				dx, dy := dirDelta(dir)
				nx, ny := x+dx, y+dy
				if nx < 0 || nx >= pm.Width || ny < 0 || ny >= pm.Height {
					continue
				}
				srcCanLeave := pm.CanPass(x, y, dir)
				dstCanEnter := pm.CanPass(nx, ny, reverseDir(dir))
				if srcCanLeave != dstCanEnter {
					mismatches++
					if mismatches <= 5 {
						t.Logf("  One-way passage: (%d,%d) dir=%d leave=%v enter=%v",
							x, y, dir, srcCanLeave, dstCanEnter)
					}
				}
			}
		}
	}
	t.Logf("Total one-way passages: %d (these may be intentional counters or gates)", mismatches)
}

// ========================================================================
// Test: Region restrictions on Map 5
// ========================================================================

func TestRealData_Map5_RegionRestrictions(t *testing.T) {
	rl := loadRealResources(t)

	pm := rl.Passability[5]
	require.NotNil(t, pm)

	if rl.RegionRestr == nil {
		t.Log("No region restrictions configured")
		return
	}

	t.Logf("Region restrictions: EventRestrict=%v AllRestrict=%v EventAllow=%v AllAllow=%v",
		rl.RegionRestr.EventRestrict, rl.RegionRestr.AllRestrict,
		rl.RegionRestr.EventAllow, rl.RegionRestr.AllAllow)

	// Count tiles with restricted regions
	restricted := 0
	for y := 0; y < pm.Height; y++ {
		for x := 0; x < pm.Width; x++ {
			region := pm.RegionAt(x, y)
			if region > 0 && rl.RegionRestr.IsEventRestricted(region) {
				restricted++
			}
		}
	}
	t.Logf("Map 5: %d tiles with event-restricted regions", restricted)
}

// ========================================================================
// Test: All NPCs on Map 5 can't walk through blocked tiles
// ========================================================================

func TestRealData_Map5_NoNPCWalksThroughWalls(t *testing.T) {
	rl := loadRealResources(t)
	state := NewGameState(nil, nil)
	room := newMapRoom(5, rl, state, zap.NewNop())

	violations := 0
	for _, npc := range room.npcs {
		if npc.ActivePage == nil || npc.ActivePage.MoveType == 0 {
			continue
		}
		if npc.ActivePage.Through {
			continue // Through NPCs intentionally walk through walls
		}

		origX, origY := npc.X, npc.Y
		for _, dir := range []int{2, 4, 6, 8} {
			npc.X = origX
			npc.Y = origY
			npc.dirty = false

			dx, dy := dirDelta(dir)
			destX, destY := origX+dx, origY+dy

			// Check if destination is in bounds
			if destX < 0 || destX >= room.passMap.Width || destY < 0 || destY >= room.passMap.Height {
				// tryMoveNPC should return false
				if room.tryMoveNPC(npc, dir) {
					t.Errorf("NPC[%d] moved out of bounds from (%d,%d) dir=%d", npc.EventID, origX, origY, dir)
					violations++
				}
				npc.X = origX
				npc.Y = origY
				continue
			}

			// Check passability
			srcPass := room.passMap.CanPass(origX, origY, dir)
			dstPass := room.passMap.CanPass(destX, destY, reverseDir(dir))

			// Check region restrictions
			regionBlocked := false
			if rl.RegionRestr != nil {
				region := room.passMap.RegionAt(destX, destY)
				if rl.RegionRestr.IsEventRestricted(region) {
					regionBlocked = true
				}
			}

			moved := room.tryMoveNPC(npc, dir)

			if moved && (!srcPass || !dstPass) && !regionBlocked {
				// Check region allow override
				regionAllowed := false
				if rl.RegionRestr != nil {
					region := room.passMap.RegionAt(destX, destY)
					if rl.RegionRestr.IsEventAllowed(region) {
						regionAllowed = true
					}
				}
				if !regionAllowed {
					t.Errorf("NPC[%d] %q walked through wall from (%d,%d) to (%d,%d) dir=%d src=%v dst=%v",
						npc.EventID, npc.Name, origX, origY, destX, destY, dir, srcPass, dstPass)
					violations++
				}
			}

			npc.X = origX
			npc.Y = origY
		}
	}

	if violations == 0 {
		t.Log("All non-Through NPCs correctly blocked by walls")
	}
}

// ========================================================================
// Test: NPCSnapshot includes correct data for client rendering
// ========================================================================

func TestRealData_Map5_NPCSnapshot(t *testing.T) {
	rl := loadRealResources(t)
	state := NewGameState(nil, nil)
	room := newMapRoom(5, rl, state, zap.NewNop())

	snap := room.NPCSnapshot()
	t.Logf("NPCSnapshot: %d entries", len(snap))

	// Verify each snapshot entry has required fields
	for _, s := range snap {
		eventID, _ := s["event_id"].(int)
		walkName, _ := s["walk_name"].(string)
		x, _ := s["x"].(int)
		y, _ := s["y"].(int)

		assert.NotZero(t, eventID, "event_id should be set")

		// Log invisible events that have non-default priority (this is valid —
		// some events have no sprite but still have priority set in their page).
		if walkName == "" {
			pt, _ := s["priority_type"].(int)
			if pt != 0 {
				t.Logf("  Invisible NPC event_id=%d has priority_type=%d (expected 0 for invisible)", eventID, pt)
			}
		}

		// Position should be within map bounds
		assert.True(t, x >= 0 && x < room.passMap.Width, "x=%d out of bounds", x)
		assert.True(t, y >= 0 && y < room.passMap.Height, "y=%d out of bounds", y)
	}
}
