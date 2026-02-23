package skill

import (
	"sync"
	"time"
)

// BuffInstance represents an active buff or debuff on a combatant.
type BuffInstance struct {
	BuffID    int
	Stacks    int
	ExpireAt  time.Time
	NextTick  time.Time
	TickMS    int64   // DOT/HOT interval in ms (0 = no tick effect)
	DotDmg    int     // damage per tick (positive = damage, negative = heal)
	mu        sync.Mutex
}

// IsExpired reports whether this buff has expired.
func (b *BuffInstance) IsExpired(now time.Time) bool {
	return !b.ExpireAt.IsZero() && now.After(b.ExpireAt)
}

// NeedsTickAt reports whether a DOT/HOT tick is due.
func (b *BuffInstance) NeedsTickAt(now time.Time) bool {
	return b.TickMS > 0 && !b.NextTick.IsZero() && now.After(b.NextTick)
}

// AdvanceTick advances the NextTick timer.
func (b *BuffInstance) AdvanceTick() {
	b.NextTick = b.NextTick.Add(time.Duration(b.TickMS) * time.Millisecond)
}

// BuffList manages the buff list for a single entity (player or monster).
type BuffList struct {
	mu    sync.RWMutex
	buffs []*BuffInstance
}

// Add adds or refreshes a buff.
func (bl *BuffList) Add(buffID int, duration time.Duration, tickMS int64, dotDmg, maxStacks int) *BuffInstance {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	for _, b := range bl.buffs {
		if b.BuffID == buffID {
			// Refresh.
			b.mu.Lock()
			b.ExpireAt = time.Now().Add(duration)
			if b.Stacks < maxStacks {
				b.Stacks++
			}
			b.mu.Unlock()
			return b
		}
	}
	b := &BuffInstance{
		BuffID:   buffID,
		Stacks:   1,
		ExpireAt: time.Now().Add(duration),
		TickMS:   tickMS,
		DotDmg:   dotDmg,
	}
	if tickMS > 0 {
		b.NextTick = time.Now().Add(time.Duration(tickMS) * time.Millisecond)
	}
	bl.buffs = append(bl.buffs, b)
	return b
}

// Remove removes a buff by ID. Returns true if it was present.
func (bl *BuffList) Remove(buffID int) bool {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	for i, b := range bl.buffs {
		if b.BuffID == buffID {
			bl.buffs = append(bl.buffs[:i], bl.buffs[i+1:]...)
			return true
		}
	}
	return false
}

// Get returns the buff with buffID, or nil.
func (bl *BuffList) Get(buffID int) *BuffInstance {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	for _, b := range bl.buffs {
		if b.BuffID == buffID {
			return b
		}
	}
	return nil
}

// All returns a snapshot of all active buffs.
func (bl *BuffList) All() []*BuffInstance {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	out := make([]*BuffInstance, len(bl.buffs))
	copy(out, bl.buffs)
	return out
}

// TickResult describes the outcome of one buff tick.
type TickResult struct {
	BuffID  int
	DotDmg  int  // positive = damage, negative = heal
	Expired bool
}

// Tick processes all buffs: applies DOT/HOT, removes expired buffs.
// Returns the list of tick results for broadcasting.
func (bl *BuffList) Tick(now time.Time) []TickResult {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	var results []TickResult
	newBuffs := bl.buffs[:0]
	for _, b := range bl.buffs {
		if b.IsExpired(now) {
			results = append(results, TickResult{BuffID: b.BuffID, Expired: true})
			continue
		}
		if b.NeedsTickAt(now) {
			results = append(results, TickResult{BuffID: b.BuffID, DotDmg: b.DotDmg})
			b.AdvanceTick()
		}
		newBuffs = append(newBuffs, b)
	}
	bl.buffs = newBuffs
	return results
}
