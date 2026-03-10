package resource

// plugin_adapter_branches_test.go — additional branch coverage for
// templateEventAdapter.Apply, parseOverrideTarget, copyEventPage,
// applyPluginAdapters.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// templateEventAdapter.Apply: remaining branches
// ---------------------------------------------------------------------------

func TestTemplateEventAdapter_InvalidTemplateMapID(t *testing.T) {
	rl := NewLoader("", "")
	adapter := &templateEventAdapter{}
	// TemplateMapId = "abc" → invalid → returns nil, no panic
	err := adapter.Apply(rl, map[string]string{"TemplateMapId": "abc"})
	assert.NoError(t, err)
}

func TestTemplateEventAdapter_ZeroTemplateMapID(t *testing.T) {
	rl := NewLoader("", "")
	adapter := &templateEventAdapter{}
	err := adapter.Apply(rl, map[string]string{"TemplateMapId": "0"})
	assert.NoError(t, err)
}

func TestTemplateEventAdapter_TemplateMapNotFound(t *testing.T) {
	rl := NewLoader("", "")
	// Map 99 does not exist
	adapter := &templateEventAdapter{}
	err := adapter.Apply(rl, map[string]string{"TemplateMapId": "99"})
	assert.NoError(t, err)
}

func TestTemplateEventAdapter_AutoOverride_True(t *testing.T) {
	// AutoOverride=true → every TE event uses OverrideTarget (no <OverRide> tag needed)
	tmplEvent := &MapEvent{
		ID:   1,
		Name: "Base",
		Pages: []*EventPage{
			{Trigger: 3, Image: EventImage{CharacterName: "TmplSprite"}},
		},
	}
	tmplMap := &MapData{ID: 2, Events: []*MapEvent{nil, tmplEvent}}

	// No <OverRide> tag on target event
	targetEvent := &MapEvent{
		ID:   1,
		Note: "<TE:Base>",
		Pages: []*EventPage{
			{Trigger: 1, Image: EventImage{CharacterName: "OrigSprite"}, List: []*EventCommand{{Code: 0}}},
		},
	}
	targetMap := &MapData{ID: 5, Events: []*MapEvent{nil, targetEvent}}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	params := map[string]string{
		"TemplateMapId": "2",
		"AutoOverride":  "true",
		// Image=true, Trigger=true → original values used
		"OverrideTarget": `{"Image":"true","Direction":"false","Move":"false","Priority":"false","Trigger":"true","Option":"false"}`,
	}
	err := adapter.Apply(rl, params)
	require.NoError(t, err)

	// With AutoOverride=true and Image=true, Trigger=true: original values override template
	assert.Equal(t, "OrigSprite", targetEvent.Pages[0].Image.CharacterName)
	assert.Equal(t, 1, targetEvent.Pages[0].Trigger)
}

func TestTemplateEventAdapter_IntegrateNote_Mode2(t *testing.T) {
	// IntegrateNote=2 → original note is kept as-is
	tmplEvent := &MapEvent{
		ID:   1,
		Name: "Base",
		Note: "<template:meta>",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "Actor1"}, List: []*EventCommand{{Code: 0}}},
		},
	}
	tmplMap := &MapData{ID: 2, Events: []*MapEvent{nil, tmplEvent}}

	targetEvent := &MapEvent{
		ID:   1,
		Note: "<TE:Base>",
		Pages: []*EventPage{
			{List: []*EventCommand{{Code: 0}}},
		},
	}
	targetMap := &MapData{ID: 5, Events: []*MapEvent{nil, targetEvent}}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	err := adapter.Apply(rl, map[string]string{
		"TemplateMapId": "2",
		"IntegrateNote": "2",
	})
	require.NoError(t, err)

	// Mode 2: target.Note unchanged
	assert.Equal(t, "<TE:Base>", targetEvent.Note)
}

func TestTemplateEventAdapter_NilEventInTargetMap_Skipped(t *testing.T) {
	tmplEvent := &MapEvent{
		ID:   1,
		Name: "Base",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "Actor1"}, List: []*EventCommand{{Code: 0}}},
		},
	}
	tmplMap := &MapData{ID: 2, Events: []*MapEvent{nil, tmplEvent}}

	targetMap := &MapData{
		ID:     5,
		Events: []*MapEvent{nil, nil}, // nil events → skipped
	}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	err := adapter.Apply(rl, map[string]string{"TemplateMapId": "2"})
	assert.NoError(t, err)
}

func TestTemplateEventAdapter_EventWithNilNote_Skipped(t *testing.T) {
	tmplEvent := &MapEvent{
		ID:   1,
		Name: "Base",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "Actor1"}, List: []*EventCommand{{Code: 0}}},
		},
	}
	tmplMap := &MapData{ID: 2, Events: []*MapEvent{nil, tmplEvent}}

	targetEvent := &MapEvent{
		ID:   1,
		Note: "", // empty note — no TE tag
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "OrigSprite"}},
		},
	}
	targetMap := &MapData{ID: 5, Events: []*MapEvent{nil, targetEvent}}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	err := adapter.Apply(rl, map[string]string{"TemplateMapId": "2"})
	require.NoError(t, err)

	// Not replaced — note had no TE tag
	assert.Equal(t, "OrigSprite", targetEvent.Pages[0].Image.CharacterName)
}

// ---------------------------------------------------------------------------
// parseOverrideTarget: invalid JSON → returns defaults
// ---------------------------------------------------------------------------

func TestParseOverrideTarget_InvalidJSON_ReturnsDefaults(t *testing.T) {
	ot := parseOverrideTarget(`{invalid json}`)
	// Default: Image=true, Direction=true
	assert.True(t, ot.Image)
	assert.True(t, ot.Direction)
}

func TestParseOverrideTarget_AllTrue(t *testing.T) {
	ot := parseOverrideTarget(`{"Image":"true","Direction":"true","Move":"true","Priority":"true","Trigger":"true","Option":"true"}`)
	assert.True(t, ot.Image)
	assert.True(t, ot.Direction)
	assert.True(t, ot.Move)
	assert.True(t, ot.Priority)
	assert.True(t, ot.Trigger)
	assert.True(t, ot.Option)
}

// ---------------------------------------------------------------------------
// copyEventPage: nil move command in route list
// ---------------------------------------------------------------------------

func TestCopyEventPage_NilMoveCommand_Preserved(t *testing.T) {
	original := &EventPage{
		MoveRoute: &MoveRoute{
			List:   []*MoveCommand{nil, {Code: 2}}, // nil first command
			Repeat: false,
		},
	}

	copied := copyEventPage(original)
	require.NotNil(t, copied.MoveRoute)
	require.Len(t, copied.MoveRoute.List, 2)
	assert.Nil(t, copied.MoveRoute.List[0])
	assert.Equal(t, 2, copied.MoveRoute.List[1].Code)
}

func TestCopyEventPage_NilMoveCommandParameters_NotCopied(t *testing.T) {
	original := &EventPage{
		MoveRoute: &MoveRoute{
			List: []*MoveCommand{
				{Code: 1, Parameters: nil}, // nil parameters
			},
		},
	}

	copied := copyEventPage(original)
	require.NotNil(t, copied.MoveRoute)
	assert.Nil(t, copied.MoveRoute.List[0].Parameters)
}

func TestCopyEventPage_NilSrc_ReturnsNil(t *testing.T) {
	assert.Nil(t, copyEventPage(nil))
}

// ---------------------------------------------------------------------------
// applyPluginAdapters: plugin not found / unknown adapter → continue
// ---------------------------------------------------------------------------

func TestApplyPluginAdapters_NoPluginsFile_NoError(t *testing.T) {
	rl := NewLoader("/nonexistent/path", "/nonexistent/path")
	// Should gracefully return nil if plugins.js doesn't exist
	err := rl.applyPluginAdapters()
	assert.NoError(t, err)
}
