package world

// npc_runtime_te2_test.go — additional branch coverage for remaining gaps:
// selectPage, populateNPCs, findTransferInPage, ExtractIconOnEvent,
// NPCSnapshot, GetTransferAt, GetEntryPoints, GetTransferAtForPlayer,
// GetTouchEventAtForPlayer.

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// selectPage: remaining branches
// ---------------------------------------------------------------------------

func TestSelectPage_NilState_ReturnsFirstPage(t *testing.T) {
	page := &resource.EventPage{Trigger: 0}
	ev := &resource.MapEvent{ID: 1, Pages: []*resource.EventPage{page}}

	got := selectPage(ev, 1, nil)
	assert.Equal(t, page, got)
}

func TestSelectPage_EmptyPages_ReturnsNil(t *testing.T) {
	ev := &resource.MapEvent{ID: 1, Pages: nil}
	assert.Nil(t, selectPage(ev, 1, newTEState()))
}

func TestSelectPage_NilPageInList_Skipped(t *testing.T) {
	// nil page in the pages list should be skipped
	realPage := &resource.EventPage{Trigger: 0}
	ev := &resource.MapEvent{
		ID:    1,
		Pages: []*resource.EventPage{realPage, nil},
	}
	// RMMV: last page checked first → nil at index 1 skipped → realPage at index 0 returned
	got := selectPage(ev, 1, newTEState())
	assert.Equal(t, realPage, got)
}

func TestSelectPage_TEConditionFalse_ReturnsNil(t *testing.T) {
	// Page has a \TE{} condition that fails
	page := &resource.EventPage{
		Trigger: 0,
		List: []*resource.EventCommand{
			comment(`\TE{1 == 2}`), // always false
			{Code: 0},
		},
	}
	ev := &resource.MapEvent{ID: 1, Pages: []*resource.EventPage{page}}
	assert.Nil(t, selectPage(ev, 1, newTEState()))
}

// ---------------------------------------------------------------------------
// populateNPCs: key paths
// ---------------------------------------------------------------------------

func TestPopulateNPCs_BasicEvent(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)

	room.res.Maps[1] = &resource.MapData{
		ID: 1, Width: 10, Height: 10,
		Events: []*resource.MapEvent{
			nil,
			{
				ID: 1, Name: "Guard", X: 3, Y: 4,
				Pages: []*resource.EventPage{
					{Image: resource.EventImage{CharacterName: "Actor1", Direction: 2}},
				},
			},
		},
	}

	room.populateNPCs()
	require.Len(t, room.npcs, 1)
	assert.Equal(t, 1, room.npcs[0].EventID)
	assert.Equal(t, 3, room.npcs[0].X)
	assert.Equal(t, 4, room.npcs[0].Y)
}

func TestPopulateNPCs_NilRes_NoOp(t *testing.T) {
	room := &MapRoom{res: nil, npcs: []*NPCRuntime{}, logger: zap.NewNop()}
	room.populateNPCs()
	assert.Empty(t, room.npcs)
}

func TestPopulateNPCs_MapNotFound_NoOp(t *testing.T) {
	room := newTestRoom(t, 99, 10, 10) // map 99 not in res.Maps
	room.populateNPCs()
	assert.Empty(t, room.npcs)
}

func TestPopulateNPCs_NilEvent_Skipped(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	room.res.Maps[1] = &resource.MapData{
		ID: 1, Width: 10, Height: 10,
		Events: []*resource.MapEvent{nil, nil},
	}
	room.populateNPCs()
	assert.Empty(t, room.npcs)
}

func TestPopulateNPCs_EventWithOriginalPages_Logged(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	room.res.Maps[1] = &resource.MapData{
		ID: 1, Width: 10, Height: 10,
		Events: []*resource.MapEvent{
			nil,
			{
				ID: 1, Name: "Door", X: 2, Y: 3,
				Pages: []*resource.EventPage{
					{Image: resource.EventImage{CharacterName: "!Door", Direction: 4}},
				},
				OriginalPages: []*resource.EventPage{
					{List: []*resource.EventCommand{{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}}}},
				},
			},
		},
	}

	room.populateNPCs()
	require.Len(t, room.npcs, 1)
	assert.Equal(t, 1, room.npcs[0].EventID)
}

func TestPopulateNPCs_RandomPos_WithPassMap(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// Map has <RandomPos: 2> meta
	room.res.Maps[1] = &resource.MapData{
		ID: 1, Width: 10, Height: 10,
		Note: "<RandomPos: 2>",
		Events: []*resource.MapEvent{
			nil,
			{
				ID: 1, Name: "Wanderer", X: 5, Y: 5,
				Note: "<Random>", // Random tag → should be randomly positioned
				Pages: []*resource.EventPage{
					{Image: resource.EventImage{CharacterName: "Actor1"}},
				},
			},
		},
	}

	room.populateNPCs()
	require.Len(t, room.npcs, 1)
	// Position might change from original (5,5) if random position is found
	// Just verify it was populated
	assert.Equal(t, 1, room.npcs[0].EventID)
}

func TestPopulateNPCs_DefaultDirection(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	room.res.Maps[1] = &resource.MapData{
		ID: 1, Width: 10, Height: 10,
		Events: []*resource.MapEvent{
			nil,
			{
				ID: 1, X: 2, Y: 2,
				Pages: []*resource.EventPage{
					{Image: resource.EventImage{Direction: 0}}, // Direction=0 → use default 2
				},
			},
		},
	}

	room.populateNPCs()
	require.Len(t, room.npcs, 1)
	assert.Equal(t, 2, room.npcs[0].Dir) // default down
}

func TestPopulateNPCs_ActivePageDirection(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	room.res.Maps[1] = &resource.MapData{
		ID: 1, Width: 10, Height: 10,
		Events: []*resource.MapEvent{
			nil,
			{
				ID: 1, X: 2, Y: 2,
				Pages: []*resource.EventPage{
					{Image: resource.EventImage{Direction: 8}}, // facing up
				},
			},
		},
	}

	room.populateNPCs()
	require.Len(t, room.npcs, 1)
	assert.Equal(t, 8, room.npcs[0].Dir)
}

// ---------------------------------------------------------------------------
// findTransferInPage: remaining branches
// ---------------------------------------------------------------------------

func TestFindTransferInPage_IndentedTransfer_Skipped(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// Transfer at indent=1 (inside an if branch) — should be skipped
			{Code: 201, Indent: 1, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}},
			{Code: 0},
		},
	}
	assert.Nil(t, findTransferInPage(page))
}

func TestFindTransferInPage_ShortParameters_Skipped(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// Only 3 parameters (need ≥5)
			{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1)}},
			{Code: 0},
		},
	}
	assert.Nil(t, findTransferInPage(page))
}

func TestFindTransferInPage_VariableTransfer_Skipped(t *testing.T) {
	// mode != 0 means variable-based transfer — skip it
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(1), float64(5), float64(1), float64(1), float64(2)}},
			{Code: 0},
		},
	}
	assert.Nil(t, findTransferInPage(page))
}

func TestFindTransferInPage_NilCmd_Skipped(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			nil,
			{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}},
		},
	}
	td := findTransferInPage(page)
	require.NotNil(t, td)
	assert.Equal(t, 5, td.MapID)
}

// ---------------------------------------------------------------------------
// ExtractIconOnEvent: remaining branches
// ---------------------------------------------------------------------------

func TestExtractIconOnEvent_NoteEmpty_NoMatch(t *testing.T) {
	ev := &resource.MapEvent{Note: ""}
	assert.Equal(t, 0, ExtractIconOnEvent(ev, nil))
}

func TestExtractIconOnEvent_NoteWithIcon_Returned(t *testing.T) {
	ev := &resource.MapEvent{Note: "<Icon on Event: 128>"}
	assert.Equal(t, 128, ExtractIconOnEvent(ev, nil))
}

func TestExtractIconOnEvent_PageCmd_NonStringParam_Skipped(t *testing.T) {
	ev := &resource.MapEvent{Note: ""}
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 108, Parameters: []interface{}{42}}, // non-string
			{Code: 0},
		},
	}
	assert.Equal(t, 0, ExtractIconOnEvent(ev, page))
}

func TestExtractIconOnEvent_PageCmd_EmptyParams_Skipped(t *testing.T) {
	ev := &resource.MapEvent{Note: ""}
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 108, Parameters: []interface{}{}}, // empty
			{Code: 0},
		},
	}
	assert.Equal(t, 0, ExtractIconOnEvent(ev, page))
}

func TestExtractIconOnEvent_PageCmd_WithIcon(t *testing.T) {
	ev := &resource.MapEvent{Note: ""}
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("<Icon on Event: 64>"),
			{Code: 0},
		},
	}
	assert.Equal(t, 64, ExtractIconOnEvent(ev, page))
}

func TestExtractIconOnEvent_PageCmd_NonCommentCode_Skipped(t *testing.T) {
	ev := &resource.MapEvent{Note: ""}
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 101, Parameters: []interface{}{"<Icon on Event: 64>"}}, // code 101 not 108/408
			{Code: 0},
		},
	}
	assert.Equal(t, 0, ExtractIconOnEvent(ev, page))
}

// ---------------------------------------------------------------------------
// NPCSnapshot: remaining paths (tile_id, icon, label, no active page)
// ---------------------------------------------------------------------------

func TestNPCSnapshot_WithTileID(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Image: resource.EventImage{TileID: 3456, CharacterName: "irrelevant"},
	}
	npc := newNPC(1, 2, 2, page)
	room.AddNPC(npc)

	snaps := room.NPCSnapshot()
	require.Len(t, snaps, 1)
	assert.Equal(t, 3456, snaps[0]["tile_id"])
	assert.Equal(t, "", snaps[0]["walk_name"])
}

func TestNPCSnapshot_WithMiniLabel(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("<Mini Label: NPC>"),
			{Code: 0},
		},
	}
	npc := newNPC(1, 2, 2, page)
	room.AddNPC(npc)

	snaps := room.NPCSnapshot()
	require.Len(t, snaps, 1)
	assert.Equal(t, "NPC", snaps[0]["label"])
}

func TestNPCSnapshot_WithIconOnEvent(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{}
	npc := newNPC(1, 2, 2, page)
	npc.MapEvent.Note = "<Icon on Event: 96>"
	room.AddNPC(npc)

	snaps := room.NPCSnapshot()
	require.Len(t, snaps, 1)
	assert.Equal(t, 96, snaps[0]["icon_on_event"])
}

func TestNPCSnapshot_NoActivePage_DefaultFields(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := &NPCRuntime{
		EventID:    1,
		Name:       "Ghost",
		X:          2, Y: 2,
		Dir:        4,
		ActivePage: nil,
		MapEvent:   &resource.MapEvent{ID: 1},
	}
	room.AddNPC(npc)

	snaps := room.NPCSnapshot()
	require.Len(t, snaps, 1)
	assert.Equal(t, "", snaps[0]["walk_name"])
	assert.Equal(t, 0, snaps[0]["priority_type"])
	assert.Equal(t, false, snaps[0]["step_anime"])
}

func TestNPCSnapshot_FunctionalMarker_Hidden(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Image: resource.EventImage{CharacterName: "event_mark"},
	}
	npc := newNPC(1, 2, 2, page)
	room.AddNPC(npc)

	snaps := room.NPCSnapshot()
	require.Len(t, snaps, 1)
	assert.Equal(t, "", snaps[0]["walk_name"])
}

// ---------------------------------------------------------------------------
// GetTransferAt: remaining branches
// ---------------------------------------------------------------------------

func TestGetTransferAt_ActiveTriggerNotTouch_Skipped(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// Trigger 0 = action button — not touch, should be skipped
	page := &resource.EventPage{
		Trigger: 0,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}},
			{Code: 0},
		},
	}
	npc := newNPC(1, 3, 3, page)
	room.AddNPC(npc)

	assert.Nil(t, room.GetTransferAt(3, 3))
}

func TestGetTransferAt_ActivePage_NilOriginalPage_Skipped(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// Template page: has TE_CALL_ORIGIN_EVENT
	templatePage := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 356, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}},
			{Code: 0},
		},
	}
	ev := &resource.MapEvent{
		ID:            1, X: 4, Y: 4,
		Pages:         []*resource.EventPage{templatePage},
		OriginalPages: []*resource.EventPage{nil}, // nil original page → skipped
	}
	npc := &NPCRuntime{
		EventID: 1, X: 4, Y: 4,
		ActivePage: templatePage,
		MapEvent:   ev,
	}
	room.AddNPC(npc)

	// nil original page is skipped, no transfer found → returns nil
	assert.Nil(t, room.GetTransferAt(4, 4))
}

func TestGetTransferAt_NilMapEvent_Skipped(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := &NPCRuntime{EventID: 1, X: 2, Y: 2, MapEvent: nil}
	room.AddNPC(npc)

	assert.Nil(t, room.GetTransferAt(2, 2))
}

// ---------------------------------------------------------------------------
// GetEntryPoints: remaining branches
// ---------------------------------------------------------------------------

func TestGetEntryPoints_NilMapEvent_Skipped(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := &NPCRuntime{EventID: 1, X: 2, Y: 2, MapEvent: nil}
	room.AddNPC(npc)

	points := room.GetEntryPoints()
	assert.Empty(t, points)
}

func TestGetEntryPoints_DeduplicatesSamePosition(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// Two pages at same NPC position — only one entry point
	transferCmd := &resource.EventCommand{
		Code:       201,
		Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)},
	}
	page1 := &resource.EventPage{Trigger: 1, List: []*resource.EventCommand{transferCmd, {Code: 0}}}
	page2 := &resource.EventPage{Trigger: 2, List: []*resource.EventCommand{transferCmd, {Code: 0}}}

	ev := &resource.MapEvent{
		ID: 1, X: 5, Y: 5,
		Pages: []*resource.EventPage{page1, page2},
	}
	npc := &NPCRuntime{EventID: 1, X: 5, Y: 5, ActivePage: page1, MapEvent: ev}
	room.AddNPC(npc)

	points := room.GetEntryPoints()
	assert.Len(t, points, 1)
	assert.Equal(t, 5, points[0].X)
	assert.Equal(t, 5, points[0].Y)
}

func TestGetEntryPoints_NonTouchTrigger_Skipped(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// Trigger 0 = action button — not included in entry points
	page := &resource.EventPage{
		Trigger: 0,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}},
		},
	}
	ev := &resource.MapEvent{ID: 1, X: 3, Y: 3, Pages: []*resource.EventPage{page}}
	npc := &NPCRuntime{EventID: 1, X: 3, Y: 3, ActivePage: page, MapEvent: ev}
	room.AddNPC(npc)

	points := room.GetEntryPoints()
	assert.Empty(t, points)
}

// ---------------------------------------------------------------------------
// GetTransferAtForPlayer: nil original page in OriginalPages
// ---------------------------------------------------------------------------

func TestGetTransferAtForPlayer_NilOriginalPage_Skipped(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	templatePage := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 356, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}},
			{Code: 0},
		},
	}
	origPage := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(7), float64(5), float64(5), float64(2)}},
			{Code: 0},
		},
	}
	ev := &resource.MapEvent{
		ID:            1,
		X:             4, Y: 4,
		Pages:         []*resource.EventPage{templatePage},
		OriginalPages: []*resource.EventPage{nil, origPage}, // nil first, real second
	}
	npc := &NPCRuntime{
		EventID:    1,
		X:          4, Y: 4,
		ActivePage: templatePage,
		MapEvent:   ev,
	}
	room.AddNPC(npc)

	td := room.GetTransferAtForPlayer(4, 4, newTEState())
	require.NotNil(t, td)
	assert.Equal(t, 7, td.MapID)
}

// ---------------------------------------------------------------------------
// GetTouchEventAtForPlayer: nil MapEvent
// ---------------------------------------------------------------------------

func TestGetTouchEventAtForPlayer_NilMapEvent_Skipped(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := &NPCRuntime{EventID: 1, X: 3, Y: 3, MapEvent: nil}
	room.AddNPC(npc)

	id, pg := room.GetTouchEventAtForPlayer(3, 3, newTEState())
	assert.Equal(t, 0, id)
	assert.Nil(t, pg)
}

func TestGetTouchEventAtForPlayer_EmptyPages_Skipped(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := &NPCRuntime{
		EventID:  1,
		X:        3, Y: 3,
		MapEvent: &resource.MapEvent{ID: 1, Pages: nil},
	}
	room.AddNPC(npc)

	id, pg := room.GetTouchEventAtForPlayer(3, 3, newTEState())
	assert.Equal(t, 0, id)
	assert.Nil(t, pg)
}

// ---------------------------------------------------------------------------
// extractMetaInt: additional paths
// ---------------------------------------------------------------------------

func TestExtractMetaInt_NoMatch(t *testing.T) {
	v := extractMetaInt("<Other: 5>", "RandomPos")
	assert.Equal(t, 0, v)
}

func TestExtractMetaInt_Match(t *testing.T) {
	v := extractMetaInt("<RandomPos: 3>", "RandomPos")
	assert.Equal(t, 3, v)
}
