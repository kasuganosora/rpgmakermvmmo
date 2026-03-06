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
)

// ========================================================================
// EnterInstance / LeaveInstance plugin command dispatch tests
// ========================================================================

// TestExecute_EnterInstance_CallsCallback verifies that the "EnterInstance"
// plugin command (code 356) invokes the EnterInstanceFn callback.
func TestExecute_EnterInstance_CallsCallback(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	called := false
	var calledSession *player.PlayerSession

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"EnterInstance"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{
		EnterInstanceFn: func(sess *player.PlayerSession) {
			called = true
			calledSession = sess
		},
	}

	exec.Execute(context.Background(), s, page, opts)

	assert.True(t, called, "EnterInstanceFn should have been called")
	assert.Equal(t, s, calledSession, "callback should receive the correct session")
}

// TestExecute_LeaveInstance_CallsCallback verifies that the "LeaveInstance"
// plugin command (code 356) invokes the LeaveInstanceFn callback.
func TestExecute_LeaveInstance_CallsCallback(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	called := false
	var calledSession *player.PlayerSession

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"LeaveInstance"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{
		LeaveInstanceFn: func(sess *player.PlayerSession) {
			called = true
			calledSession = sess
		},
	}

	exec.Execute(context.Background(), s, page, opts)

	assert.True(t, called, "LeaveInstanceFn should have been called")
	assert.Equal(t, s, calledSession, "callback should receive the correct session")
}

// TestExecute_EnterInstance_NoCallback verifies that EnterInstance with nil
// callback does not panic. When no callback is set, the command is silently
// skipped.
func TestExecute_EnterInstance_NoCallback(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"EnterInstance"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	// nil opts — no callback set at all.
	assert.NotPanics(t, func() {
		exec.Execute(context.Background(), s, page, nil)
	}, "EnterInstance with nil opts should not panic")

	// Non-nil opts but nil callback.
	assert.NotPanics(t, func() {
		exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	}, "EnterInstance with nil EnterInstanceFn should not panic")
}

// TestExecute_LeaveInstance_NoCallback verifies that LeaveInstance with nil
// callback does not panic.
func TestExecute_LeaveInstance_NoCallback(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"LeaveInstance"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	assert.NotPanics(t, func() {
		exec.Execute(context.Background(), s, page, nil)
	}, "LeaveInstance with nil opts should not panic")

	assert.NotPanics(t, func() {
		exec.Execute(context.Background(), s, page, &ExecuteOpts{})
	}, "LeaveInstance with nil LeaveInstanceFn should not panic")
}

// TestExecute_EnterInstance_NotForwarded verifies that the EnterInstance plugin
// command is NOT forwarded to the client as an npc_effect packet. It should be
// consumed server-side only.
func TestExecute_EnterInstance_NotForwarded(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"EnterInstance"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{
		EnterInstanceFn: func(_ *player.PlayerSession) {
			// callback fires but we don't care about it here
		},
	}

	exec.Execute(context.Background(), s, page, opts)

	// Drain all packets from SendChan. Only npc_dialog_end should be present
	// (sent by Execute's sendDialogEnd at the end). No npc_effect for EnterInstance.
	pkts := drainPackets(t, s)
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(pkt.Payload, &payload))
			if cmd, ok := payload["cmd"].(map[string]interface{}); ok {
				if params, ok := cmd["parameters"].([]interface{}); ok && len(params) > 0 {
					assert.NotEqual(t, "EnterInstance", params[0],
						"EnterInstance should not be forwarded as npc_effect")
				}
			}
		}
	}
}

// TestExecute_LeaveInstance_NotForwarded verifies that the LeaveInstance plugin
// command is NOT forwarded to the client as an npc_effect packet.
func TestExecute_LeaveInstance_NotForwarded(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"LeaveInstance"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{
		LeaveInstanceFn: func(_ *player.PlayerSession) {},
	}

	exec.Execute(context.Background(), s, page, opts)

	pkts := drainPackets(t, s)
	for _, pkt := range pkts {
		if pkt.Type == "npc_effect" {
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(pkt.Payload, &payload))
			if cmd, ok := payload["cmd"].(map[string]interface{}); ok {
				if params, ok := cmd["parameters"].([]interface{}); ok && len(params) > 0 {
					assert.NotEqual(t, "LeaveInstance", params[0],
						"LeaveInstance should not be forwarded as npc_effect")
				}
			}
		}
	}
}

// TestExecute_EnterLeaveInstance_Sequence verifies that EnterInstance and
// LeaveInstance can appear in the same event command list and both callbacks
// fire in order.
func TestExecute_EnterLeaveInstance_Sequence(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)

	var callOrder []string

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: CmdPluginCommand, Parameters: []interface{}{"EnterInstance"}},
			{Code: CmdWait, Parameters: []interface{}{float64(1)}}, // 1 frame wait
			{Code: CmdPluginCommand, Parameters: []interface{}{"LeaveInstance"}},
			{Code: CmdEnd, Indent: 0},
		},
	}

	opts := &ExecuteOpts{
		EnterInstanceFn: func(_ *player.PlayerSession) {
			callOrder = append(callOrder, "enter")
		},
		LeaveInstanceFn: func(_ *player.PlayerSession) {
			callOrder = append(callOrder, "leave")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	exec.Execute(ctx, s, page, opts)

	assert.Equal(t, []string{"enter", "leave"}, callOrder,
		"callbacks should fire in order: enter then leave")
}
