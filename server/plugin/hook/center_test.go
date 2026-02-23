package hook

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHookCenter(t *testing.T) {
	hc := NewHookCenter()
	require.NotNil(t, hc)
}

func TestTrigger_NoHandlers(t *testing.T) {
	hc := NewHookCenter()
	out, err := hc.Trigger(context.Background(), "noop", 42)
	require.NoError(t, err)
	assert.Equal(t, 42, out)
}

func TestRegister_SingleHandler(t *testing.T) {
	hc := NewHookCenter()
	called := false
	hc.Register("ev", 0, "h1", func(ctx context.Context, event string, data interface{}) (interface{}, error) {
		called = true
		assert.Equal(t, "ev", event)
		return data, nil
	})
	_, err := hc.Trigger(context.Background(), "ev", "hello")
	require.NoError(t, err)
	assert.True(t, called)
}

func TestTrigger_DataPassThrough(t *testing.T) {
	hc := NewHookCenter()
	hc.Register("ev", 0, "double", func(_ context.Context, _ string, data interface{}) (interface{}, error) {
		return data.(int) * 2, nil
	})
	hc.Register("ev", 1, "addTen", func(_ context.Context, _ string, data interface{}) (interface{}, error) {
		return data.(int) + 10, nil
	})
	out, err := hc.Trigger(context.Background(), "ev", 5)
	require.NoError(t, err)
	assert.Equal(t, 20, out) // (5*2)+10
}

func TestTrigger_PriorityOrder(t *testing.T) {
	hc := NewHookCenter()
	var order []int
	hc.Register("ev", 10, "high", func(_ context.Context, _ string, d interface{}) (interface{}, error) {
		order = append(order, 10)
		return d, nil
	})
	hc.Register("ev", 1, "low", func(_ context.Context, _ string, d interface{}) (interface{}, error) {
		order = append(order, 1)
		return d, nil
	})
	hc.Register("ev", 5, "mid", func(_ context.Context, _ string, d interface{}) (interface{}, error) {
		order = append(order, 5)
		return d, nil
	})
	hc.Trigger(context.Background(), "ev", nil)
	assert.Equal(t, []int{1, 5, 10}, order)
}

func TestTrigger_ErrInterrupt(t *testing.T) {
	hc := NewHookCenter()
	var secondCalled bool
	hc.Register("ev", 0, "stopper", func(_ context.Context, _ string, d interface{}) (interface{}, error) {
		return d, ErrInterrupt
	})
	hc.Register("ev", 1, "should_not_run", func(_ context.Context, _ string, d interface{}) (interface{}, error) {
		secondCalled = true
		return d, nil
	})
	_, err := hc.Trigger(context.Background(), "ev", nil)
	assert.True(t, errors.Is(err, ErrInterrupt))
	assert.False(t, secondCalled)
}

func TestUnregister_ByName(t *testing.T) {
	hc := NewHookCenter()
	var called bool
	hc.Register("ev", 0, "h1", func(_ context.Context, _ string, d interface{}) (interface{}, error) {
		called = true
		return d, nil
	})
	hc.Unregister("ev", "h1")
	hc.Trigger(context.Background(), "ev", nil)
	assert.False(t, called)
}

func TestUnregister_OnlyNamed(t *testing.T) {
	hc := NewHookCenter()
	var c1, c2 bool
	hc.Register("ev", 0, "h1", func(_ context.Context, _ string, d interface{}) (interface{}, error) { c1 = true; return d, nil })
	hc.Register("ev", 1, "h2", func(_ context.Context, _ string, d interface{}) (interface{}, error) { c2 = true; return d, nil })
	hc.Unregister("ev", "h1")
	hc.Trigger(context.Background(), "ev", nil)
	assert.False(t, c1)
	assert.True(t, c2)
}

func TestUnregisterAll(t *testing.T) {
	hc := NewHookCenter()
	var c1, c2 bool
	hc.Register("evA", 0, "plugin", func(_ context.Context, _ string, d interface{}) (interface{}, error) { c1 = true; return d, nil })
	hc.Register("evB", 0, "plugin", func(_ context.Context, _ string, d interface{}) (interface{}, error) { c2 = true; return d, nil })
	hc.UnregisterAll("plugin")
	hc.Trigger(context.Background(), "evA", nil)
	hc.Trigger(context.Background(), "evB", nil)
	assert.False(t, c1)
	assert.False(t, c2)
}

func TestUnregisterAll_LeavesOthers(t *testing.T) {
	hc := NewHookCenter()
	var other bool
	hc.Register("evA", 0, "mine", func(_ context.Context, _ string, d interface{}) (interface{}, error) { return d, nil })
	hc.Register("evA", 1, "other", func(_ context.Context, _ string, d interface{}) (interface{}, error) { other = true; return d, nil })
	hc.UnregisterAll("mine")
	hc.Trigger(context.Background(), "evA", nil)
	assert.True(t, other)
}

func TestTrigger_NonInterruptError_Continues(t *testing.T) {
	hc := NewHookCenter()
	var secondCalled bool
	hc.Register("ev", 0, "err", func(_ context.Context, _ string, d interface{}) (interface{}, error) {
		return d, errors.New("some error")
	})
	hc.Register("ev", 1, "second", func(_ context.Context, _ string, d interface{}) (interface{}, error) {
		secondCalled = true
		return d, nil
	})
	_, err := hc.Trigger(context.Background(), "ev", nil)
	// Non-interrupt errors: the last non-interrupt error is returned, chain continues
	assert.NoError(t, err) // last handler returned nil
	assert.True(t, secondCalled)
}

func TestHookConstants(t *testing.T) {
	// Ensure constants are non-empty strings
	constants := []string{
		BeforePlayerMove, AfterPlayerMove, BeforeDamageCalc, AfterDamageCalc,
		BeforeSkillUse, AfterSkillUse, AfterMonsterDeath, BeforeItemUse,
		OnQuestComplete, OnPlayerLevelUp, OnPlayerLogin, OnPlayerLogout,
		OnChatSend, BeforeTradeCommit, OnMapEnter,
	}
	for _, c := range constants {
		assert.NotEmpty(t, c)
	}
}
