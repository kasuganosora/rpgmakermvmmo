package hook

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// ErrInterrupt signals that a Hook handler wants to stop further processing.
var ErrInterrupt = errors.New("hook interrupted")

// HookFn is a hook handler function.
// Returns (modified data, nil) to continue, or (data, ErrInterrupt) to stop.
type HookFn func(ctx context.Context, event string, data interface{}) (interface{}, error)

type hookEntry struct {
	priority int
	fn       HookFn
	name     string
}

// HookCenter manages event hook registrations.
type HookCenter struct {
	mu    sync.RWMutex
	hooks map[string][]*hookEntry
}

// NewHookCenter creates a new HookCenter.
func NewHookCenter() *HookCenter {
	return &HookCenter{hooks: make(map[string][]*hookEntry)}
}

// Register adds a HookFn for the given event with the given priority (lower runs first).
// name is used for Unregister.
func (hc *HookCenter) Register(event string, priority int, name string, fn HookFn) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	entries := hc.hooks[event]
	entries = append(entries, &hookEntry{priority: priority, fn: fn, name: name})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority < entries[j].priority
	})
	hc.hooks[event] = entries
}

// Unregister removes all hooks with the given name for the given event.
func (hc *HookCenter) Unregister(event, name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	entries := hc.hooks[event]
	n := 0
	for _, e := range entries {
		if e.name != name {
			entries[n] = e
			n++
		}
	}
	hc.hooks[event] = entries[:n]
}

// UnregisterAll removes all hooks registered with the given name across all events.
func (hc *HookCenter) UnregisterAll(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	for event, entries := range hc.hooks {
		n := 0
		for _, e := range entries {
			if e.name != name {
				entries[n] = e
				n++
			}
		}
		hc.hooks[event] = entries[:n]
	}
}

// Trigger executes all registered hooks for event in priority order.
// Data flows through each handler, allowing modification.
// If any handler returns ErrInterrupt, execution stops.
func (hc *HookCenter) Trigger(ctx context.Context, event string, data interface{}) (interface{}, error) {
	hc.mu.RLock()
	entries := make([]*hookEntry, len(hc.hooks[event]))
	copy(entries, hc.hooks[event])
	hc.mu.RUnlock()

	var err error
	for _, e := range entries {
		data, err = e.fn(ctx, event, data)
		if errors.Is(err, ErrInterrupt) {
			return data, err
		}
	}
	return data, nil
}

// ---- Hook event name constants (§8.2) ----

const (
	BeforePlayerMove  = "before_player_move"
	AfterPlayerMove   = "after_player_move"
	BeforeDamageCalc  = "before_damage_calc"
	AfterDamageCalc   = "after_damage_calc"
	BeforeSkillUse    = "before_skill_use"
	AfterSkillUse     = "after_skill_use"
	AfterMonsterDeath = "after_monster_death"
	BeforeItemUse     = "before_item_use"
	OnQuestComplete   = "on_quest_complete"
	OnPlayerLevelUp   = "on_player_level_up"
	OnPlayerLogin     = "on_player_login"
	OnPlayerLogout    = "on_player_logout"
	OnChatSend        = "on_chat_send"
	BeforeTradeCommit = "before_trade_commit"
	OnMapEnter        = "on_map_enter"
)
