package npc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================================================================
// Issue 1: TE_CALL_ORIGIN_EVENT page index off-by-one
// TemplateEvent.js uses: pages[pageIndex - 1 || this._pageIndex]
// When pageIndex=1, JS evaluates (1-1)=0 which is falsy → uses _pageIndex.
// Server must use 1-based indexing: arg=1→page 0, arg=2→page 1, etc.
// arg=0 or omitted → page 0 (default).
// ========================================================================

func TestTECallOriginEvent_PageIndex1_UsesFirstPage(t *testing.T) {
	// Setup: event with 3 original pages
	page0Cmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(100), float64(100), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page1Cmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(200), float64(200), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page2Cmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(300), float64(300), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	resLoader := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			68: {
				ID: 68,
				Events: []*resource.MapEvent{
					nil,
					{
						ID: 1, Name: "TestTE",
						Pages: []*resource.EventPage{{List: nil}},
						OriginalPages: []*resource.EventPage{
							{List: page0Cmds},
							{List: page1Cmds},
							{List: page2Cmds},
						},
					},
				},
			},
		},
		CommonEvents: make([]*resource.CommonEvent, 2),
	}

	e := New(nil, resLoader, nopLogger())
	gs := newMockGameState()

	// TE_CALL_ORIGIN_EVENT 1 → should execute page 0 (first page)
	// TemplateEvent.js: pages[(1-1) || _pageIndex] = pages[0 || _pageIndex]
	// Since 0 is falsy, falls back to _pageIndex. In common case _pageIndex=0.
	// Server approx: arg=1 → page 0.
	s := testSession(1)
	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT 1"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: gs, MapID: 68, EventID: 1,
	})

	// page0 sets switch 100 ON
	assert.True(t, gs.GetSwitch(100), "TE_CALL_ORIGIN_EVENT 1 should execute original page 0 (switch 100)")
	assert.False(t, gs.GetSwitch(200), "should NOT execute page 1 (switch 200)")
	assert.False(t, gs.GetSwitch(300), "should NOT execute page 2 (switch 300)")
}

func TestTECallOriginEvent_PageIndex2_UsesSecondPage(t *testing.T) {
	page0Cmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(100), float64(100), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page1Cmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(200), float64(200), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	resLoader := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			68: {
				ID: 68,
				Events: []*resource.MapEvent{
					nil,
					{
						ID: 1, Name: "TestTE",
						Pages:         []*resource.EventPage{{List: nil}},
						OriginalPages: []*resource.EventPage{{List: page0Cmds}, {List: page1Cmds}},
					},
				},
			},
		},
		CommonEvents: make([]*resource.CommonEvent, 2),
	}

	e := New(nil, resLoader, nopLogger())
	gs := newMockGameState()
	s := testSession(1)

	// TE_CALL_ORIGIN_EVENT 2 → pages[(2-1)] = pages[1] = page 2 (0-indexed: 1)
	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT 2"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: gs, MapID: 68, EventID: 1,
	})

	assert.False(t, gs.GetSwitch(100), "should NOT execute page 0")
	assert.True(t, gs.GetSwitch(200), "TE_CALL_ORIGIN_EVENT 2 should execute original page 1 (switch 200)")
}

func TestTECallOriginEvent_NoArg_UsesFirstPage(t *testing.T) {
	page0Cmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(100), float64(100), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	resLoader := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			68: {
				ID: 68,
				Events: []*resource.MapEvent{
					nil,
					{
						ID: 1, Pages: []*resource.EventPage{{List: nil}},
						OriginalPages: []*resource.EventPage{{List: page0Cmds}},
					},
				},
			},
		},
		CommonEvents: make([]*resource.CommonEvent, 2),
	}

	e := New(nil, resLoader, nopLogger())
	gs := newMockGameState()
	s := testSession(1)

	// TE_CALL_ORIGIN_EVENT (no page arg) → page 0
	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: gs, MapID: 68, EventID: 1,
	})

	assert.True(t, gs.GetSwitch(100), "no arg should default to page 0")
}

// ========================================================================
// Issue 2: TE_CALL_MAP_EVENT searches wrong scope
// TemplateEvent.js searches the CURRENT MAP's events by ID or name.
// Server should search the current map (opts.MapID), not all maps.
// Also, numeric eventId should be looked up by event ID, not name.
// ========================================================================

func TestTECallMapEvent_SearchesCurrentMap(t *testing.T) {
	targetCmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(500), float64(500), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	wrongCmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(600), float64(600), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	resLoader := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			68: {
				ID: 68,
				Events: []*resource.MapEvent{
					nil,
					{ID: 1, Name: "Caller", Pages: []*resource.EventPage{{List: nil}}},
					{ID: 2, Name: "TargetEvent", Pages: []*resource.EventPage{{List: targetCmds}}},
				},
			},
			99: {
				ID: 99,
				Events: []*resource.MapEvent{
					nil,
					{ID: 1, Name: "TargetEvent", Pages: []*resource.EventPage{{List: wrongCmds}}},
				},
			},
		},
		CommonEvents: make([]*resource.CommonEvent, 2),
	}

	e := New(nil, resLoader, nopLogger())
	gs := newMockGameState()
	s := testSession(1)

	// TE_CALL_MAP_EVENT TargetEvent 1 → should find event in map 68, NOT map 99
	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT TargetEvent 1"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: gs, MapID: 68, EventID: 1,
	})

	assert.True(t, gs.GetSwitch(500), "should execute event from current map 68 (switch 500)")
	assert.False(t, gs.GetSwitch(600), "should NOT execute event from map 99 (switch 600)")
}

func TestTECallMapEvent_ByEventId(t *testing.T) {
	targetCmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(700), float64(700), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	resLoader := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			68: {
				ID: 68,
				Events: []*resource.MapEvent{
					nil,
					{ID: 1, Name: "Caller", Pages: []*resource.EventPage{{List: nil}}},
					nil, nil, nil,
					{ID: 5, Name: "SomeEvent", Pages: []*resource.EventPage{{List: targetCmds}}},
				},
			},
		},
		CommonEvents: make([]*resource.CommonEvent, 2),
	}

	e := New(nil, resLoader, nopLogger())
	gs := newMockGameState()
	s := testSession(1)

	// TE_CALL_MAP_EVENT 5 1 → should find event ID 5 in current map
	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT 5 1"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: gs, MapID: 68, EventID: 1,
	})

	assert.True(t, gs.GetSwitch(700), "should execute event ID 5 in current map")
}

// ========================================================================
// Issue 3: TE_SET_SELF_VARIABLE \sv[N] operand resolution
// TemplateEvent.js: convertAllSelfVariables replaces \sv[N] in all args.
// Server should resolve \sv[N] in operand before parsing as int.
// ========================================================================

func TestTESetSelfVariable_SvReference(t *testing.T) {
	resLoader := &resource.ResourceLoader{
		CommonEvents: make([]*resource.CommonEvent, 2),
	}

	e := New(nil, resLoader, nopLogger())
	gs := newMockGameState()
	gs.SetSelfVariable(68, 1, 1, 42) // \sv[1] = 42

	s := testSession(1)

	// TE_SET_SELF_VARIABLE 5 0 \sv[1] → set sv[5] = value of sv[1] = 42
	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_SELF_VARIABLE 5 0 \\sv[1]"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: gs, MapID: 68, EventID: 1,
	})

	assert.Equal(t, 42, gs.GetSelfVariable(68, 1, 5), "sv[5] should be set to value of sv[1] (42)")
}

func TestTESetSelfVariable_VarReference(t *testing.T) {
	resLoader := &resource.ResourceLoader{
		CommonEvents: make([]*resource.CommonEvent, 2),
	}

	e := New(nil, resLoader, nopLogger())
	gs := newMockGameState()
	gs.SetVariable(10, 99) // \v[10] = 99

	s := testSession(1)

	// TE_SET_SELF_VARIABLE 3 0 \v[10] → set sv[3] = value of var[10] = 99
	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"TE_SET_SELF_VARIABLE 3 0 \\v[10]"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: gs, MapID: 68, EventID: 1,
	})

	assert.Equal(t, 99, gs.GetSelfVariable(68, 1, 3), "sv[3] should be set to value of var[10] (99)")
}

// ========================================================================
// TE_CALL_MAP_EVENT page index: same JS quirk as TE_CALL_ORIGIN_EVENT
// ========================================================================

func TestTECallMapEvent_PageIndex1_UsesFirstPage(t *testing.T) {
	page0Cmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(800), float64(800), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page1Cmds := []*resource.EventCommand{
		{Code: CmdChangeSwitches, Parameters: []interface{}{float64(900), float64(900), float64(0)}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	resLoader := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			68: {
				ID: 68,
				Events: []*resource.MapEvent{
					nil,
					{ID: 1, Name: "Caller", Pages: []*resource.EventPage{{List: nil}}},
					{ID: 2, Name: "Target", Pages: []*resource.EventPage{
						{List: page0Cmds}, {List: page1Cmds},
					}},
				},
			},
		},
		CommonEvents: make([]*resource.CommonEvent, 2),
	}

	e := New(nil, resLoader, nopLogger())
	gs := newMockGameState()
	s := testSession(1)

	// TE_CALL_MAP_EVENT Target 1 → page index 1 → pages[0] (1-based)
	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_MAP_EVENT Target 1"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}
	page := &resource.EventPage{List: cmds}
	e.Execute(context.Background(), s, page, &ExecuteOpts{
		GameState: gs, MapID: 68, EventID: 1,
	})

	assert.True(t, gs.GetSwitch(800), "page index 1 should map to pages[0]")
	assert.False(t, gs.GetSwitch(900), "should NOT execute pages[1]")
}

// ========================================================================
// Regression: dialog_end is sent after TE_CALL_ORIGIN_EVENT
// ========================================================================
func TestTECallOriginEvent_DialogEnd(t *testing.T) {
	originCmds := []*resource.EventCommand{
		{Code: CmdShowText, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
		{Code: CmdShowTextLine, Parameters: []interface{}{"Hello from origin"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	resLoader := &resource.ResourceLoader{
		Maps: map[int]*resource.MapData{
			68: {
				ID: 68,
				Events: []*resource.MapEvent{
					nil,
					{
						ID: 1, Pages: []*resource.EventPage{{List: nil}},
						OriginalPages: []*resource.EventPage{{List: originCmds}},
					},
				},
			},
		},
		CommonEvents: make([]*resource.CommonEvent, 2),
	}

	e := New(nil, resLoader, nopLogger())
	s := testSession(1)

	cmds := []*resource.EventCommand{
		{Code: CmdPluginCommand, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT 1"}},
		{Code: CmdEnd, Parameters: []interface{}{}},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		page := &resource.EventPage{List: cmds}
		e.Execute(context.Background(), s, page, &ExecuteOpts{
			GameState: newMockGameState(), MapID: 68, EventID: 1,
		})
	}()

	// Wait for dialog
	pkt := recvPacket(t, s, 2*time.Second)
	require.Equal(t, "npc_dialog", pkt.Type)

	var data map[string]interface{}
	json.Unmarshal(pkt.Payload, &data)
	lines := data["lines"].([]interface{})
	assert.Equal(t, "Hello from origin", lines[0])

	// Ack dialog
	s.DialogAckCh <- struct{}{}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}
