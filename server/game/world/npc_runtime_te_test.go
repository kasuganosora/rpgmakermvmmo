package world

// npc_runtime_te_test.go — unit tests targeting TemplateEvent.js server-side
// features in npc_runtime.go: TE condition evaluation chain, per-player NPC
// functions, ExtractMiniLabel, helper utilities.

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Minimal GameStateReader mock for TE tests
// ---------------------------------------------------------------------------

type teState struct {
	vars     map[int]int
	switches map[int]bool
	selfVars map[[3]int]int
}

func newTEState() *teState {
	return &teState{
		vars:     make(map[int]int),
		switches: make(map[int]bool),
		selfVars: make(map[[3]int]int),
	}
}

func (s *teState) GetSwitch(id int) bool  { return s.switches[id] }
func (s *teState) SetSwitch(id int, v bool) { s.switches[id] = v }
func (s *teState) GetVariable(id int) int  { return s.vars[id] }
func (s *teState) SetVariable(id int, v int) { s.vars[id] = v }
func (s *teState) GetSelfSwitch(mapID, eventID int, ch string) bool { return false }
func (s *teState) SetSelfSwitch(mapID, eventID int, ch string, v bool) {}
func (s *teState) GetSelfVariable(mapID, eventID, index int) int {
	return s.selfVars[[3]int{mapID, eventID, index}]
}
func (s *teState) SetSelfVariable(mapID, eventID, index, v int) {
	s.selfVars[[3]int{mapID, eventID, index}] = v
}

// comment is a helper to build a code-108 EventCommand with text content.
func comment(text string) *resource.EventCommand {
	return &resource.EventCommand{Code: 108, Parameters: []interface{}{text}}
}

// contComment is a code-408 continuation comment line.
func contComment(text string) *resource.EventCommand {
	return &resource.EventCommand{Code: 408, Parameters: []interface{}{text}}
}

// ---------------------------------------------------------------------------
// getStartComments
// ---------------------------------------------------------------------------

func TestGetStartComments_NilPage(t *testing.T) {
	assert.Equal(t, "", getStartComments(nil))
}

func TestGetStartComments_EmptyList(t *testing.T) {
	page := &resource.EventPage{List: []*resource.EventCommand{}}
	assert.Equal(t, "", getStartComments(page))
}

func TestGetStartComments_NilCmd_StopsEarly(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("first"),
			nil, // nil stops iteration
			comment("third"),
		},
	}
	assert.Equal(t, "first", getStartComments(page))
}

func TestGetStartComments_NonCommentCode_Stops(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("line1"),
			{Code: 101, Parameters: []interface{}{"text"}}, // ShowText — stops
			comment("should not appear"),
		},
	}
	assert.Equal(t, "line1", getStartComments(page))
}

func TestGetStartComments_MultipleLines(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("first"),
			contComment("second"),
			contComment("third"),
			{Code: 0},
		},
	}
	assert.Equal(t, "first\nsecond\nthird", getStartComments(page))
}

func TestGetStartComments_NonStringParam_Skipped(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 108, Parameters: []interface{}{42}}, // non-string param
			comment("after"),
		},
	}
	// 42 is not a string → skipped, "after" stops because first had no string param
	// Actually the loop continues for code 108 even if param is not a string;
	// it just doesn't write anything. "after" IS code 108 so it does get written.
	result := getStartComments(page)
	assert.Equal(t, "after", result)
}

func TestGetStartComments_EmptyParams(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 108, Parameters: []interface{}{}}, // empty params
			comment("next"),
		},
	}
	// Empty params → no write for first cmd, second cmd writes "next"
	assert.Equal(t, "next", getStartComments(page))
}

// ---------------------------------------------------------------------------
// meetsTEConditions
// ---------------------------------------------------------------------------

func TestMeetsTEConditions_NoComments_ReturnsTrue(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 101, Parameters: []interface{}{"text"}},
			{Code: 0},
		},
	}
	assert.True(t, meetsTEConditions(page, 1, 1, newTEState()))
}

func TestMeetsTEConditions_NoTETags_ReturnsTrue(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("just a regular comment, no TE condition"),
			{Code: 0},
		},
	}
	assert.True(t, meetsTEConditions(page, 1, 1, newTEState()))
}

func TestMeetsTEConditions_SingleTrue(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment(`\TE{1 == 1}`),
			{Code: 0},
		},
	}
	assert.True(t, meetsTEConditions(page, 1, 1, newTEState()))
}

func TestMeetsTEConditions_SingleFalse(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment(`\TE{1 == 2}`),
			{Code: 0},
		},
	}
	assert.False(t, meetsTEConditions(page, 1, 1, newTEState()))
}

func TestMeetsTEConditions_MultipleAllTrue(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment(`\TE{2 > 1}`),
			contComment(`\TE{3 != 5}`),
			{Code: 0},
		},
	}
	assert.True(t, meetsTEConditions(page, 1, 1, newTEState()))
}

func TestMeetsTEConditions_MultipleSomeFalse(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment(`\TE{1 == 1}`),
			contComment(`\TE{1 == 2}`), // this one is false
			{Code: 0},
		},
	}
	assert.False(t, meetsTEConditions(page, 1, 1, newTEState()))
}

func TestMeetsTEConditions_SelfVariableSubstitution(t *testing.T) {
	state := newTEState()
	state.SetSelfVariable(5, 10, 3, 99)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment(`\TE{\sv[3] >= 50}`),
			{Code: 0},
		},
	}
	assert.True(t, meetsTEConditions(page, 5, 10, state))
}

func TestMeetsTEConditions_GameVariableSubstitution(t *testing.T) {
	state := newTEState()
	state.SetVariable(202, 3)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment(`\TE{\v[202] == 3}`),
			{Code: 0},
		},
	}
	assert.True(t, meetsTEConditions(page, 1, 1, state))
}

func TestMeetsTEConditions_SwitchSubstitution_True(t *testing.T) {
	state := newTEState()
	state.SetSwitch(10, true)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment(`\TE{\s[10] == 1}`),
			{Code: 0},
		},
	}
	assert.True(t, meetsTEConditions(page, 1, 1, state))
}

func TestMeetsTEConditions_SwitchSubstitution_False(t *testing.T) {
	state := newTEState()
	// switch 10 = false (default)

	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment(`\TE{\s[10] == 1}`),
			{Code: 0},
		},
	}
	assert.False(t, meetsTEConditions(page, 1, 1, state))
}

// ---------------------------------------------------------------------------
// evalTEExpression
// ---------------------------------------------------------------------------

func TestEvalTEExpression_SvSubstitution(t *testing.T) {
	state := newTEState()
	state.SetSelfVariable(1, 2, 5, 7)
	// \sv[5] should resolve to 7; 7 > 5 → true
	assert.True(t, evalTEExpression(`\sv[5] > 5`, 1, 2, state))
}

func TestEvalTEExpression_VarSubstitution(t *testing.T) {
	state := newTEState()
	state.SetVariable(100, 42)
	// \v[100] == 42 → true
	assert.True(t, evalTEExpression(`\v[100] == 42`, 1, 1, state))
}

func TestEvalTEExpression_SwitchTrueSubstitution(t *testing.T) {
	state := newTEState()
	state.SetSwitch(5, true)
	// \s[5] == 1 → true
	assert.True(t, evalTEExpression(`\s[5] == 1`, 1, 1, state))
}

func TestEvalTEExpression_SwitchFalseSubstitution(t *testing.T) {
	state := newTEState()
	// \s[5] == 0 (switch false)
	assert.True(t, evalTEExpression(`\s[5] == 0`, 1, 1, state))
}

func TestEvalTEExpression_SvOrCondition(t *testing.T) {
	state := newTEState()
	state.SetVariable(206, 2)
	// \v[206] == 2 || \v[206] == 4 → true (left side matches)
	assert.True(t, evalTEExpression(`\v[206] == 2 || \v[206] == 4`, 1, 1, state))
}

func TestEvalTEExpression_SvAndCondition(t *testing.T) {
	state := newTEState()
	state.SetSelfVariable(1, 1, 1, 3)
	state.SetVariable(100, 5)
	// \sv[1] >= 3 && \v[100] == 5 → true
	assert.True(t, evalTEExpression(`\sv[1] >= 3 && \v[100] == 5`, 1, 1, state))
}

// ---------------------------------------------------------------------------
// evalLogicalExpr
// ---------------------------------------------------------------------------

func TestEvalLogicalExpr_SimpleTrue(t *testing.T) {
	assert.True(t, evalLogicalExpr("5 == 5"))
}

func TestEvalLogicalExpr_SimpleFalse(t *testing.T) {
	assert.False(t, evalLogicalExpr("5 == 6"))
}

func TestEvalLogicalExpr_OrFirstTrue(t *testing.T) {
	assert.True(t, evalLogicalExpr("1 == 1 || 2 == 3"))
}

func TestEvalLogicalExpr_OrAllFalse(t *testing.T) {
	assert.False(t, evalLogicalExpr("1 == 2 || 3 == 4"))
}

func TestEvalLogicalExpr_OrSecondTrue(t *testing.T) {
	assert.True(t, evalLogicalExpr("1 == 2 || 3 == 3"))
}

func TestEvalLogicalExpr_AndAllTrue(t *testing.T) {
	assert.True(t, evalLogicalExpr("1 == 1 && 2 == 2"))
}

func TestEvalLogicalExpr_AndFirstFalse(t *testing.T) {
	assert.False(t, evalLogicalExpr("1 == 2 && 2 == 2"))
}

func TestEvalLogicalExpr_AndSecondFalse(t *testing.T) {
	assert.False(t, evalLogicalExpr("1 == 1 && 2 == 3"))
}

func TestEvalLogicalExpr_TripleOr(t *testing.T) {
	// Only the last part is true
	assert.True(t, evalLogicalExpr("1 == 2 || 3 == 4 || 5 == 5"))
}

func TestEvalLogicalExpr_TripleAnd(t *testing.T) {
	assert.False(t, evalLogicalExpr("1 == 1 && 2 == 2 && 3 == 4"))
}

// ---------------------------------------------------------------------------
// splitLogical
// ---------------------------------------------------------------------------

func TestSplitLogical_NoSplit_ReturnsNil(t *testing.T) {
	parts := splitLogical("1 == 1", "||")
	assert.Nil(t, parts)
}

func TestSplitLogical_SplitByOr(t *testing.T) {
	parts := splitLogical("a || b", "||")
	require.Len(t, parts, 2)
	assert.Equal(t, "a ", parts[0])
	assert.Equal(t, " b", parts[1])
}

func TestSplitLogical_SplitByAnd(t *testing.T) {
	parts := splitLogical("x && y && z", "&&")
	require.Len(t, parts, 3)
}

func TestSplitLogical_InsideParens_NotSplit(t *testing.T) {
	// The || inside parens should not be split
	parts := splitLogical("(a || b) && c", "||")
	assert.Nil(t, parts, "|| inside parens should not split")
}

func TestSplitLogical_InsideQuote_NotSplit(t *testing.T) {
	// The || inside a quote should not be split
	parts := splitLogical("'a || b'", "||")
	assert.Nil(t, parts, "|| inside single-quoted string should not split")
}

func TestSplitLogical_InsideDoubleQuote_NotSplit(t *testing.T) {
	parts := splitLogical(`"a || b"`, "||")
	assert.Nil(t, parts)
}

func TestSplitLogical_ThreeParts(t *testing.T) {
	parts := splitLogical("1 || 2 || 3", "||")
	require.Len(t, parts, 3)
}

// ---------------------------------------------------------------------------
// evalComparison
// ---------------------------------------------------------------------------

func TestEvalComparison_StrictEqual_True(t *testing.T) {
	assert.True(t, evalComparison("5 === 5"))
}

func TestEvalComparison_StrictEqual_False(t *testing.T) {
	assert.False(t, evalComparison("5 === 6"))
}

func TestEvalComparison_StrictNotEqual_True(t *testing.T) {
	assert.True(t, evalComparison("5 !== 6"))
}

func TestEvalComparison_StrictNotEqual_False(t *testing.T) {
	assert.False(t, evalComparison("5 !== 5"))
}

func TestEvalComparison_Equal_True(t *testing.T) {
	assert.True(t, evalComparison("3 == 3"))
}

func TestEvalComparison_Equal_False(t *testing.T) {
	assert.False(t, evalComparison("3 == 4"))
}

func TestEvalComparison_NotEqual_True(t *testing.T) {
	assert.True(t, evalComparison("3 != 4"))
}

func TestEvalComparison_GreaterOrEqual_True(t *testing.T) {
	assert.True(t, evalComparison("5 >= 5"))
	assert.True(t, evalComparison("6 >= 5"))
}

func TestEvalComparison_GreaterOrEqual_False(t *testing.T) {
	assert.False(t, evalComparison("4 >= 5"))
}

func TestEvalComparison_LessOrEqual_True(t *testing.T) {
	assert.True(t, evalComparison("4 <= 5"))
	assert.True(t, evalComparison("5 <= 5"))
}

func TestEvalComparison_Greater_True(t *testing.T) {
	assert.True(t, evalComparison("6 > 5"))
}

func TestEvalComparison_Less_True(t *testing.T) {
	assert.True(t, evalComparison("4 < 5"))
}

func TestEvalComparison_NoOperator_Empty_False(t *testing.T) {
	assert.False(t, evalComparison(""))
}

func TestEvalComparison_NoOperator_Zero_False(t *testing.T) {
	assert.False(t, evalComparison("0"))
}

func TestEvalComparison_NoOperator_FalseLiteral_False(t *testing.T) {
	assert.False(t, evalComparison("false"))
}

func TestEvalComparison_NoOperator_NonZero_True(t *testing.T) {
	assert.True(t, evalComparison("1"))
}

func TestEvalComparison_NoOperator_StringValue_True(t *testing.T) {
	assert.True(t, evalComparison("something"))
}

// ---------------------------------------------------------------------------
// compareValues
// ---------------------------------------------------------------------------

func TestCompareValues_Numeric_Equal(t *testing.T) {
	assert.True(t, compareValues("10", "10", "=="))
	assert.True(t, compareValues("10", "10", "==="))
}

func TestCompareValues_Numeric_NotEqual(t *testing.T) {
	assert.True(t, compareValues("10", "11", "!="))
	assert.True(t, compareValues("10", "11", "!=="))
}

func TestCompareValues_Numeric_Greater(t *testing.T) {
	assert.True(t, compareValues("11", "10", ">"))
	assert.False(t, compareValues("10", "10", ">"))
}

func TestCompareValues_Numeric_Less(t *testing.T) {
	assert.True(t, compareValues("9", "10", "<"))
}

func TestCompareValues_Numeric_GTE(t *testing.T) {
	assert.True(t, compareValues("10", "10", ">="))
	assert.True(t, compareValues("11", "10", ">="))
}

func TestCompareValues_Numeric_LTE(t *testing.T) {
	assert.True(t, compareValues("10", "10", "<="))
	assert.True(t, compareValues("9", "10", "<="))
}

func TestCompareValues_String_Equal(t *testing.T) {
	assert.True(t, compareValues("'AAA'", "'AAA'", "=="))
	assert.False(t, compareValues("'AAA'", "'BBB'", "=="))
}

func TestCompareValues_String_StrictEqual(t *testing.T) {
	assert.True(t, compareValues("'AAA'", "'AAA'", "==="))
}

func TestCompareValues_String_NotEqual(t *testing.T) {
	assert.True(t, compareValues("'AAA'", "'BBB'", "!="))
	assert.True(t, compareValues("'AAA'", "'BBB'", "!=="))
}

func TestCompareValues_String_Greater(t *testing.T) {
	assert.True(t, compareValues("'B'", "'A'", ">"))
}

func TestCompareValues_String_Less(t *testing.T) {
	assert.True(t, compareValues("'A'", "'B'", "<"))
}

func TestCompareValues_String_GTE(t *testing.T) {
	assert.True(t, compareValues("'B'", "'B'", ">="))
}

func TestCompareValues_String_LTE(t *testing.T) {
	assert.True(t, compareValues("'A'", "'B'", "<="))
}

func TestCompareValues_UnknownOp_ReturnsFalse(t *testing.T) {
	assert.False(t, compareValues("1", "1", "??"))
}

func TestCompareValues_StringUnknownOp_ReturnsFalse(t *testing.T) {
	assert.False(t, compareValues("'a'", "'b'", "??"))
}

// ---------------------------------------------------------------------------
// stripQuotes
// ---------------------------------------------------------------------------

func TestStripQuotes_SingleQuotes(t *testing.T) {
	assert.Equal(t, "hello", stripQuotes("'hello'"))
}

func TestStripQuotes_DoubleQuotes(t *testing.T) {
	assert.Equal(t, "world", stripQuotes(`"world"`))
}

func TestStripQuotes_NoQuotes(t *testing.T) {
	assert.Equal(t, "abc", stripQuotes("abc"))
}

func TestStripQuotes_TooShort_Unchanged(t *testing.T) {
	assert.Equal(t, "'", stripQuotes("'"))
	assert.Equal(t, "", stripQuotes(""))
}

func TestStripQuotes_MismatchedQuotes_Unchanged(t *testing.T) {
	// Single open, double close — no stripping
	assert.Equal(t, `'hello"`, stripQuotes(`'hello"`))
}

// ---------------------------------------------------------------------------
// ExtractMiniLabel
// ---------------------------------------------------------------------------

func TestExtractMiniLabel_NilPage(t *testing.T) {
	assert.Equal(t, "", ExtractMiniLabel(nil))
}

func TestExtractMiniLabel_NoCommands(t *testing.T) {
	page := &resource.EventPage{List: []*resource.EventCommand{}}
	assert.Equal(t, "", ExtractMiniLabel(page))
}

func TestExtractMiniLabel_NilCmd_Skipped(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			nil,
			comment("<Mini Label: Boss>"),
		},
	}
	// nil cmd is skipped; the second cmd is found
	assert.Equal(t, "Boss", ExtractMiniLabel(page))
}

func TestExtractMiniLabel_NonStringParam_Skipped(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 108, Parameters: []interface{}{42}}, // non-string param
		},
	}
	assert.Equal(t, "", ExtractMiniLabel(page))
}

func TestExtractMiniLabel_WhitespaceOnly_Empty(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("<Mini Label:   >"), // only spaces after stripping
		},
	}
	assert.Equal(t, "", ExtractMiniLabel(page))
}

func TestExtractMiniLabel_Found_Code108(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("<Mini Label: ShopKeeper>"),
			{Code: 0},
		},
	}
	assert.Equal(t, "ShopKeeper", ExtractMiniLabel(page))
}

func TestExtractMiniLabel_Found_Code408(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 101, Parameters: []interface{}{}},           // not a comment — skipped
			contComment("<Mini Label: Guard>"),                 // code 408
		},
	}
	// code 101 is NOT 108/408, but ExtractMiniLabel iterates ALL cmds (not just start)
	assert.Equal(t, "Guard", ExtractMiniLabel(page))
}

func TestExtractMiniLabel_CaseInsensitive(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("<mini label: Wizard>"),
		},
	}
	assert.Equal(t, "Wizard", ExtractMiniLabel(page))
}

func TestExtractMiniLabel_NoEmptyParams(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			{Code: 108, Parameters: []interface{}{}}, // empty params
			comment("<Mini Label: Found>"),
		},
	}
	assert.Equal(t, "Found", ExtractMiniLabel(page))
}

// ---------------------------------------------------------------------------
// GetNPC
// ---------------------------------------------------------------------------

func TestGetNPC_Found(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := newNPC(5, 3, 4, &resource.EventPage{Trigger: 0})
	room.AddNPC(npc)

	got := room.GetNPC(5)
	require.NotNil(t, got)
	assert.Equal(t, 5, got.EventID)
}

func TestGetNPC_NotFound(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	assert.Nil(t, room.GetNPC(99))
}

// ---------------------------------------------------------------------------
// NPCSnapshotForPlayer
// ---------------------------------------------------------------------------

func TestNPCSnapshotForPlayer_WithActivePage(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger:      0,
		PriorityType: 1,
		MoveType:     1,
		StepAnime:    true,
		WalkAnime:    true,
		Image:        resource.EventImage{CharacterName: "Actor1", CharacterIndex: 2, Direction: 4},
	}
	npc := newNPC(1, 3, 4, page)
	npc.MapEvent.Note = ""
	room.AddNPC(npc)

	state := newTEState() // no switch conditions → page 0 meets conditions
	snaps := room.NPCSnapshotForPlayer(state)
	require.Len(t, snaps, 1)
	snap := snaps[0]
	assert.Equal(t, 1, snap["event_id"])
	assert.Equal(t, "Actor1", snap["walk_name"])
	assert.Equal(t, 2, snap["walk_index"])
	assert.Equal(t, 1, snap["priority_type"])
	assert.Equal(t, true, snap["step_anime"])
}

func TestNPCSnapshotForPlayer_NoActivePage_InactiveFallback(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// Page with switch condition that won't be met
	page := &resource.EventPage{
		Trigger: 0,
		Conditions: resource.EventPageConditions{
			Switch1Valid: true,
			Switch1ID:    999, // switch 999 is false
		},
		Image: resource.EventImage{CharacterName: "Actor1"},
	}
	npc := newNPC(2, 1, 1, page)
	room.AddNPC(npc)

	state := newTEState() // switch 999 = false
	snaps := room.NPCSnapshotForPlayer(state)
	require.Len(t, snaps, 1)
	snap := snaps[0]
	// No active page → empty defaults
	assert.Equal(t, "", snap["walk_name"])
	assert.Equal(t, 0, snap["priority_type"])
}

func TestNPCSnapshotForPlayer_TileID(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Image: resource.EventImage{TileID: 2572, CharacterName: "whatever"},
	}
	npc := newNPC(3, 2, 2, page)
	room.AddNPC(npc)

	snaps := room.NPCSnapshotForPlayer(newTEState())
	require.Len(t, snaps, 1)
	assert.Equal(t, "", snaps[0]["walk_name"])
	assert.Equal(t, 2572, snaps[0]["tile_id"])
}

func TestNPCSnapshotForPlayer_FunctionalMarker_Hidden(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Image: resource.EventImage{CharacterName: "event_mark"},
	}
	npc := newNPC(4, 2, 3, page)
	room.AddNPC(npc)

	snaps := room.NPCSnapshotForPlayer(newTEState())
	require.Len(t, snaps, 1)
	assert.Equal(t, "", snaps[0]["walk_name"])
}

func TestNPCSnapshotForPlayer_MiniLabel(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("<Mini Label: Boss>"),
			{Code: 0},
		},
	}
	npc := newNPC(5, 5, 5, page)
	room.AddNPC(npc)

	snaps := room.NPCSnapshotForPlayer(newTEState())
	require.Len(t, snaps, 1)
	assert.Equal(t, "Boss", snaps[0]["label"])
}

func TestNPCSnapshotForPlayer_IconOnEvent_FromNote(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{}
	npc := newNPC(6, 1, 1, page)
	npc.MapEvent.Note = "<Icon on Event: 64>"
	room.AddNPC(npc)

	snaps := room.NPCSnapshotForPlayer(newTEState())
	require.Len(t, snaps, 1)
	assert.Equal(t, 64, snaps[0]["icon_on_event"])
}

// ---------------------------------------------------------------------------
// GetTransferAtForPlayer
// ---------------------------------------------------------------------------

func TestGetTransferAtForPlayer_NoNPCAtPosition(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := newNPC(1, 5, 5, &resource.EventPage{Trigger: 1})
	room.AddNPC(npc)

	td := room.GetTransferAtForPlayer(0, 0, newTEState())
	assert.Nil(t, td)
}

func TestGetTransferAtForPlayer_NilMapEvent(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := &NPCRuntime{EventID: 1, X: 3, Y: 3, MapEvent: nil}
	room.AddNPC(npc)

	assert.Nil(t, room.GetTransferAtForPlayer(3, 3, newTEState()))
}

func TestGetTransferAtForPlayer_NoActivePage(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// Page requires switch 999 (not set) → no active page
	page := &resource.EventPage{
		Trigger: 1,
		Conditions: resource.EventPageConditions{Switch1Valid: true, Switch1ID: 999},
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}},
			{Code: 0},
		},
	}
	npc := newNPC(1, 3, 3, page)
	room.AddNPC(npc)

	assert.Nil(t, room.GetTransferAtForPlayer(3, 3, newTEState()))
}

func TestGetTransferAtForPlayer_WrongTrigger_ReturnsNil(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// Trigger 0 = action button, not touch
	page := &resource.EventPage{
		Trigger: 0,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}},
			{Code: 0},
		},
	}
	npc := newNPC(1, 3, 3, page)
	room.AddNPC(npc)

	assert.Nil(t, room.GetTransferAtForPlayer(3, 3, newTEState()))
}

func TestGetTransferAtForPlayer_SimpleTransfer_Returned(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(2), float64(3), float64(4)}},
			{Code: 0},
		},
	}
	npc := newNPC(1, 3, 3, page)
	room.AddNPC(npc)

	td := room.GetTransferAtForPlayer(3, 3, newTEState())
	require.NotNil(t, td)
	assert.Equal(t, 5, td.MapID)
	assert.Equal(t, 2, td.X)
	assert.Equal(t, 3, td.Y)
}

func TestGetTransferAtForPlayer_ComplexEvent_ReturnsNil(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// More than 3 meaningful commands → complex event, handled by executor
	page := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 101, Parameters: []interface{}{}},
			{Code: 401, Parameters: []interface{}{"line1"}},
			{Code: 401, Parameters: []interface{}{"line2"}},
			{Code: 401, Parameters: []interface{}{"line3"}},
			{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}},
			{Code: 0},
		},
	}
	npc := newNPC(1, 3, 3, page)
	room.AddNPC(npc)

	assert.Nil(t, room.GetTransferAtForPlayer(3, 3, newTEState()))
}

func TestGetTransferAtForPlayer_TECallOrigin_WithOriginalPages(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	// Template page: has TE_CALL_ORIGIN_EVENT, no direct transfer
	templatePage := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 117, Parameters: []interface{}{float64(12)}},          // Common Event
			{Code: 356, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}}, // TE marker
			{Code: 0},
		},
	}
	// Original page: has transfer
	origPage := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 201, Parameters: []interface{}{float64(0), float64(7), float64(5), float64(5), float64(2)}},
			{Code: 0},
		},
	}
	ev := &resource.MapEvent{
		ID:            1,
		X:             4, Y: 4,
		Pages:         []*resource.EventPage{templatePage},
		OriginalPages: []*resource.EventPage{origPage},
	}
	npc := &NPCRuntime{
		EventID:    1,
		X:          4, Y: 4,
		ActivePage: templatePage,
		MapEvent:   ev,
	}
	room.AddNPC(npc)

	td := room.GetTransferAtForPlayer(4, 4, newTEState())
	require.NotNil(t, td)
	assert.Equal(t, 7, td.MapID)
}

// ---------------------------------------------------------------------------
// GetTouchEventAtForPlayer
// ---------------------------------------------------------------------------

func TestGetTouchEventAtForPlayer_NoMatch(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := newNPC(1, 5, 5, &resource.EventPage{Trigger: 1})
	room.AddNPC(npc)

	id, page := room.GetTouchEventAtForPlayer(0, 0, newTEState())
	assert.Equal(t, 0, id)
	assert.Nil(t, page)
}

func TestGetTouchEventAtForPlayer_NoActivePage(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 1,
		Conditions: resource.EventPageConditions{Switch1Valid: true, Switch1ID: 999},
		List: []*resource.EventCommand{{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}}, {Code: 0}},
	}
	npc := newNPC(1, 3, 3, page)
	room.AddNPC(npc)

	id, pg := room.GetTouchEventAtForPlayer(3, 3, newTEState())
	assert.Equal(t, 0, id)
	assert.Nil(t, pg)
}

func TestGetTouchEventAtForPlayer_WrongTrigger(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 0, // action button, not touch
		List: []*resource.EventCommand{{Code: 101}, {Code: 0}},
	}
	npc := newNPC(1, 2, 2, page)
	room.AddNPC(npc)

	id, pg := room.GetTouchEventAtForPlayer(2, 2, newTEState())
	assert.Equal(t, 0, id)
	assert.Nil(t, pg)
}

func TestGetTouchEventAtForPlayer_EmptyCommandList(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 1,
		List:    []*resource.EventCommand{{Code: 0}}, // only end marker (len==1)
	}
	npc := newNPC(1, 2, 2, page)
	room.AddNPC(npc)

	id, pg := room.GetTouchEventAtForPlayer(2, 2, newTEState())
	assert.Equal(t, 0, id)
	assert.Nil(t, pg)
}

func TestGetTouchEventAtForPlayer_Found(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 1,
		List: []*resource.EventCommand{
			{Code: 101, Parameters: []interface{}{}},
			{Code: 0},
		},
	}
	npc := newNPC(7, 4, 4, page)
	room.AddNPC(npc)

	id, pg := room.GetTouchEventAtForPlayer(4, 4, newTEState())
	assert.Equal(t, 7, id)
	assert.Equal(t, page, pg)
}

func TestGetTouchEventAtForPlayer_Trigger2_Found(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 2, // event touch
		List:    []*resource.EventCommand{{Code: 101}, {Code: 0}},
	}
	npc := newNPC(8, 5, 5, page)
	room.AddNPC(npc)

	id, _ := room.GetTouchEventAtForPlayer(5, 5, newTEState())
	assert.Equal(t, 8, id)
}

// ---------------------------------------------------------------------------
// GetActivePageForPlayer
// ---------------------------------------------------------------------------

func TestGetActivePageForPlayer_Found(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{Trigger: 0}
	npc := newNPC(3, 1, 1, page)
	room.AddNPC(npc)

	got := room.GetActivePageForPlayer(3, newTEState())
	assert.Equal(t, page, got)
}

func TestGetActivePageForPlayer_NotFound(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	assert.Nil(t, room.GetActivePageForPlayer(99, newTEState()))
}

func TestGetActivePageForPlayer_ConditionNotMet_NilPage(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger:    0,
		Conditions: resource.EventPageConditions{Switch1Valid: true, Switch1ID: 999},
	}
	npc := newNPC(3, 1, 1, page)
	room.AddNPC(npc)

	// Switch 999 is false → no active page
	got := room.GetActivePageForPlayer(3, newTEState())
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// GetAutorunNPCsForPlayer
// ---------------------------------------------------------------------------

func TestGetAutorunNPCsForPlayer_None(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := newNPC(1, 1, 1, &resource.EventPage{Trigger: 0, List: []*resource.EventCommand{{Code: 101}, {Code: 0}}})
	room.AddNPC(npc)

	result := room.GetAutorunNPCsForPlayer(newTEState())
	assert.Empty(t, result)
}

func TestGetAutorunNPCsForPlayer_Found(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 3,
		List:    []*resource.EventCommand{{Code: 101}, {Code: 0}},
	}
	npc := newNPC(1, 1, 1, page)
	room.AddNPC(npc)

	result := room.GetAutorunNPCsForPlayer(newTEState())
	require.Len(t, result, 1)
	assert.Equal(t, 1, result[0].EventID)
}

func TestGetAutorunNPCsForPlayer_EmptyList_Excluded(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 3,
		List:    []*resource.EventCommand{{Code: 0}}, // only end marker
	}
	npc := newNPC(1, 1, 1, page)
	room.AddNPC(npc)

	result := room.GetAutorunNPCsForPlayer(newTEState())
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// GetParallelNPCsForPlayer
// ---------------------------------------------------------------------------

func TestGetParallelNPCsForPlayer_None(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	npc := newNPC(1, 1, 1, &resource.EventPage{Trigger: 3, List: []*resource.EventCommand{{Code: 101}, {Code: 0}}})
	room.AddNPC(npc)

	result := room.GetParallelNPCsForPlayer(newTEState())
	assert.Empty(t, result)
}

func TestGetParallelNPCsForPlayer_Found(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 4,
		List:    []*resource.EventCommand{{Code: 101}, {Code: 0}},
	}
	npc := newNPC(2, 2, 2, page)
	room.AddNPC(npc)

	result := room.GetParallelNPCsForPlayer(newTEState())
	require.Len(t, result, 1)
	assert.Equal(t, 2, result[0].EventID)
}

func TestGetParallelNPCsForPlayer_EmptyList_Excluded(t *testing.T) {
	room := newTestRoom(t, 1, 10, 10)
	page := &resource.EventPage{
		Trigger: 4,
		List:    []*resource.EventCommand{{Code: 0}},
	}
	npc := newNPC(2, 2, 2, page)
	room.AddNPC(npc)

	result := room.GetParallelNPCsForPlayer(newTEState())
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// countMeaningfulCommands
// ---------------------------------------------------------------------------

func TestCountMeaningfulCommands_Empty(t *testing.T) {
	page := &resource.EventPage{List: []*resource.EventCommand{}}
	assert.Equal(t, 0, countMeaningfulCommands(page))
}

func TestCountMeaningfulCommands_OnlyTerminator(t *testing.T) {
	page := &resource.EventPage{List: []*resource.EventCommand{{Code: 0}}}
	assert.Equal(t, 0, countMeaningfulCommands(page))
}

func TestCountMeaningfulCommands_OnlyComments(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("a comment"),
			contComment("continuation"),
		},
	}
	assert.Equal(t, 0, countMeaningfulCommands(page))
}

func TestCountMeaningfulCommands_Mixed(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			comment("comment"),          // 108 → ignored
			{Code: 101},                // meaningful
			{Code: 401},                // meaningful
			contComment("continuation"),// 408 → ignored
			{Code: 201},                // meaningful
			{Code: 0},                  // terminator → ignored
		},
	}
	assert.Equal(t, 3, countMeaningfulCommands(page))
}

func TestCountMeaningfulCommands_NilCmd_Skipped(t *testing.T) {
	page := &resource.EventPage{
		List: []*resource.EventCommand{
			nil,
			{Code: 101},
		},
	}
	assert.Equal(t, 1, countMeaningfulCommands(page))
}

// ---------------------------------------------------------------------------
// findRandomPassablePosition
// ---------------------------------------------------------------------------

func TestFindRandomPassablePosition_NoPassMap_ReturnsNegOne(t *testing.T) {
	room := &MapRoom{passMap: nil}
	x, y := room.findRandomPassablePosition(0, 100)
	assert.Equal(t, -1, x)
	assert.Equal(t, -1, y)
}

func TestFindRandomPassablePosition_ZeroAttempts_ReturnsNegOne(t *testing.T) {
	// maxAttempts=0 → loop never runs → returns (-1, -1)
	pm := resource.NewPassabilityMap(5, 5)
	room := &MapRoom{passMap: pm, npcs: []*NPCRuntime{}}
	x, y := room.findRandomPassablePosition(0, 0)
	assert.Equal(t, -1, x)
	assert.Equal(t, -1, y)
}

func TestFindRandomPassablePosition_PassableTile_Found(t *testing.T) {
	// NewPassabilityMap defaults to all tiles passable — any tile can be found.
	pm := resource.NewPassabilityMap(5, 5)
	room := &MapRoom{passMap: pm, npcs: []*NPCRuntime{}}
	x, y := room.findRandomPassablePosition(0, 100)
	// Should find some passable tile
	assert.NotEqual(t, -1, x)
	assert.NotEqual(t, -1, y)
	assert.True(t, x >= 0 && x < 5)
	assert.True(t, y >= 0 && y < 5)
}

func TestFindRandomPassablePosition_OccupiedByNPC_Skipped(t *testing.T) {
	pm := resource.NewPassabilityMap(3, 3)
	// Make all tiles passable
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			pm.SetPass(x, y, 2, true)
		}
	}

	// Place NPCs at all positions except (1,1)
	npcs := []*NPCRuntime{}
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			if x != 1 || y != 1 {
				npcs = append(npcs, &NPCRuntime{X: x, Y: y})
			}
		}
	}

	room := &MapRoom{passMap: pm, npcs: npcs}
	x, y := room.findRandomPassablePosition(0, 10000)
	if x != -1 {
		assert.Equal(t, 1, x)
		assert.Equal(t, 1, y)
	}
}

// ---------------------------------------------------------------------------
// extractMetaFloat
// ---------------------------------------------------------------------------

func TestExtractMetaFloat_ValidFloat(t *testing.T) {
	v := extractMetaFloat("<Random: 2.5>", "Random")
	assert.Equal(t, 2.5, v)
}

func TestExtractMetaFloat_TagWithoutValue(t *testing.T) {
	// <Random> with no value → returns 1.0
	v := extractMetaFloat("<Random>", "Random")
	assert.Equal(t, 1.0, v)
}

func TestExtractMetaFloat_NotFound(t *testing.T) {
	v := extractMetaFloat("<Other: 5>", "Random")
	assert.Equal(t, 0.0, v)
}

func TestExtractMetaFloat_Integer(t *testing.T) {
	v := extractMetaFloat("<Random: 3>", "Random")
	assert.Equal(t, 3.0, v)
}

// ---------------------------------------------------------------------------
// toInt
// ---------------------------------------------------------------------------

func TestToInt_Float64(t *testing.T) {
	assert.Equal(t, 42, toInt(float64(42)))
}

func TestToInt_Int(t *testing.T) {
	assert.Equal(t, 7, toInt(7))
}

func TestToInt_Int64(t *testing.T) {
	assert.Equal(t, 100, toInt(int64(100)))
}

func TestToInt_Unknown_ReturnsZero(t *testing.T) {
	assert.Equal(t, 0, toInt("not a number"))
	assert.Equal(t, 0, toInt(nil))
	assert.Equal(t, 0, toInt(true))
}
