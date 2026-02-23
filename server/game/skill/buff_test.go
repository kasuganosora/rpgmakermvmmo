package skill

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- BuffInstance ----

func TestBuffInstance_IsExpired_Future(t *testing.T) {
	b := &BuffInstance{ExpireAt: time.Now().Add(time.Hour)}
	assert.False(t, b.IsExpired(time.Now()))
}

func TestBuffInstance_IsExpired_Past(t *testing.T) {
	b := &BuffInstance{ExpireAt: time.Now().Add(-time.Second)}
	assert.True(t, b.IsExpired(time.Now()))
}

func TestBuffInstance_IsExpired_Zero(t *testing.T) {
	b := &BuffInstance{} // zero ExpireAt = never expires
	assert.False(t, b.IsExpired(time.Now()))
}

func TestBuffInstance_NeedsTickAt_Due(t *testing.T) {
	b := &BuffInstance{TickMS: 100, NextTick: time.Now().Add(-time.Millisecond)}
	assert.True(t, b.NeedsTickAt(time.Now()))
}

func TestBuffInstance_NeedsTickAt_NotDue(t *testing.T) {
	b := &BuffInstance{TickMS: 100, NextTick: time.Now().Add(time.Minute)}
	assert.False(t, b.NeedsTickAt(time.Now()))
}

func TestBuffInstance_NeedsTickAt_NoTick(t *testing.T) {
	b := &BuffInstance{TickMS: 0, NextTick: time.Now().Add(-time.Millisecond)}
	assert.False(t, b.NeedsTickAt(time.Now()))
}

func TestBuffInstance_AdvanceTick(t *testing.T) {
	base := time.Now()
	b := &BuffInstance{TickMS: 500, NextTick: base}
	b.AdvanceTick()
	expected := base.Add(500 * time.Millisecond)
	assert.Equal(t, expected, b.NextTick)
}

// ---- BuffList ----

func TestBuffList_Add_New(t *testing.T) {
	var bl BuffList
	b := bl.Add(1, time.Minute, 0, 0, 5)
	require.NotNil(t, b)
	assert.Equal(t, 1, b.BuffID)
	assert.Equal(t, 1, b.Stacks)
}

func TestBuffList_Add_Refreshes(t *testing.T) {
	var bl BuffList
	bl.Add(1, time.Minute, 0, 0, 5)
	b2 := bl.Add(1, 2*time.Minute, 0, 0, 5)
	assert.Equal(t, 2, b2.Stacks)
	assert.Len(t, bl.All(), 1)
}

func TestBuffList_Add_MaxStacks(t *testing.T) {
	var bl BuffList
	bl.Add(1, time.Minute, 0, 0, 3)
	bl.Add(1, time.Minute, 0, 0, 3)
	bl.Add(1, time.Minute, 0, 0, 3)
	b := bl.Add(1, time.Minute, 0, 0, 3)
	assert.Equal(t, 3, b.Stacks) // capped at maxStacks
}

func TestBuffList_Remove_Present(t *testing.T) {
	var bl BuffList
	bl.Add(1, time.Minute, 0, 0, 1)
	ok := bl.Remove(1)
	assert.True(t, ok)
	assert.Empty(t, bl.All())
}

func TestBuffList_Remove_Absent(t *testing.T) {
	var bl BuffList
	ok := bl.Remove(99)
	assert.False(t, ok)
}

func TestBuffList_Get_Found(t *testing.T) {
	var bl BuffList
	bl.Add(7, time.Minute, 0, 0, 1)
	b := bl.Get(7)
	require.NotNil(t, b)
	assert.Equal(t, 7, b.BuffID)
}

func TestBuffList_Get_NotFound(t *testing.T) {
	var bl BuffList
	assert.Nil(t, bl.Get(99))
}

func TestBuffList_All_Snapshot(t *testing.T) {
	var bl BuffList
	bl.Add(1, time.Minute, 0, 0, 1)
	bl.Add(2, time.Minute, 0, 0, 1)
	bl.Add(3, time.Minute, 0, 0, 1)
	all := bl.All()
	assert.Len(t, all, 3)
}

func TestBuffList_Tick_ExpiresOldBuffs(t *testing.T) {
	var bl BuffList
	bl.Add(1, -time.Second, 0, 0, 1) // already expired
	bl.Add(2, time.Minute, 0, 0, 1)  // still active

	results := bl.Tick(time.Now())
	require.Len(t, results, 1)
	assert.Equal(t, 1, results[0].BuffID)
	assert.True(t, results[0].Expired)
	assert.Len(t, bl.All(), 1) // only the active one remains
}

func TestBuffList_Tick_DOT(t *testing.T) {
	var bl BuffList
	b := bl.Add(5, time.Minute, 10, 30, 1)
	// Force next tick to be in the past
	b.NextTick = time.Now().Add(-time.Millisecond)

	results := bl.Tick(time.Now())
	require.Len(t, results, 1)
	assert.Equal(t, 5, results[0].BuffID)
	assert.Equal(t, 30, results[0].DotDmg)
	assert.False(t, results[0].Expired)
}

func TestBuffList_Tick_NoTickDue(t *testing.T) {
	var bl BuffList
	bl.Add(1, time.Minute, 100, 10, 1)
	// NextTick is in the future by default
	results := bl.Tick(time.Now())
	assert.Empty(t, results)
}

func TestBuffList_Tick_Empty(t *testing.T) {
	var bl BuffList
	results := bl.Tick(time.Now())
	assert.Empty(t, results)
}

func TestBuffList_MultipleBuffs_Independence(t *testing.T) {
	var bl BuffList
	bl.Add(1, time.Minute, 0, 0, 1)
	bl.Add(2, time.Minute, 0, 0, 1)
	bl.Remove(1)
	assert.Len(t, bl.All(), 1)
	assert.Equal(t, 2, bl.All()[0].BuffID)
}
