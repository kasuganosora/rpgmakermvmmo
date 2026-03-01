package npc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---- 通用辅助函数 ----

func nopLogger() *zap.Logger { return zap.NewNop() }

// testSession 创建无真实 WebSocket 连接的 PlayerSession，用于单元测试。
// 使用缓冲 SendChan 便于检查发送的数据包。
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

// testSessionWithStats 创建带有 HP/MP/Level/Exp 初始值的测试会话。
func testSessionWithStats(charID int64, hp, maxHP, mp, maxMP, level int, exp int64) *player.PlayerSession {
	s := testSession(charID)
	s.HP = hp
	s.MaxHP = maxHP
	s.MP = mp
	s.MaxMP = maxMP
	s.Level = level
	s.Exp = exp
	return s
}

// recvPacket 从会话的 SendChan 读取下一个数据包。
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

// drainPackets 读取会话中所有可用的数据包。
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

// packetTypes 提取数据包类型列表，方便断言。
func packetTypes(pkts []*player.Packet) []string {
	types := make([]string, len(pkts))
	for i, p := range pkts {
		types[i] = p.Type
	}
	return types
}

// ---- mockGameState：GameStateAccessor 的测试 mock ----

type mockGameState struct {
	switches      map[int]bool
	variables     map[int]int
	selfSwitches  map[string]bool // key: "mapID_eventID_ch"
	selfVariables map[string]int  // key: "mapID_eventID_index"
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
	return m.selfSwitches[selfSwitchKey(mapID, eventID, ch)]
}
func (m *mockGameState) SetSelfSwitch(mapID, eventID int, ch string, val bool) {
	m.selfSwitches[selfSwitchKey(mapID, eventID, ch)] = val
}
func (m *mockGameState) GetSelfVariable(mapID, eventID, index int) int {
	return m.selfVariables[selfVariableKey(mapID, eventID, index)]
}
func (m *mockGameState) SetSelfVariable(mapID, eventID, index, val int) {
	m.selfVariables[selfVariableKey(mapID, eventID, index)] = val
}

func selfSwitchKey(mapID, eventID int, ch string) string {
	return fmt.Sprintf("%d_%d_%s", mapID, eventID, ch)
}
func selfVariableKey(mapID, eventID, index int) string {
	return fmt.Sprintf("%d_%d_%d", mapID, eventID, index)
}

// ---- mockInventoryStore：InventoryStore 的测试 mock ----

type mockInventoryStore struct {
	gold  map[int64]int64      // charID -> gold
	items map[string]int       // "charID_itemID" -> qty
}

func newMockInventoryStore() *mockInventoryStore {
	return &mockInventoryStore{
		gold:  make(map[int64]int64),
		items: make(map[string]int),
	}
}

func itemKey(charID int64, itemID int) string {
	return fmt.Sprintf("%d_%d", charID, itemID)
}

func (m *mockInventoryStore) GetGold(_ context.Context, charID int64) (int64, error) {
	return m.gold[charID], nil
}

func (m *mockInventoryStore) UpdateGold(_ context.Context, charID int64, amount int64) error {
	m.gold[charID] += amount
	return nil
}

func (m *mockInventoryStore) GetItem(_ context.Context, charID int64, itemID int) (int, error) {
	k := itemKey(charID, itemID)
	qty, ok := m.items[k]
	if !ok {
		return 0, fmt.Errorf("item %d not found", itemID)
	}
	return qty, nil
}

func (m *mockInventoryStore) AddItem(_ context.Context, charID int64, itemID, qty int) error {
	k := itemKey(charID, itemID)
	m.items[k] += qty
	return nil
}

func (m *mockInventoryStore) RemoveItem(_ context.Context, charID int64, itemID, qty int) error {
	k := itemKey(charID, itemID)
	cur := m.items[k]
	if cur < qty {
		return fmt.Errorf("insufficient: have %d, need %d", cur, qty)
	}
	newQty := cur - qty
	if newQty <= 0 {
		delete(m.items, k)
	} else {
		m.items[k] = newQty
	}
	return nil
}

// ========================================================================
// ShowText 对话显示测试
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

	s.DialogAckCh <- struct{}{}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	require.True(t, len(pkts) >= 2, "Expected at least dialog + dialog_end packets, got %d", len(pkts))

	assert.Equal(t, "npc_dialog", pkts[0].Type)

	var dialogData map[string]interface{}
	require.NoError(t, json.Unmarshal(pkts[0].Payload, &dialogData))
	assert.Equal(t, "Actor1", dialogData["face"])

	lines, ok := dialogData["lines"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, "Hello!", lines[0])
	assert.Equal(t, "How are you?", lines[1])

	assert.Equal(t, "npc_dialog_end", pkts[len(pkts)-1].Type)
}

// ========================================================================
// ShowChoices 选项显示测试
// ========================================================================

func TestExecute_ShowChoices_SendsChoicesAndWaits(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowChoices, Indent: 0, Parameters: []interface{}{
				[]interface{}{"Yes", "No"}, float64(-1),
			}},
			{Code: CmdWhenBranch, Indent: 0, Parameters: []interface{}{float64(0)}},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"You said Yes!"}},
			{Code: CmdWhenBranch, Indent: 0, Parameters: []interface{}{float64(1)}},
			{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"You said No!"}},
			{Code: CmdBranchEnd, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.ChoiceCh <- 0
		time.Sleep(50 * time.Millisecond)
		s.DialogAckCh <- struct{}{}
	}()

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	types := packetTypes(pkts)

	assert.Contains(t, types, "npc_choices", "Should send npc_choices packet")
	assert.Contains(t, types, "npc_dialog", "Should send dialog for the chosen branch")

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
// ConditionalBranch 条件分支测试
// ========================================================================

func TestExecute_ConditionalBranch_SwitchTrue(t *testing.T) {
	gs := newMockGameState()
	gs.switches[10] = true

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
			assert.Equal(t, "Switch is ON!", lines[0])
			foundDialog = true
		}
	}
	assert.True(t, foundDialog, "Should execute IF branch when switch is ON")
}

func TestExecute_ConditionalBranch_SwitchFalse(t *testing.T) {
	gs := newMockGameState()

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

// TestExecute_ConditionalBranch_Variable 测试变量条件分支（condType=1）。
// RMMV 参数格式：[1, varID, 0, compareType, value]
// compareType: 0=等于, 1=大于等于, 2=小于等于, 3=大于, 4=小于, 5=不等于
func TestExecute_ConditionalBranch_Variable(t *testing.T) {
	tests := []struct {
		name        string
		varVal      int
		compareType int
		compareVal  int
		expectTrue  bool
	}{
		{"等于_匹配", 5, 0, 5, true},
		{"等于_不匹配", 5, 0, 3, false},
		{"大于等于_等于", 5, 1, 5, true},
		{"大于等于_大于", 5, 1, 3, true},
		{"大于等于_小于", 3, 1, 5, false},
		{"小于等于_等于", 5, 2, 5, true},
		{"小于等于_小于", 3, 2, 5, true},
		{"小于等于_大于", 5, 2, 3, false},
		{"大于_匹配", 5, 3, 3, true},
		{"大于_不匹配", 3, 3, 5, false},
		{"小于_匹配", 3, 4, 5, true},
		{"小于_不匹配", 5, 4, 3, false},
		{"不等于_匹配", 5, 5, 3, true},
		{"不等于_不匹配", 5, 5, 5, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := newMockGameState()
			gs.variables[10] = tc.varVal

			exec := New(nil, &resource.ResourceLoader{}, nopLogger())
			s := testSession(1)

			page := &resource.EventPage{
				List: []*resource.EventCommand{
					// condType=1(variable), varID=10, refType=0(constant), refVal, op
					{Code: CmdConditionalStart, Indent: 0, Parameters: []interface{}{
						float64(1), float64(10), float64(0), float64(tc.compareVal), float64(tc.compareType),
					}},
					{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
					{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"TRUE"}},
					{Code: CmdElseBranch, Indent: 0},
					{Code: CmdShowText, Indent: 1, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
					{Code: CmdShowTextLine, Indent: 1, Parameters: []interface{}{"FALSE"}},
					{Code: CmdConditionalEnd, Indent: 0},
					{Code: CmdEnd, Indent: 0},
				},
			}

			s.DialogAckCh <- struct{}{}
			exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

			pkts := drainPackets(t, s)
			expected := "FALSE"
			if tc.expectTrue {
				expected = "TRUE"
			}
			for _, pkt := range pkts {
				if pkt.Type == "npc_dialog" {
					var data map[string]interface{}
					json.Unmarshal(pkt.Payload, &data)
					lines := data["lines"].([]interface{})
					assert.Equal(t, expected, lines[0])
				}
			}
		})
	}
}

// ========================================================================
// ChangeSwitches / ChangeVars / ChangeSelfSwitch 状态变更测试
// ========================================================================

func TestExecute_ChangeSwitches(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Indent: 0, Parameters: []interface{}{
				float64(306), float64(306), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	assert.False(t, gs.switches[306])
	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})
	assert.True(t, gs.switches[306], "Switch 306 should be ON after execution")
}

// TestExecute_ChangeSwitches_OFF 测试将开关设为 OFF（value=1）。
func TestExecute_ChangeSwitches_OFF(t *testing.T) {
	gs := newMockGameState()
	gs.switches[100] = true

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// value=1 表示 OFF
			{Code: CmdChangeSwitches, Indent: 0, Parameters: []interface{}{
				float64(100), float64(100), float64(1),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})
	assert.False(t, gs.switches[100], "Switch 100 should be OFF")
}

// TestExecute_ChangeSwitches_Range 测试批量设置连续范围的开关。
func TestExecute_ChangeSwitches_Range(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeSwitches, Indent: 0, Parameters: []interface{}{
				float64(10), float64(13), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	for id := 10; id <= 13; id++ {
		assert.True(t, gs.switches[id], "Switch %d should be ON", id)
	}
}

func TestExecute_ChangeVariables_Add(t *testing.T) {
	gs := newMockGameState()
	gs.variables[206] = 5

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(206), float64(206), float64(1), float64(0), float64(3),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})
	assert.Equal(t, 8, gs.variables[206], "Variable 206 should be 5 + 3 = 8")
}

// TestExecute_ChangeVariables_AllOps 测试变量变更所有操作类型。
func TestExecute_ChangeVariables_AllOps(t *testing.T) {
	tests := []struct {
		name     string
		op       int
		initial  int
		operand  int
		expected int
	}{
		{"设置", 0, 5, 10, 10},
		{"加", 1, 5, 3, 8},
		{"减", 2, 10, 3, 7},
		{"乘", 3, 5, 4, 20},
		{"除", 4, 20, 5, 4},
		{"取模", 5, 17, 5, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := newMockGameState()
			gs.variables[1] = tc.initial

			exec := New(nil, &resource.ResourceLoader{}, nopLogger())
			s := testSession(1)

			page := &resource.EventPage{
				List: []*resource.EventCommand{
					{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
						float64(1), float64(1), float64(tc.op), float64(0), float64(tc.operand),
					}},
					{Code: CmdEnd, Indent: 0},
				},
			}

			exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})
			assert.Equal(t, tc.expected, gs.variables[1])
		})
	}
}

// TestExecute_ChangeVariables_DivByZero 测试除零保护。
func TestExecute_ChangeVariables_DivByZero(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 10

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// 除以 0 应该不改变值
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(4), float64(0), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})
	assert.Equal(t, 10, gs.variables[1], "Division by zero should not change value")
}

// TestExecute_ChangeVariables_VarReference 测试变量引用操作数（operandType=1）。
func TestExecute_ChangeVariables_VarReference(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 10
	gs.variables[2] = 3

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// op=1(add), operandType=1(variable), operand=2(var[2])
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(1), float64(1), float64(2),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})
	assert.Equal(t, 13, gs.variables[1], "var[1]=10 + var[2]=3 = 13")
}

func TestExecute_ChangeSelfSwitch(t *testing.T) {
	gs := newMockGameState()
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
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
// Transfer Player 地图传送测试
// ========================================================================

func TestExecute_Transfer_CallsTransferFn(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	var transferCalled bool
	var tMapID, tX, tY, tDir int

	page := &resource.EventPage{
		List: []*resource.EventCommand{
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

	assert.True(t, transferCalled)
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
// NPC 效果转发测试
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
// Wait 等待指令测试
// ========================================================================

func TestExecute_Wait_Pauses(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
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
// 上下文取消测试
// ========================================================================

func TestExecute_ContextCancel_StopsExecution(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdWait, Indent: 0, Parameters: []interface{}{float64(600)}},
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
// nil/空页面处理测试
// ========================================================================

func TestExecute_NilPage_NoOp(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

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
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_dialog_end" {
			found = true
		}
	}
	assert.True(t, found, "Should send dialog_end even for empty command list")
}

// ========================================================================
// Loop / BreakLoop 循环控制测试
// ========================================================================

// TestExecute_Loop_BreakLoop 测试循环执行和 break 退出。
// 使用变量累加 + 条件判断 break 验证循环正确执行指定次数。
func TestExecute_Loop_BreakLoop(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 0 // 循环计数器

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// Loop
			{Code: CmdLoop, Indent: 0},
			// var[1] += 1
			{Code: CmdChangeVars, Indent: 1, Parameters: []interface{}{
				float64(1), float64(1), float64(1), float64(0), float64(1),
			}},
			// if var[1] >= 5, break
			{Code: CmdConditionalStart, Indent: 1, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(5), float64(1), // condType=1(var), varID=1, refType=0(const), refVal=5, op=1(>=)
			}},
			{Code: CmdBreakLoop, Indent: 2},
			{Code: CmdConditionalEnd, Indent: 1},
			// RepeatAbove (end of loop body)
			{Code: CmdRepeatAbove, Indent: 0},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	assert.Equal(t, 5, gs.variables[1], "Loop should execute 5 times before breaking")
}

// ========================================================================
// Label / JumpToLabel 标签跳转测试
// ========================================================================

// TestExecute_Label_JumpToLabel 测试标签跳转。
func TestExecute_Label_JumpToLabel(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 0

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// 跳过中间的 var[1]=99
			{Code: CmdJumpToLabel, Indent: 0, Parameters: []interface{}{"skip"}},
			// 这里不应被执行
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(0), float64(99),
			}},
			// Label "skip"
			{Code: CmdLabel, Indent: 0, Parameters: []interface{}{"skip"}},
			// var[1] = 42
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(0), float64(42),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	assert.Equal(t, 42, gs.variables[1], "Should jump to label, skipping var[1]=99")
}

// ========================================================================
// ExitEvent 中途退出测试
// ========================================================================

// TestExecute_ExitEvent 测试中途退出事件处理。
func TestExecute_ExitEvent(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 0

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// var[1] = 10
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(0), float64(10),
			}},
			// Exit Event
			{Code: CmdExitEvent, Indent: 0},
			// 这里不应被执行
			{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
				float64(1), float64(1), float64(0), float64(0), float64(99),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	assert.Equal(t, 10, gs.variables[1], "ExitEvent should stop execution, var[1] stays at 10")
}

// ========================================================================
// CallCommonEvent 公共事件调用测试
// ========================================================================

// TestExecute_CallCommonEvent 测试公共事件调用。
func TestExecute_CallCommonEvent(t *testing.T) {
	gs := newMockGameState()
	gs.variables[1] = 0

	rl := &resource.ResourceLoader{}
	rl.CommonEvents = []*resource.CommonEvent{
		nil, // index 0 is null (RMMV convention)
		{
			ID:   1,
			Name: "TestCE",
			List: []*resource.EventCommand{
				{Code: CmdChangeVars, Indent: 0, Parameters: []interface{}{
					float64(1), float64(1), float64(0), float64(0), float64(77),
				}},
				{Code: CmdEnd, Indent: 0},
			},
		},
	}

	exec := New(nil, rl, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// CallCommonEvent: [commonEventID=1]
			{Code: CmdCallCommonEvent, Indent: 0, Parameters: []interface{}{float64(1)}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	assert.Equal(t, 77, gs.variables[1], "Common event should set var[1] to 77")
}

// ========================================================================
// ChangeGold 金币变更测试
// ========================================================================

// TestExecute_ChangeGold_Add 测试增加金币。
func TestExecute_ChangeGold_Add(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 100

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeGold: [op=0(增加), operandType=0(常量), amount=50]
			{Code: CmdChangeGold, Indent: 0, Parameters: []interface{}{
				float64(0), float64(0), float64(50),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, int64(150), store.gold[1], "Gold should increase from 100 to 150")
}

// TestExecute_ChangeGold_Subtract 测试减少金币。
func TestExecute_ChangeGold_Subtract(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 100

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeGold: [op=1(减少), operandType=0(常量), amount=30]
			{Code: CmdChangeGold, Indent: 0, Parameters: []interface{}{
				float64(1), float64(0), float64(30),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, int64(70), store.gold[1], "Gold should decrease from 100 to 70")
}

// TestExecute_ChangeGold_InsufficientGold 测试余额不足时不扣除。
func TestExecute_ChangeGold_InsufficientGold(t *testing.T) {
	store := newMockInventoryStore()
	store.gold[1] = 10

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeGold, Indent: 0, Parameters: []interface{}{
				float64(1), float64(0), float64(50),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, int64(10), store.gold[1], "Gold should remain unchanged when insufficient")
}

// ========================================================================
// ChangeItems 物品变更测试
// ========================================================================

// TestExecute_ChangeItems_Add 测试增加物品。
func TestExecute_ChangeItems_Add(t *testing.T) {
	store := newMockInventoryStore()

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeItems: [itemID=5, op=0(增加), operandType=0(常量), qty=3]
			{Code: CmdChangeItems, Indent: 0, Parameters: []interface{}{
				float64(5), float64(0), float64(0), float64(3),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	qty, _ := store.GetItem(context.Background(), 1, 5)
	assert.Equal(t, 3, qty, "Should have 3 of item 5")
}

// TestExecute_ChangeItems_Remove 测试减少物品。
func TestExecute_ChangeItems_Remove(t *testing.T) {
	store := newMockInventoryStore()
	store.items[itemKey(1, 5)] = 10

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeItems: [itemID=5, op=1(减少), operandType=0(常量), qty=3]
			{Code: CmdChangeItems, Indent: 0, Parameters: []interface{}{
				float64(5), float64(1), float64(0), float64(3),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	qty, _ := store.GetItem(context.Background(), 1, 5)
	assert.Equal(t, 7, qty, "Should have 10 - 3 = 7 of item 5")
}

// TestExecute_ChangeItems_InsufficientQty 测试物品数量不足时不扣除。
func TestExecute_ChangeItems_InsufficientQty(t *testing.T) {
	store := newMockInventoryStore()
	store.items[itemKey(1, 5)] = 2

	exec := New(store, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeItems, Indent: 0, Parameters: []interface{}{
				float64(5), float64(1), float64(0), float64(10),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	qty, _ := store.GetItem(context.Background(), 1, 5)
	assert.Equal(t, 2, qty, "Should remain at 2 when insufficient")
}

// ========================================================================
// ChangeHP 变更 HP 测试
// ========================================================================

// TestExecute_ChangeHP_Increase 测试增加 HP。
func TestExecute_ChangeHP_Increase(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 50, 100, 30, 50, 1, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeHP: [fixedActor=0, actorID=1, op=0(增加), operandType=0(常量), amount=30, allowDeath=0]
			{Code: CmdChangeHP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(0), float64(30), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 80, s.HP, "HP should increase from 50 to 80")
}

// TestExecute_ChangeHP_ClampMax 测试 HP 不超过上限。
func TestExecute_ChangeHP_ClampMax(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 90, 100, 30, 50, 1, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeHP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(0), float64(50), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 100, s.HP, "HP should be clamped to MaxHP=100")
}

// TestExecute_ChangeHP_Decrease_AllowDeath 测试减少 HP 允许死亡。
func TestExecute_ChangeHP_Decrease_AllowDeath(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 30, 100, 30, 50, 1, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// op=1(减少), amount=50, allowDeath=1
			{Code: CmdChangeHP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(1), float64(0), float64(50), float64(1),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 0, s.HP, "HP should be 0 when allowDeath=true and damage exceeds HP")
}

// TestExecute_ChangeHP_Decrease_NoDeath 测试减少 HP 不允许死亡（最低为 1）。
func TestExecute_ChangeHP_Decrease_NoDeath(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 30, 100, 30, 50, 1, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// op=1(减少), amount=50, allowDeath=0
			{Code: CmdChangeHP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(1), float64(0), float64(50), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 1, s.HP, "HP should be clamped to 1 when allowDeath=false")
}

// ========================================================================
// ChangeMP 变更 MP 测试
// ========================================================================

// TestExecute_ChangeMP 测试增加和限制 MP。
func TestExecute_ChangeMP(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 100, 100, 20, 50, 1, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// MP +40
			{Code: CmdChangeMP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(0), float64(40),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 50, s.MP, "MP should be clamped to MaxMP=50 (20+40=60 > 50)")
}

// TestExecute_ChangeMP_Decrease 测试减少 MP（不低于 0）。
func TestExecute_ChangeMP_Decrease(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 100, 100, 20, 50, 1, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeMP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(1), float64(0), float64(30),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 0, s.MP, "MP should be clamped to 0 (20-30=-10 < 0)")
}

// ========================================================================
// RecoverAll 完全恢复测试
// ========================================================================

// TestExecute_RecoverAll 测试完全恢复 HP 和 MP。
func TestExecute_RecoverAll(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 30, 100, 10, 50, 1, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdRecoverAll, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 100, s.HP, "HP should be restored to MaxHP")
	assert.Equal(t, 50, s.MP, "MP should be restored to MaxMP")
}

// ========================================================================
// ChangeEXP 经验值变更测试
// ========================================================================

// TestExecute_ChangeEXP_Add 测试增加经验值。
func TestExecute_ChangeEXP_Add(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 100, 100, 50, 50, 1, 100)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeEXP: [fixedActor=0, actorID=1, op=0(增加), operandType=0(常量), amount=50, showLevelUp=0]
			{Code: CmdChangeEXP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(0), float64(50), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, int64(150), s.Exp, "EXP should increase from 100 to 150")
}

// TestExecute_ChangeEXP_Subtract_ClampZero 测试经验值不低于 0。
func TestExecute_ChangeEXP_Subtract_ClampZero(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 100, 100, 50, 50, 1, 30)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeEXP, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(1), float64(0), float64(50), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, int64(0), s.Exp, "EXP should be clamped to 0")
}

// ========================================================================
// ChangeLevel 等级变更测试
// ========================================================================

// TestExecute_ChangeLevel_Add 测试增加等级。
func TestExecute_ChangeLevel_Add(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 100, 100, 50, 50, 5, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeLevel, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(0), float64(0), float64(3), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 8, s.Level, "Level should increase from 5 to 8")
}

// TestExecute_ChangeLevel_Subtract_ClampOne 测试等级不低于 1。
func TestExecute_ChangeLevel_Subtract_ClampOne(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 100, 100, 50, 50, 3, 0)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeLevel, Indent: 0, Parameters: []interface{}{
				float64(0), float64(1), float64(1), float64(0), float64(10), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 1, s.Level, "Level should be clamped to 1")
}

// ========================================================================
// ChangeClass 职业变更测试
// ========================================================================

// TestExecute_ChangeClass_ScalesHPMP 测试职业变更时 HP/MP 等比缩放。
func TestExecute_ChangeClass_ScalesHPMP(t *testing.T) {
	rl := &resource.ResourceLoader{}
	rl.Classes = []*resource.Class{
		nil,
		{
			ID:   1,
			Name: "Warrior",
			// Params[0]=HP曲线, Params[1]=MP曲线，索引为等级（0起始预留）
			Params: [][]int{
				{0, 100, 120, 150, 200, 250}, // HP by level
				{0, 20, 25, 30, 40, 50},       // MP by level
			},
		},
		{
			ID:   2,
			Name: "Mage",
			Params: [][]int{
				{0, 80, 100, 120, 160, 200}, // HP by level
				{0, 50, 60, 70, 80, 100},    // MP by level
			},
		},
	}

	exec := New(nil, rl, nopLogger())
	s := testSessionWithStats(1, 200, 250, 40, 50, 5, 0)
	s.ClassID = 1

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ChangeClass: [actorID=1, classID=2, keepExp=0]
			{Code: CmdChangeClass, Indent: 0, Parameters: []interface{}{
				float64(1), float64(2), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 2, s.ClassID, "ClassID should change to 2")
	// Warrior lv5 HP=250, Mage lv5 HP=200
	// ratio = 200/250 = 0.8, new HP = 0.8 * 200 = 160
	assert.Equal(t, 200, s.MaxHP, "MaxHP should be set to Mage lv5=200")
	assert.Equal(t, 160, s.HP, "HP should be scaled: 200/250 * 200 = 160")
	// Warrior lv5 MP=50, Mage lv5 MP=100
	// ratio = 40/50 = 0.8, new MP = 0.8 * 100 = 80
	assert.Equal(t, 100, s.MaxMP, "MaxMP should be set to Mage lv5=100")
	assert.Equal(t, 80, s.MP, "MP should be scaled: 40/50 * 100 = 80")
}

// TestExecute_ChangeClass_InvalidClassID 测试无效职业 ID 不变更。
func TestExecute_ChangeClass_InvalidClassID(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSessionWithStats(1, 100, 100, 50, 50, 1, 0)
	s.ClassID = 1

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdChangeClass, Indent: 0, Parameters: []interface{}{
				float64(1), float64(0), float64(0), // classID=0 (invalid)
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	assert.Equal(t, 1, s.ClassID, "ClassID should remain 1 when invalid ID is given")
}

// ========================================================================
// resolveOperand 操作数解析测试
// ========================================================================

// TestResolveOperand_Constant 测试常量操作数。
func TestResolveOperand_Constant(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	params := []interface{}{float64(0), float64(0), float64(0), float64(0), float64(42)}
	val := exec.resolveOperand(params, 3, 4, nil)
	assert.Equal(t, 42, val)
}

// TestResolveOperand_Variable 测试变量引用操作数。
func TestResolveOperand_Variable(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 77

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	// operandType=1(变量), operand=10(变量ID)
	params := []interface{}{float64(0), float64(0), float64(0), float64(1), float64(10)}
	val := exec.resolveOperand(params, 3, 4, &ExecuteOpts{GameState: gs})
	assert.Equal(t, 77, val, "Should resolve variable reference to var[10]=77")
}

// ========================================================================
// Text escape codes 文本转义码测试
// ========================================================================

// TestResolveTextCodes_VariableSubstitution 测试 \V[n] 变量替换。
func TestResolveTextCodes_VariableSubstitution(t *testing.T) {
	gs := newMockGameState()
	gs.variables[5] = 42

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdShowText, Parameters: []interface{}{"", float64(0), float64(0), float64(2)}},
			{Code: CmdShowTextLine, Parameters: []interface{}{`You have \V[5] gold.`}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	s.DialogAckCh <- struct{}{}
	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	pkts := drainPackets(t, s)
	for _, pkt := range pkts {
		if pkt.Type == "npc_dialog" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			lines := data["lines"].([]interface{})
			assert.Equal(t, "You have 42 gold.", lines[0])
		}
	}
}

// ========================================================================
// paramInt / paramStr 工具函数测试
// ========================================================================

func TestParamInt_ValidFloat(t *testing.T) {
	params := []interface{}{float64(42), "hello", float64(3.7)}
	assert.Equal(t, 42, paramInt(params, 0))
	assert.Equal(t, 3, paramInt(params, 2)) // truncated
}

func TestParamInt_OutOfBounds(t *testing.T) {
	params := []interface{}{float64(1)}
	assert.Equal(t, 0, paramInt(params, 5), "Out of bounds should return 0")
}

func TestParamInt_NonNumeric(t *testing.T) {
	params := []interface{}{"hello"}
	assert.Equal(t, 0, paramInt(params, 0), "Non-numeric should return 0")
}

func TestParamStr_Valid(t *testing.T) {
	params := []interface{}{"hello", float64(42)}
	assert.Equal(t, "hello", paramStr(params, 0))
}

func TestParamStr_OutOfBounds(t *testing.T) {
	params := []interface{}{"hello"}
	assert.Equal(t, "", paramStr(params, 5), "Out of bounds should return empty string")
}

// ========================================================================
// classParam 职业参数读取测试
// ========================================================================

func TestClassParam_Normal(t *testing.T) {
	cls := &resource.Class{
		Params: [][]int{
			{0, 100, 200, 300},
			{0, 50, 100, 150},
		},
	}
	assert.Equal(t, 200, classParam(cls, 0, 2), "HP at level 2")
	assert.Equal(t, 100, classParam(cls, 1, 2), "MP at level 2")
}

func TestClassParam_LevelExceedsArray(t *testing.T) {
	cls := &resource.Class{
		Params: [][]int{
			{0, 100, 200},
		},
	}
	assert.Equal(t, 200, classParam(cls, 0, 99), "Should return last element when level exceeds array")
}

func TestClassParam_NilClass(t *testing.T) {
	assert.Equal(t, 0, classParam(nil, 0, 1), "Nil class should return 0")
}

func TestClassParam_InvalidParamIdx(t *testing.T) {
	cls := &resource.Class{
		Params: [][]int{{0, 100}},
	}
	assert.Equal(t, 0, classParam(cls, 5, 1), "Invalid param index should return 0")
}

// ========================================================================
// processBattle 战斗处理测试
// ========================================================================

// TestExecute_Battle_CallsBattleFn 测试战斗处理调用回调。
func TestExecute_Battle_CallsBattleFn(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	var calledTroopID int
	var calledEscape, calledLose bool

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// BattleProcessing: [troopType=0(直接指定), troopID=5, canEscape=1, canLose=0]
			{Code: CmdBattleProcessing, Indent: 0, Parameters: []interface{}{
				float64(0), float64(5), float64(1), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{
		BattleFn: func(ctx context.Context, s *player.PlayerSession, troopID int, canEscape, canLose bool) int {
			calledTroopID = troopID
			calledEscape = canEscape
			calledLose = canLose
			return 0
		},
	}

	exec.Execute(context.Background(), s, page, opts)

	assert.Equal(t, 5, calledTroopID)
	assert.True(t, calledEscape, "canEscape should be true")
	assert.False(t, calledLose, "canLose should be false")
}

// TestExecute_Battle_NoBattleFn_Fallback 测试无战斗回调时转发给客户端。
func TestExecute_Battle_NoBattleFn_Fallback(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdBattleProcessing, Indent: 0, Parameters: []interface{}{
				float64(0), float64(3), float64(0), float64(1),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			if data["code"] == float64(CmdBattleProcessing) {
				found = true
			}
		}
	}
	assert.True(t, found, "Should forward battle processing as npc_effect when no BattleFn")
}

// ========================================================================
// Script 脚本过滤测试
// ========================================================================

// TestExecute_Script_FilteredLines 测试 Script 指令仅转发白名单行。
func TestExecute_Script_GameScreenLine(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdScript, Indent: 0, Parameters: []interface{}{"$gameScreen.startTint([0,0,0,0], 60)"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			if data["code"] == float64(CmdScript) {
				found = true
			}
		}
	}
	assert.True(t, found, "$gameScreen lines should be forwarded")
}

// ========================================================================
// Comment 注释指令测试
// ========================================================================

// TestExecute_Comment_Ignored 测试注释指令不产生输出。
func TestExecute_Comment_Ignored(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdComment, Indent: 0, Parameters: []interface{}{"This is a comment"}},
			{Code: CmdCommentCont, Indent: 0, Parameters: []interface{}{"More comment text"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{})

	pkts := drainPackets(t, s)
	// Only dialog_end should be present
	for _, pkt := range pkts {
		assert.NotEqual(t, "npc_effect", pkt.Type, "Comments should not produce npc_effect packets")
	}
}

// ========================================================================
// ShowPicture 变量坐标解析测试
// ========================================================================

// TestExecute_ShowPicture_VariableCoords 测试显示图片时变量坐标解析。
func TestExecute_ShowPicture_VariableCoords(t *testing.T) {
	gs := newMockGameState()
	gs.variables[10] = 200
	gs.variables[11] = 300

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			// ShowPicture: [pictureId=1, name="pic", origin=0, designationType=1(变量),
			//               varIdX=10, varIdY=11, scaleX=100, scaleY=100, opacity=255, blendMode=0]
			{Code: CmdShowPicture, Indent: 0, Parameters: []interface{}{
				float64(1), "pic", float64(0), float64(1),
				float64(10), float64(11),
				float64(100), float64(100), float64(255), float64(0),
			}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	exec.Execute(context.Background(), s, page, &ExecuteOpts{GameState: gs})

	pkts := drainPackets(t, s)
	found := false
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var data map[string]interface{}
			json.Unmarshal(pkt.Payload, &data)
			if data["code"] == float64(CmdShowPicture) {
				found = true
				params := data["params"].([]interface{})
				// 变量坐标应被解析为实际值
				assert.Equal(t, float64(200), params[4], "X should be resolved from var[10]=200")
				assert.Equal(t, float64(300), params[5], "Y should be resolved from var[11]=300")
			}
		}
	}
	assert.True(t, found, "ShowPicture should be forwarded as npc_effect")
}
