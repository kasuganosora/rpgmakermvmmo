package resource

// plugin_adapter_rr_test.go — unit tests for RegionRestrictions adapter,
// parseIntList, cpStarPassFixAdapter, integrateNotes, and applyTemplate
// coverage gaps found in the TemplateEvent MMO integration audit.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// integrateNotes
// ---------------------------------------------------------------------------

func TestIntegrateNotes_Mode1_Integrate(t *testing.T) {
	target := &MapEvent{Note: "original"}
	tmpl := &MapEvent{Note: "template"}
	integrateNotes(target, tmpl, 1)
	// Mode 1: template note prepended to original note
	assert.Equal(t, "templateoriginal", target.Note)
}

func TestIntegrateNotes_Mode2_Override_NoOp(t *testing.T) {
	target := &MapEvent{Note: "original"}
	tmpl := &MapEvent{Note: "template"}
	integrateNotes(target, tmpl, 2)
	// Mode 2: original note stays (it already IS the original)
	assert.Equal(t, "original", target.Note)
}

func TestIntegrateNotes_Mode0_NoChange(t *testing.T) {
	// Caller should never pass 0 (guarded by `integrateNote > 0`),
	// but the function itself does nothing for unknown modes.
	target := &MapEvent{Note: "original"}
	tmpl := &MapEvent{Note: "template"}
	integrateNotes(target, tmpl, 0)
	assert.Equal(t, "original", target.Note)
}

func TestIntegrateNotes_Mode1_BothEmpty(t *testing.T) {
	target := &MapEvent{Note: ""}
	tmpl := &MapEvent{Note: ""}
	integrateNotes(target, tmpl, 1)
	assert.Equal(t, "", target.Note)
}

// ---------------------------------------------------------------------------
// integrateNotes: verify unconditional application (P0-1 fix)
// ---------------------------------------------------------------------------

// TestIntegrateNote_AppliedWithoutOverRideTag verifies that after the P0-1 fix,
// integrateNotes is called for events without <OverRide> tag when IntegrateNote > 0.
func TestIntegrateNote_AppliedWithoutOverRideTag(t *testing.T) {
	tmplEvent := &MapEvent{
		ID:   1,
		Name: "Base",
		Note: "<server:global>",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "Actor1"}, List: []*EventCommand{{Code: 0}}},
		},
	}
	tmplMap := &MapData{ID: 2, Events: []*MapEvent{nil, tmplEvent}}

	// Target has <TE:Base> but NO <OverRide> tag
	targetEvent := &MapEvent{
		ID:   1,
		Note: "<TE:Base>",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "event_mark"}, List: []*EventCommand{{Code: 0}}},
		},
	}
	targetMap := &MapData{ID: 5, Events: []*MapEvent{nil, targetEvent}}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	params := map[string]string{
		"TemplateMapId":  "2",
		"IntegrateNote":  "1",
		// AutoOverride intentionally absent (defaults to false)
		"OverrideTarget": `{"Image":"false","Direction":"false","Move":"false","Priority":"false","Trigger":"false","Option":"false"}`,
	}
	err := adapter.Apply(rl, params)
	require.NoError(t, err)

	// After P0-1 fix: Note should contain the template note prepended,
	// even without <OverRide> tag.
	assert.Contains(t, targetEvent.Note, "<server:global>",
		"IntegrateNote=1 should apply regardless of OverRide tag (P0-1 fix)")
}

// ---------------------------------------------------------------------------
// parseIntList
// ---------------------------------------------------------------------------

func TestParseIntList_ValidList(t *testing.T) {
	result := parseIntList("1 2 3")
	assert.Equal(t, []int{1, 2, 3}, result)
}

func TestParseIntList_Empty(t *testing.T) {
	result := parseIntList("")
	assert.Nil(t, result)
}

func TestParseIntList_OnlySpaces(t *testing.T) {
	result := parseIntList("   ")
	assert.Nil(t, result)
}

func TestParseIntList_ZeroFiltered(t *testing.T) {
	// "0" entries are explicitly filtered out
	result := parseIntList("0 1 0 2")
	assert.Equal(t, []int{1, 2}, result)
}

func TestParseIntList_InvalidEntries_Filtered(t *testing.T) {
	result := parseIntList("1 abc 3")
	assert.Equal(t, []int{1, 3}, result)
}

func TestParseIntList_NegativeFiltered(t *testing.T) {
	// n > 0 check: negative numbers are filtered
	result := parseIntList("-1 2 -3 4")
	assert.Equal(t, []int{2, 4}, result)
}

func TestParseIntList_MixedValid(t *testing.T) {
	result := parseIntList("5 0 abc -2 10")
	assert.Equal(t, []int{5, 10}, result)
}

func TestParseIntList_SingleValue(t *testing.T) {
	result := parseIntList("7")
	assert.Equal(t, []int{7}, result)
}

func TestParseIntList_ExtraSpaces(t *testing.T) {
	result := parseIntList("  1   2  ")
	assert.Equal(t, []int{1, 2}, result)
}

// ---------------------------------------------------------------------------
// regionRestrictionsAdapter.Apply
// ---------------------------------------------------------------------------

func TestRegionRestrictionsAdapter_Name(t *testing.T) {
	a := &regionRestrictionsAdapter{}
	assert.Equal(t, "YEP_RegionRestrictions", a.Name())
}

func TestRegionRestrictionsAdapter_Apply_AllFields(t *testing.T) {
	rl := NewLoader("", "")
	a := &regionRestrictionsAdapter{}
	params := map[string]string{
		"Player Restrict": "1 2 3",
		"Player Allow":    "4 5",
		"Event Restrict":  "6",
		"All Restrict":    "7 8",
		"Event Allow":     "9",
		"All Allow":       "10 11 12",
	}
	err := a.Apply(rl, params)
	require.NoError(t, err)
	require.NotNil(t, rl.RegionRestr)

	assert.Equal(t, []int{1, 2, 3}, rl.RegionRestr.PlayerRestrict)
	assert.Equal(t, []int{4, 5}, rl.RegionRestr.PlayerAllow)
	assert.Equal(t, []int{6}, rl.RegionRestr.EventRestrict)
	assert.Equal(t, []int{7, 8}, rl.RegionRestr.AllRestrict)
	assert.Equal(t, []int{9}, rl.RegionRestr.EventAllow)
	assert.Equal(t, []int{10, 11, 12}, rl.RegionRestr.AllAllow)
}

func TestRegionRestrictionsAdapter_Apply_EmptyFields(t *testing.T) {
	rl := NewLoader("", "")
	a := &regionRestrictionsAdapter{}
	err := a.Apply(rl, map[string]string{})
	require.NoError(t, err)
	require.NotNil(t, rl.RegionRestr)

	assert.Nil(t, rl.RegionRestr.PlayerRestrict)
	assert.Nil(t, rl.RegionRestr.PlayerAllow)
	assert.Nil(t, rl.RegionRestr.EventRestrict)
}

func TestRegionRestrictionsAdapter_IsPlayerRestricted(t *testing.T) {
	rl := NewLoader("", "")
	a := &regionRestrictionsAdapter{}
	a.Apply(rl, map[string]string{"Player Restrict": "5 10"})

	assert.True(t, rl.RegionRestr.IsPlayerRestricted(5))
	assert.True(t, rl.RegionRestr.IsPlayerRestricted(10))
	assert.False(t, rl.RegionRestr.IsPlayerRestricted(3))
}

func TestRegionRestrictionsAdapter_IsPlayerAllowed(t *testing.T) {
	rl := NewLoader("", "")
	a := &regionRestrictionsAdapter{}
	a.Apply(rl, map[string]string{"Player Allow": "8"})

	assert.True(t, rl.RegionRestr.IsPlayerAllowed(8))
	assert.False(t, rl.RegionRestr.IsPlayerAllowed(9))
}

func TestRegionRestrictionsAdapter_AllRestrict_BlocksEvent(t *testing.T) {
	rl := NewLoader("", "")
	a := &regionRestrictionsAdapter{}
	a.Apply(rl, map[string]string{"All Restrict": "3"})

	assert.True(t, rl.RegionRestr.IsEventRestricted(3))
	assert.True(t, rl.RegionRestr.IsPlayerRestricted(3))
}

func TestRegionRestrictionsAdapter_AllAllow_AllowsEvent(t *testing.T) {
	rl := NewLoader("", "")
	a := &regionRestrictionsAdapter{}
	a.Apply(rl, map[string]string{"All Allow": "4"})

	assert.True(t, rl.RegionRestr.IsEventAllowed(4))
	assert.True(t, rl.RegionRestr.IsPlayerAllowed(4))
}

// ---------------------------------------------------------------------------
// cpStarPassFixAdapter.Apply
// ---------------------------------------------------------------------------

func TestCPStarPassFixAdapter_Name(t *testing.T) {
	a := &cpStarPassFixAdapter{}
	assert.Equal(t, "CP_Star_Passability_Fix", a.Name())
}

func TestCPStarPassFixAdapter_Apply_SetsFlag(t *testing.T) {
	rl := NewLoader("", "")
	assert.False(t, rl.CPStarPassFix)

	a := &cpStarPassFixAdapter{}
	err := a.Apply(rl, map[string]string{})
	require.NoError(t, err)
	assert.True(t, rl.CPStarPassFix)
}

// ---------------------------------------------------------------------------
// applyTemplate: remaining branch coverage
// ---------------------------------------------------------------------------

func TestApplyTemplate_EmptyTemplatePagesNoOp(t *testing.T) {
	target := &MapEvent{
		Note: "original",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "OriginalSprite"}},
		},
	}
	tmpl := &MapEvent{Pages: nil} // empty template

	applyTemplate(target, tmpl, overrideTarget{})
	// Should be a no-op
	require.Len(t, target.Pages, 1)
	assert.Equal(t, "OriginalSprite", target.Pages[0].Image.CharacterName)
}

func TestApplyTemplate_OverrideDirection_WithoutImage(t *testing.T) {
	tmplPage := &EventPage{
		Image: EventImage{CharacterName: "TmplSprite", Direction: 2},
	}
	origPage := &EventPage{
		Image: EventImage{CharacterName: "OrigSprite", Direction: 8},
	}

	target := &MapEvent{Pages: []*EventPage{origPage}}
	tmpl := &MapEvent{Pages: []*EventPage{tmplPage}}

	// Image=false, Direction=true: only direction is overridden from original
	applyTemplate(target, tmpl, overrideTarget{Image: false, Direction: true})

	require.Len(t, target.Pages, 1)
	// CharacterName comes from template (Image=false → no override)
	assert.Equal(t, "TmplSprite", target.Pages[0].Image.CharacterName)
	// Direction comes from original (Direction=true → override)
	assert.Equal(t, 8, target.Pages[0].Image.Direction)
}

func TestApplyTemplate_OverrideAll(t *testing.T) {
	tmplPage := &EventPage{
		Trigger:      0,
		PriorityType: 0,
		MoveType:     1,
		MoveSpeed:    3,
		MoveFrequency: 3,
		StepAnime:    true,
		DirectionFix: false,
		Through:      false,
		WalkAnime:    true,
		Image:        EventImage{CharacterName: "TmplSprite", Direction: 2},
	}
	origPage := &EventPage{
		Trigger:      3,
		PriorityType: 2,
		MoveType:     0,
		MoveSpeed:    5,
		MoveFrequency: 5,
		StepAnime:    false,
		DirectionFix: true,
		Through:      true,
		WalkAnime:    false,
		Image:        EventImage{CharacterName: "OrigSprite", Direction: 6},
	}

	target := &MapEvent{Pages: []*EventPage{origPage}}
	tmpl := &MapEvent{Pages: []*EventPage{tmplPage}}

	// All override flags = true → original values replace template
	applyTemplate(target, tmpl, overrideTarget{
		Image:     true,
		Direction: true,
		Move:      true,
		Priority:  true,
		Trigger:   true,
		Option:    true,
	})

	p := target.Pages[0]
	assert.Equal(t, 3, p.Trigger)             // from original
	assert.Equal(t, 2, p.PriorityType)         // from original
	assert.Equal(t, 0, p.MoveType)             // from original
	assert.Equal(t, "OrigSprite", p.Image.CharacterName) // from original
	assert.True(t, p.DirectionFix)             // from original
	assert.True(t, p.Through)                  // from original
}

func TestApplyTemplate_OriginalPages_Preserved(t *testing.T) {
	origPage := &EventPage{
		List: []*EventCommand{{Code: 201, Parameters: []interface{}{float64(0), float64(5), float64(1), float64(1), float64(2)}}},
	}
	tmplPage := &EventPage{
		List: []*EventCommand{{Code: 356, Parameters: []interface{}{"TE_CALL_ORIGIN_EVENT"}}, {Code: 0}},
	}

	target := &MapEvent{Pages: []*EventPage{origPage}}
	tmpl := &MapEvent{Pages: []*EventPage{tmplPage}}

	applyTemplate(target, tmpl, overrideTarget{})

	// OriginalPages should be the pre-apply pages
	require.NotNil(t, target.OriginalPages)
	require.Len(t, target.OriginalPages, 1)
	assert.Equal(t, 201, target.OriginalPages[0].List[0].Code)

	// New pages come from template
	require.Len(t, target.Pages, 1)
	assert.Equal(t, 356, target.Pages[0].List[0].Code)
}

func TestApplyTemplate_NilTemplatePage_Skipped(t *testing.T) {
	// Template has a nil page at index 0
	target := &MapEvent{
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "Orig"}},
		},
	}
	tmpl := &MapEvent{
		Pages: []*EventPage{nil, {Image: EventImage{CharacterName: "Second"}}},
	}

	applyTemplate(target, tmpl, overrideTarget{})

	require.Len(t, target.Pages, 2)
	assert.Nil(t, target.Pages[0]) // nil template page → nil result
	assert.Equal(t, "Second", target.Pages[1].Image.CharacterName)
}

// ---------------------------------------------------------------------------
// applyTemplate: fewer original pages than template pages (no override applied)
// ---------------------------------------------------------------------------

func TestApplyTemplate_FewerOrigPagesThanTemplate(t *testing.T) {
	// Template has 2 pages; original has 1
	// Override should only apply at index 0
	tmplPage0 := &EventPage{
		Image:   EventImage{CharacterName: "TmplA"},
		Trigger: 0,
	}
	tmplPage1 := &EventPage{
		Image:   EventImage{CharacterName: "TmplB"},
		Trigger: 3,
	}
	origPage0 := &EventPage{
		Image:   EventImage{CharacterName: "OrigA"},
		Trigger: 1,
	}

	target := &MapEvent{Pages: []*EventPage{origPage0}}
	tmpl := &MapEvent{Pages: []*EventPage{tmplPage0, tmplPage1}}

	applyTemplate(target, tmpl, overrideTarget{Image: true, Trigger: true})

	require.Len(t, target.Pages, 2)
	// Page 0: override applies → Image and Trigger from original
	assert.Equal(t, "OrigA", target.Pages[0].Image.CharacterName)
	assert.Equal(t, 1, target.Pages[0].Trigger)
	// Page 1: no original at index 1 → no override → template values kept
	assert.Equal(t, "TmplB", target.Pages[1].Image.CharacterName)
	assert.Equal(t, 3, target.Pages[1].Trigger)
}

// ---------------------------------------------------------------------------
// hasOverrideTag
// ---------------------------------------------------------------------------

func TestHasOverrideTag_TEOverRide(t *testing.T) {
	assert.True(t, hasOverrideTag("<TEOverRide>"))
}

func TestHasOverrideTag_Japanese(t *testing.T) {
	assert.True(t, hasOverrideTag("<TE上書き>"))
}

func TestHasOverrideTag_Plain(t *testing.T) {
	assert.True(t, hasOverrideTag("<OverRide>"))
}

func TestHasOverrideTag_PlainJapanese(t *testing.T) {
	assert.True(t, hasOverrideTag("<上書き>"))
}

func TestHasOverrideTag_None(t *testing.T) {
	assert.False(t, hasOverrideTag("<TE:SomeName>"))
}
