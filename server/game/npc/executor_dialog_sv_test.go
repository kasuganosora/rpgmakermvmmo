package npc

// executor_dialog_sv_test.go — unit tests for the \sv[n] self-variable text
// substitution path added to resolveTextCodes (TemplateEvent.js P0-2 fix).

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// resolveTextCodes: \sv[n] substitution
// ---------------------------------------------------------------------------

func TestResolveTextCodes_SvBasic(t *testing.T) {
	gs := newMockGameState()
	gs.SetSelfVariable(5, 10, 3, 42)

	s := testSession(1)
	s.CharName = "Hero"

	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 5, EventID: 10}

	result := exec.resolveTextCodes(`回数：\sv[3]回`, s, opts)
	assert.Equal(t, "回数：42回", result)
}

func TestResolveTextCodes_SvCaseInsensitive(t *testing.T) {
	gs := newMockGameState()
	gs.SetSelfVariable(1, 2, 5, 99)

	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 2}

	// Uppercase \SV should also work (case-insensitive regex)
	result := exec.resolveTextCodes(`\SV[5]`, s, opts)
	assert.Equal(t, "99", result)
}

func TestResolveTextCodes_SvZeroWhenNotSet(t *testing.T) {
	gs := newMockGameState()
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	// Self-variable 7 not set → value is 0
	result := exec.resolveTextCodes(`count=\sv[7]`, s, opts)
	assert.Equal(t, "count=0", result)
}

func TestResolveTextCodes_Sv_NilOpts_NoSubstitution(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())

	// nil opts → no \sv substitution, code left as-is
	result := exec.resolveTextCodes(`\sv[3]`, s, nil)
	assert.Equal(t, `\sv[3]`, result)
}

func TestResolveTextCodes_Sv_MapIDZero_NoSubstitution(t *testing.T) {
	gs := newMockGameState()
	gs.SetSelfVariable(0, 0, 3, 77)

	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	// MapID=0 → skips sv substitution
	opts := &ExecuteOpts{GameState: gs, MapID: 0, EventID: 5}

	result := exec.resolveTextCodes(`\sv[3]`, s, opts)
	assert.Equal(t, `\sv[3]`, result)
}

func TestResolveTextCodes_Sv_NilGameState_NoSubstitution(t *testing.T) {
	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{MapID: 5, EventID: 10, GameState: nil}

	result := exec.resolveTextCodes(`\sv[1]`, s, opts)
	assert.Equal(t, `\sv[1]`, result)
}

func TestResolveTextCodes_SvMultiple_AllReplaced(t *testing.T) {
	gs := newMockGameState()
	gs.SetSelfVariable(3, 7, 1, 10)
	gs.SetSelfVariable(3, 7, 2, 20)

	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 3, EventID: 7}

	result := exec.resolveTextCodes(`a=\sv[1], b=\sv[2]`, s, opts)
	assert.Equal(t, "a=10, b=20", result)
}

func TestResolveTextCodes_SvAndV_BothReplaced(t *testing.T) {
	gs := newMockGameState()
	gs.SetSelfVariable(1, 1, 3, 55)
	gs.SetVariable(100, 88)

	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	result := exec.resolveTextCodes(`sv=\sv[3] v=\V[100]`, s, opts)
	assert.Equal(t, "sv=55 v=88", result)
}

func TestResolveTextCodes_SvAndN_BothReplaced(t *testing.T) {
	gs := newMockGameState()
	gs.SetSelfVariable(2, 5, 1, 7)

	s := testSession(1)
	s.CharName = "Luna"
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 2, EventID: 5}

	result := exec.resolveTextCodes(`Hello \N[1]! You visited \sv[1] times.`, s, opts)
	assert.Equal(t, "Hello Luna! You visited 7 times.", result)
}

// ---------------------------------------------------------------------------
// resolveTextCodes: existing paths still work after \sv addition
// ---------------------------------------------------------------------------

func TestResolveTextCodes_V_StillWorks(t *testing.T) {
	gs := newMockGameState()
	gs.SetVariable(202, 3)

	s := testSession(1)
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	opts := &ExecuteOpts{GameState: gs, MapID: 1, EventID: 1}

	result := exec.resolveTextCodes(`期間：\V[202]`, s, opts)
	assert.Equal(t, "期間：3", result)
}

func TestResolveTextCodes_N_ActorName_StillWorks(t *testing.T) {
	res := &resource.ResourceLoader{
		Actors: []*resource.Actor{nil, nil, nil, {ID: 3, Name: "Elias"}},
	}
	s := testSession(1)
	exec := New(nil, res, nopLogger())
	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}

	result := exec.resolveTextCodes(`\N[3]`, s, opts)
	assert.Equal(t, "Elias", result)
}

func TestResolveTextCodes_N_ActorNotFound_ReturnsMatch(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{Actors: []*resource.Actor{nil}}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}

	// \N[99] — actor 99 not found → original \N[99] is kept
	result := exec.resolveTextCodes(`\N[99]`, s, opts)
	assert.Equal(t, `\N[99]`, result)
}

func TestResolveTextCodes_P_NonPlayer_ReturnsMatch(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	s.CharName = "Hero"
	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}

	// \P[2] — party member 2 (not player) → original code kept
	result := exec.resolveTextCodes(`\P[2]`, s, opts)
	assert.Equal(t, `\P[2]`, result)
}

func TestResolveTextCodes_V_NilGameState_ReturnsZero(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	// opts.GameState == nil → \V[5] returns "0"
	opts := &ExecuteOpts{MapID: 1, EventID: 1, GameState: nil}

	result := exec.resolveTextCodes(`count=\V[5]`, s, opts)
	assert.Equal(t, "count=0", result)
}

func TestResolveTextCodes_UnknownCode_ReturnsMatch(t *testing.T) {
	exec := New(nil, &resource.ResourceLoader{}, nopLogger())
	s := testSession(1)
	opts := &ExecuteOpts{GameState: newMockGameState(), MapID: 1, EventID: 1}

	// \S[5] matches the regex ([NVPS]) but no switch case handles S → return match unchanged
	result := exec.resolveTextCodes(`\S[5]`, s, opts)
	assert.Equal(t, `\S[5]`, result)
}
