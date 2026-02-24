package npc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---- Helpers ----

func nopLogger() *zap.Logger { return zap.NewNop() }

// testSession creates a PlayerSession with no real WebSocket connection.
// Uses a buffered SendChan so we can inspect sent packets.
func testSession(charID int64) *player.PlayerSession {
	return &player.PlayerSession{
		CharID:       charID,
		AccountID:    1,
		SendChan:     make(chan []byte, 64),
		Done:         make(chan struct{}),
		ChoiceCh:     make(chan int, 1),
		DialogAckCh:  make(chan struct{}, 1),
		SceneReadyCh: make(chan struct{}, 1),
	}
}

// recvPacket reads the next packet from the session's SendChan.
func recvPacket(t *testing.T, s *player.PlayerSession, timeout time.Duration) *player.Packet {
	t.Helper()
	select {
	case data := <-s.SendChan:
		var pkt player.Packet
		require.NoError(t, json.Unmarshal(data, &pkt))
		return &pkt
	case <-time.After(timeout):
		t.Fatal("timeout waiting for packet")
		return nil
	}
}

// drainPackets reads all available packets from the session.
func drainPackets(t *testing.T, s *player.PlayerSession) []*player.Packet {
	t.Helper()
	var pkts []*player.Packet
	for {
		select {
		case data := <-s.SendChan:
			var pkt player.Packet
			require.NoError(t, json.Unmarshal(data, &pkt))
			pkts = append(pkts, &pkt)
		default:
			return pkts
		}
	}
}

// mockGameState implements GameStateAccessor for testing.
type mockGameState struct {
	switches      map[int]bool
	variables     map[int]int
	selfSwitches  map[string]bool // key: "mapID_eventID_ch"
	selfVariables map[string]int  // key: "mapID_eventID_index" for TemplateEvent.js
}

func newMockGameState() *mockGameState {
	return &mockGameState{
		switches:      make(map[int]bool),
		variables:     make(map[int]int),
		selfSwitches:  make(map[string]bool),
		selfVariables: make(map[string]int),
	}
}

func (m *mockGameState) GetSwitch(id int) bool         { return m.switches[id] }
func (m *mockGameState) SetSwitch(id int, val bool)     { m.switches[id] = val }
func (m *mockGameState) GetVariable(id int) int          { return m.variables[id] }
func (m *mockGameState) SetVariable(id int, val int)     { m.variables[id] = val }
func (m *mockGameState) GetSelfSwitch(mapID, eventID int, ch string) bool {
	key := selfSwitchKey(mapID, eventID, ch)
	return m.selfSwitches[key]
}
func (m *mockGameState) SetSelfSwitch(mapID, eventID int, ch string, val bool) {
	key := selfSwitchKey(mapID, eventID, ch)
	m.selfSwitches[key] = val
}

func (m *mockGameState) GetSelfVariable(mapID, eventID, index int) int {
	key := selfVariableKey(mapID, eventID, index)
	return m.selfVariables[key]
}

func (m *mockGameState) SetSelfVariable(mapID, eventID, index, val int) {
	key := selfVariableKey(mapID, eventID, index)
	m.selfVariables[key] = val
}

func selfSwitchKey(mapID, eventID int, ch string) string {
	return string(rune(mapID)) + "_" + string(rune(eventID)) + "_" + ch
}

func selfVariableKey(mapID, eventID, index int) string {
	return string(rune(mapID)) + "_" + string(rune(eventID)) + "_" + string(rune(index))
}

// ========================================================================
// Tests for basic ShowText execution
// ========================================================================

func TestExecute_ShowText_SendsDialog(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowText, Parameters: []interface{}{"Actor1", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Parameters: []interface{}{"Hello!"}},
			{Code: CmdShowTextLine, Parameters: []interface{}{"How are you?"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	// Pre-fill the dialog ack so the executor doesn't block.
	s.DialogAckCh <- struct{}{}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	require.True(t, len(pkts) >= 2, "Expected at least dialog + dialog_end packets, got %d", len(pkts))

	// First packet should be npc_dialog
	assert.Equal(t, "npc_dialog", pkts[0].Type)

	var dialogData map[string]interface{}
	require.NoError(t, json.Unmarshal(pkts[0].Payload, &dialogData))
	assert.Equal(t, "Actor1", dialogData["face"])

	lines, ok := dialogData["lines"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, "Hello!", lines[0])
	assert.Equal(t, "How are you?", lines[1])

	// Last packet should be npc_dialog_end
	assert.Equal(t, "npc_dialog_end", pkts[len(pkts)-1].Type)
}

// ========================================================================
// Tests for ShowChoices execution
// ========================================================================

func TestExecute_ShowChoices_SendsChoicesAndWaits(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ShowChoices: choices=["Yes","No"], cancelType=-1
			{Code: CmdShowChoices, Indent: 0, Parameters: []interface{}{
				[]interface{}{"Yes", "No"}, float64(-1),
			}},
			// When [0] = Yes
			{Code: CmdWhenBranch, Indent: 0, Parameters: []interface{}{float64(0)}},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"You said Yes!"}},
			// When [1] = No
			{Code: CmdWhenBranch, Indent: 0, Parameters: []interface{}{float64(1)}},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"You said No!"}},
			// Branch End
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}

	// Simulate player choosing "Yes" (index 0)
	go func() {
		// Wait a bit for the executor to send choices
		time.Sleep(50 * time.Millisecond)
		s.ChoiceCh <- 0
		// Then ack the resulting dialog
		time.Sleep(50 * time.Millisecond)
		s.DialogAckCh <- struct{}{}
	}()

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	types := make([]string, len(pkts))
	for i, p := range pkts {
		types[i] = p.Type
	}

	assert.Contains(t, types, "npc_choices", "Should send npc_choices packet")
	assert.Contains(t, types, "npc_dialog", "Should send dialog for the chosen branch")

	// Check that the dialog was "You said Yes!" (branch 0)
	for _, pkt := range pkts {
		if pkt.Type == "npc_dialog" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			lines := data["lines"].([]interface{})
			assert.Equal(t, "You said Yes!", lines[0])
		}
	}
}

// ========================================================================
// Tests for ConditionalBranch execution
// ========================================================================

func TestExecute_ConditionalBranch_SwitchTrue(t *testing.T) {
	gs := newMockGameState()
	gs.switches[10] = true

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// if Switch[10] == ON
			{Code: CmdConditionalStart, Indent: 0, Parameters: []interface{}{
				float64(0), float64(10), float64(0), // condType=0(switch), switchID=10, expected=0(ON)
			}},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"Switch is ON!"}},
			// Else
			{Code: CmdElseBranch, Indent: 0},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"Switch is OFF!"}},
			// End
			{Code: CmdConditionalEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}

	s.DialogAckCh <- struct{}{}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	pkts := drainPackets(t, s)
	foundDialog := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_dialog" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			lines := data["lines"].([]interface{})
			assert.Equal(t, "Switch is ON!", lines[0])
			foundDialog = true
		}
	}
	assert.True(t, foundDialog, "Should execute IF branch when switch is ON")
}

func TestExecute_ConditionalBranch_SwitchFalse(t *testing.T) {
	gs := newMockGameState()
	// Switch 10 is OFF (default)

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdConditionalStart, Indent: 0, Parameters: []interface{}{
				float64(0), float64(10), float64(0),
			}},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"Switch is ON!"}},
			{Code: CmdElseBranch, Indent: 0},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"Switch is OFF!"}},
			{Code: CmdConditionalEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}

	s.DialogAckCh <- struct{}{}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	pkts := drainPackets(t, s)
	foundDialog := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_dialog" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			lines := data["lines"].([]interface{})
			assert.Equal(t, "Switch is OFF!", lines[0])
			foundDialog = true
		}
	}
	assert.True(t, foundDialog, "Should execute ELSE branch when switch is OFF")
}

// ========================================================================
// Tests for ChangeSwitches / ChangeVars / ChangeSelfSwitch
// ========================================================================

func TestExecute_ChangeSwitches(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeSwitches: [startID=306, endID=306, value=0(ON)]
			{Code: CmdChangeSwitches, Indent: 0, Parameters: []interface{}{
				float64(306), float64(306), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	assert.False(t, gs.switches[306], "Switch 306 should be OFF initially")

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	assert.True(t, gs.switches[306], "Switch 306 should be ON after execution")
}

func TestExecute_ChangeVariables_Add(t *testing.T) {
	gs := newMockGameState()
	gs.variables[206] = 5

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeVars: [startID=206, endID=206, op=1(add), operandType=0(const), operand=3]
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(206), float64(206), float64(1), float64(0), float64(3),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	assert.Equal(t, 8, gs.variables[206], "Variable 206 should be 5 + 3 = 8")
}

func TestExecute_ChangeSelfSwitch(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeSelfSwitch: [ch="A", value=0(ON)]
			{Code: CmdChangeSelfSwitch, Indent: 0, Parameters: []interface{}{
				"A", float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{GameState: gs, MapID: 5, EventID: 20}
	exec.Execute(context.Background(), s, page, opts)

	assert.True(t, gs.GetSelfSwitch(5, 20, "A"), "Self-switch A should be ON after execution")
}

// ========================================================================
// Tests for Transfer Player (command 201)
// ========================================================================

func TestExecute_Transfer_CallsTransferFn(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	var transferCalled bool
	var tMapID, tX, tY, tDir int

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// Transfer: [mode=0(direct), mapID=3, x=10, y=15, dir=4]
			{Code: CmdTransfer, Indent: 0, Parameters: []interface{}{
				float64(0), float64(3), float64(10), float64(15), float64(4),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{
		MapID:   5,
		EventID: 20,
		TransferFn: func(s *player.PlayerSession, mapID, x, y, dir int) {
			transferCalled = true
			tMapID = mapID
			tX = x
			tY = y
			tDir = dir
		},
	}

	exec.Execute(context.Background(), s, page, opts)

	assert.True(t, transferCalled, "Transfer callback should be called")
	assert.Equal(t, 3, tMapID)
	assert.Equal(t, 10, tX)
	assert.Equal(t, 15, tY)
	assert.Equal(t, 4, tDir)
}

func TestExecute_Transfer_NoTransferFn_SendsFallback(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdTransfer, Indent: 0, Parameters: []interface{}{
				float64(0), float64(3), float64(10), float64(15), float64(4),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{MapID: 5, EventID: 20})

	pkts := drainPackets(t, s)
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "transfer_player" {
			found = true
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			assert.Equal(t, float64(3), data["map_id"])
			assert.Equal(t, float64(10), data["x"])
			assert.Equal(t, float64(15), data["y"])
		}
	}
	assert.True(t, found, "Should send transfer_player fallback when no TransferFn")
}

// ========================================================================
// Tests for NPC effect forwarding
// ========================================================================

func TestExecute_PluginCommand_ForwardsAsEffect(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Indent: 0, Parameters: []interface{}{"SomePlugin arg1 arg2"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			found = true
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			assert.Equal(t, float64(CmdPluginCommand), data["code"])
		}
	}
	assert.True(t, found, "Plugin command should be forwarded as npc_effect")
}

func TestExecute_ScreenEffects_ForwardsAsEffect(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdFadeout, Indent: 0, Parameters: []interface{}{float64(30)}},
			{Code: CmdFadein, Indent: 0, Parameters: []interface{}{float64(30)}},
			{Code: CmdPlaySE, Indent: 0, Parameters: []interface{}{
				map[string]interface{}{"name": "Cursor1", "volume": float64(80), "pitch": float64(100), "pan": float64(0)},
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	effectCount := 0
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			effectCount++
		}
	}
	assert.Equal(t, 3, effectCount, "All 3 effect commands should be forwarded")
}

// ========================================================================
// Tests for Wait command
// ========================================================================

func TestExecute_Wait_Pauses(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// Wait 6 frames = 100ms at 60fps
			{Code: CmdWait, Indent: 0, Parameters: []interface{}{float64(6)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	start := time.Now()
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	elapsed := time.Since(start)

	assert.True(t, elapsed >= 90*time.Millisecond, "Wait command should pause execution (~100ms)")
}

// ========================================================================
// Tests for context cancellation
// ========================================================================

func TestExecute_ContextCancel_StopsExecution(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// A long wait that should be cancelled
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdWait, Indent: 0, Parameters: []interface{}{float64(600)}}, // 10 seconds
			{Code: CmdEnd, Indent: 0},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	exec.Execute(ctx, s, page, &ExecuteOpts{})
	elapsed := time.Since(start)

	assert.True(t, elapsed < 500*time.Millisecond, "Execution should be cancelled quickly")
}

// ========================================================================
// Tests for nil/empty page handling
// ========================================================================

func TestExecute_NilPage_NoOp(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	// Should not panic
	exec.Execute(context.Background(), s, nil, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	assert.Empty(t, pkts, "No packets should be sent for nil page")
}

func TestExecute_EmptyList_SendsDialogEnd(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{List: []*resource.EventCommand{}}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	// Should get at least dialog_end
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_dialog_end" {
			found = true
		}
	}
	assert.True(t, found, "Should send dialog_end even for empty command list")
}
