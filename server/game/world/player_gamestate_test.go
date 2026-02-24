package world

import (
	"testing"

	"go.uber.org/zap"
)

func TestGlobalWhitelist_IsSwitchGlobal(t *testing.T) {
	wl := NewGlobalWhitelist()
	wl.Switches[10] = true
	wl.Switches[20] = true

	if !wl.IsSwitchGlobal(10) {
		t.Error("expected switch 10 to be global")
	}
	if !wl.IsSwitchGlobal(20) {
		t.Error("expected switch 20 to be global")
	}
	if wl.IsSwitchGlobal(11) {
		t.Error("expected switch 11 to NOT be global")
	}
}

func TestGlobalWhitelist_IsVariableGlobal(t *testing.T) {
	wl := NewGlobalWhitelist()
	wl.Variables[202] = true
	wl.Variables[203] = true

	if !wl.IsVariableGlobal(202) {
		t.Error("expected variable 202 to be global")
	}
	if !wl.IsVariableGlobal(203) {
		t.Error("expected variable 203 to be global")
	}
	if wl.IsVariableGlobal(1) {
		t.Error("expected variable 1 to NOT be global")
	}
}

func TestCompositeGameState_RoutesToGlobal(t *testing.T) {
	// Setup global state
	global := NewGameState(nil, zap.NewNop())
	global.SetSwitch(10, true)
	global.SetVariable(202, 42)

	// Setup player state (different values)
	player := NewPlayerGameState(1, nil)
	player.SetSwitch(10, false) // Should not affect global
	player.SetVariable(202, 99) // Should not affect global

	// Setup whitelist with switch 10 and variable 202 as global
	wl := NewGlobalWhitelist()
	wl.Switches[10] = true
	wl.Variables[202] = true

	composite := NewCompositeGameState(global, player, wl)

	// Global IDs should return global values
	if !composite.GetSwitch(10) {
		t.Error("expected switch 10 to read from global (true)")
	}
	if composite.GetVariable(202) != 42 {
		t.Errorf("expected variable 202 to read from global (42), got %d", composite.GetVariable(202))
	}

	// Setting global IDs should update global state
	composite.SetSwitch(10, false)
	if global.GetSwitch(10) != false {
		t.Error("expected global switch 10 to be updated to false")
	}

	composite.SetVariable(202, 100)
	if global.GetVariable(202) != 100 {
		t.Errorf("expected global variable 202 to be updated to 100, got %d", global.GetVariable(202))
	}
}

func TestCompositeGameState_RoutesToPlayer(t *testing.T) {
	// Setup global state
	global := NewGameState(nil, zap.NewNop())
	global.SetSwitch(5, true)
	global.SetVariable(100, 42)

	// Setup player state (different values)
	player := NewPlayerGameState(1, nil)
	player.SetSwitch(5, false)
	player.SetVariable(100, 99)

	// Setup empty whitelist (no global IDs)
	wl := NewGlobalWhitelist()

	composite := NewCompositeGameState(global, player, wl)

	// Non-global IDs should return player values
	if composite.GetSwitch(5) != false {
		t.Errorf("expected switch 5 to read from player (false), got %v", composite.GetSwitch(5))
	}
	if composite.GetVariable(100) != 99 {
		t.Errorf("expected variable 100 to read from player (99), got %d", composite.GetVariable(100))
	}

	// Setting non-global IDs should update player state
	composite.SetSwitch(5, true)
	if player.GetSwitch(5) != true {
		t.Error("expected player switch 5 to be updated to true")
	}
	// Global should be unchanged
	if global.GetSwitch(5) != true {
		t.Error("expected global switch 5 to remain unchanged")
	}

	composite.SetVariable(100, 200)
	if player.GetVariable(100) != 200 {
		t.Errorf("expected player variable 100 to be updated to 200, got %d", player.GetVariable(100))
	}
	// Global should be unchanged
	if global.GetVariable(100) != 42 {
		t.Errorf("expected global variable 100 to remain 42, got %d", global.GetVariable(100))
	}
}

func TestCompositeGameState_SelfSwitchesAlwaysPerPlayer(t *testing.T) {
	// Setup global state (should not be used for self-switches)
	global := NewGameState(nil, zap.NewNop())

	// Setup player state
	player := NewPlayerGameState(1, nil)
	player.SetSelfSwitch(1, 10, "A", true)

	// Setup whitelist (self-switches should ignore whitelist)
	wl := NewGlobalWhitelist()
	wl.Switches[1] = true // This should not affect self-switches

	composite := NewCompositeGameState(global, player, wl)

	// Self-switches should always read from player state
	if !composite.GetSelfSwitch(1, 10, "A") {
		t.Error("expected self-switch (1,10,A) to read from player (true)")
	}
	if composite.GetSelfSwitch(1, 10, "B") {
		t.Error("expected unset self-switch (1,10,B) to return false")
	}

	// Setting self-switches should always update player state
	composite.SetSelfSwitch(1, 10, "B", true)
	if !player.GetSelfSwitch(1, 10, "B") {
		t.Error("expected player self-switch (1,10,B) to be updated to true")
	}
}

func TestPlayerStateManager_GetOrLoad_CachesState(t *testing.T) {
	global := NewGameState(nil, zap.NewNop())
	wl := NewGlobalWhitelist()
	psm := NewPlayerStateManager(global, wl, nil)

	// First call should create new state
	ps1, err := psm.GetOrLoad(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Set a value
	ps1.SetSwitch(5, true)

	// Second call should return cached state
	ps2, err := psm.GetOrLoad(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be the same object
	if ps1 != ps2 {
		t.Error("expected GetOrLoad to return cached state")
	}

	// Value should be preserved
	if !ps2.GetSwitch(5) {
		t.Error("expected cached state to preserve switch value")
	}
}

func TestPlayerStateManager_GetComposite(t *testing.T) {
	global := NewGameState(nil, zap.NewNop())
	wl := NewGlobalWhitelist()
	wl.Variables[202] = true
	psm := NewPlayerStateManager(global, wl, nil)

	composite, err := psm.GetComposite(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify routing works
	global.SetVariable(202, 42)
	if composite.GetVariable(202) != 42 {
		t.Errorf("expected composite to route to global for var 202")
	}

	// Set player-specific value
	composite.SetVariable(1, 100)
	if composite.GetVariable(1) != 100 {
		t.Errorf("expected composite to route to player for var 1")
	}
}

func TestPlayerStateManager_Unload(t *testing.T) {
	global := NewGameState(nil, zap.NewNop())
	wl := NewGlobalWhitelist()
	psm := NewPlayerStateManager(global, wl, nil)

	// Load state
	ps1, _ := psm.GetOrLoad(1)
	ps1.SetSwitch(5, true)

	// Unload
	psm.Unload(1)

	// Get again - should create new state (not cached)
	ps2, _ := psm.GetOrLoad(1)
	if ps1 == ps2 {
		t.Error("expected Unload to clear cache")
	}

	// New state should not have the value
	if ps2.GetSwitch(5) {
		t.Error("expected new state to not preserve old values after unload")
	}
}
