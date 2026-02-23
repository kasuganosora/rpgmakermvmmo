package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- TemplateEvent adapter unit tests ----

func TestTemplateEventAdapter_Basic(t *testing.T) {
	// Template map (map 2) has template events.
	tmplEvent := &MapEvent{
		ID:   1,
		Name: "MyTemplate",
		Pages: []*EventPage{
			{
				Trigger: 0,
				Image:   EventImage{CharacterName: "Actor1", CharacterIndex: 0, Direction: 2},
				List:    []*EventCommand{{Code: 101, Parameters: []interface{}{}}},
			},
		},
	}
	tmplMap := &MapData{
		ID:     2,
		Events: []*MapEvent{nil, tmplEvent},
	}

	// Target map (map 5) has an event with <TE:MyTemplate> note.
	targetEvent := &MapEvent{
		ID:   3,
		Name: "EV003",
		X:    10,
		Y:    20,
		Note: "<TE:MyTemplate>",
		Pages: []*EventPage{
			{
				Trigger: 0,
				Image:   EventImage{CharacterName: "event_mark"},
				List:    []*EventCommand{{Code: 0}}, // empty placeholder
			},
		},
	}
	targetMap := &MapData{
		ID:     5,
		Events: []*MapEvent{nil, nil, nil, targetEvent},
	}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	params := map[string]string{
		"TemplateMapId": "2",
		"OverrideTarget": `{"Image":"false","Direction":"false","Move":"false","Priority":"false","Trigger":"false","Option":"false"}`,
	}
	err := adapter.Apply(rl, params)
	require.NoError(t, err)

	// Target event should now have the template's pages.
	require.Len(t, targetEvent.Pages, 1)
	assert.Equal(t, "Actor1", targetEvent.Pages[0].Image.CharacterName)
	assert.Equal(t, 101, targetEvent.Pages[0].List[0].Code)

	// Position should be unchanged.
	assert.Equal(t, 10, targetEvent.X)
	assert.Equal(t, 20, targetEvent.Y)
	assert.Equal(t, 3, targetEvent.ID)
}

func TestTemplateEventAdapter_ByNumericID(t *testing.T) {
	// Template event at ID 5.
	tmplEvent := &MapEvent{
		ID:   5,
		Name: "SomeEvent",
		Pages: []*EventPage{
			{
				Trigger: 3,
				Image:   EventImage{CharacterName: "Monster1"},
				List:    []*EventCommand{{Code: 230}},
			},
		},
	}
	tmplMap := &MapData{
		ID:     2,
		Events: []*MapEvent{nil, nil, nil, nil, nil, tmplEvent},
	}

	// Event with <TE:5> (numeric reference).
	targetEvent := &MapEvent{
		ID:   1,
		Note: "<TE:5>",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "event_mark"}, List: []*EventCommand{{Code: 0}}},
		},
	}
	targetMap := &MapData{
		ID:     10,
		Events: []*MapEvent{nil, targetEvent},
	}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[10] = targetMap

	adapter := &templateEventAdapter{}
	params := map[string]string{"TemplateMapId": "2"}
	err := adapter.Apply(rl, params)
	require.NoError(t, err)

	// Should have been replaced by template event 5.
	require.Len(t, targetEvent.Pages, 1)
	assert.Equal(t, "Monster1", targetEvent.Pages[0].Image.CharacterName)
	assert.Equal(t, 3, targetEvent.Pages[0].Trigger)
}

func TestTemplateEventAdapter_NoMatch(t *testing.T) {
	tmplMap := &MapData{
		ID:     2,
		Events: []*MapEvent{nil, {ID: 1, Name: "Existing"}},
	}

	targetEvent := &MapEvent{
		ID:   1,
		Note: "<TE:NonExistent>",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "original"}},
		},
	}
	targetMap := &MapData{
		ID:     5,
		Events: []*MapEvent{nil, targetEvent},
	}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	params := map[string]string{"TemplateMapId": "2"}
	err := adapter.Apply(rl, params)
	require.NoError(t, err)

	// Should be unchanged — no match found.
	assert.Equal(t, "original", targetEvent.Pages[0].Image.CharacterName)
}

func TestTemplateEventAdapter_SkipsTemplateMap(t *testing.T) {
	// Events IN the template map should not be resolved.
	tmplEvent := &MapEvent{
		ID:   1,
		Name: "Self",
		Note: "<TE:Self>",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "template_sprite"}},
		},
	}
	tmplMap := &MapData{
		ID:     2,
		Events: []*MapEvent{nil, tmplEvent},
	}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap

	adapter := &templateEventAdapter{}
	params := map[string]string{"TemplateMapId": "2"}
	err := adapter.Apply(rl, params)
	require.NoError(t, err)

	// Template map event should be unchanged.
	assert.Equal(t, "template_sprite", tmplEvent.Pages[0].Image.CharacterName)
}

func TestTemplateEventAdapter_MultiPage(t *testing.T) {
	// Template with multiple pages.
	tmplEvent := &MapEvent{
		ID:   1,
		Name: "MultiPage",
		Pages: []*EventPage{
			{
				Trigger: 3,
				Image:   EventImage{CharacterName: ""},
				List:    []*EventCommand{{Code: 101}, {Code: 0}},
			},
			{
				Trigger: 0,
				Image:   EventImage{CharacterName: ""},
				List:    []*EventCommand{{Code: 0}},
			},
		},
	}
	tmplMap := &MapData{
		ID:     2,
		Events: []*MapEvent{nil, tmplEvent},
	}

	targetEvent := &MapEvent{
		ID:   1,
		Note: "<TE:MultiPage>",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "event_mark"}, List: []*EventCommand{{Code: 0}}},
		},
	}
	targetMap := &MapData{
		ID:     5,
		Events: []*MapEvent{nil, targetEvent},
	}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	params := map[string]string{"TemplateMapId": "2"}
	err := adapter.Apply(rl, params)
	require.NoError(t, err)

	// Should now have 2 pages from the template.
	require.Len(t, targetEvent.Pages, 2)
	assert.Equal(t, 3, targetEvent.Pages[0].Trigger)
	assert.Len(t, targetEvent.Pages[0].List, 2)
}

func TestTemplateEventAdapter_UnicodeNames(t *testing.T) {
	// Template with Unicode name (Japanese/Chinese).
	tmplEvent := &MapEvent{
		ID:   1,
		Name: "OP概略",
		Pages: []*EventPage{
			{
				Trigger: 3,
				Image:   EventImage{CharacterName: "tg-cm02-chr-01"},
				List:    []*EventCommand{{Code: 101}, {Code: 401}, {Code: 0}},
			},
		},
	}
	tmplMap := &MapData{
		ID:     2,
		Events: []*MapEvent{nil, tmplEvent},
	}

	targetEvent := &MapEvent{
		ID:   1,
		Note: "<TE:OP概略>",
		Pages: []*EventPage{
			{Image: EventImage{CharacterName: "event_mark"}, List: []*EventCommand{{Code: 0}}},
		},
	}
	targetMap := &MapData{
		ID:     5,
		Events: []*MapEvent{nil, targetEvent},
	}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	params := map[string]string{"TemplateMapId": "2"}
	err := adapter.Apply(rl, params)
	require.NoError(t, err)

	assert.Equal(t, "tg-cm02-chr-01", targetEvent.Pages[0].Image.CharacterName)
	assert.Equal(t, 3, targetEvent.Pages[0].Trigger)
}

// ---- loadPlugins tests ----

func TestLoadPlugins_NonExistentPath(t *testing.T) {
	plugins, err := loadPlugins("/nonexistent/path")
	assert.NoError(t, err)
	assert.Nil(t, plugins)
}

// ---- parseOverrideTarget tests ----

func TestParseOverrideTarget_Empty(t *testing.T) {
	ot := parseOverrideTarget("")
	assert.True(t, ot.Image)
	assert.True(t, ot.Direction)
}

func TestParseOverrideTarget_AllFalse(t *testing.T) {
	ot := parseOverrideTarget(`{"Image":"false","Direction":"false","Move":"false","Priority":"false","Trigger":"false","Option":"false"}`)
	assert.False(t, ot.Image)
	assert.False(t, ot.Direction)
	assert.False(t, ot.Move)
	assert.False(t, ot.Priority)
	assert.False(t, ot.Trigger)
	assert.False(t, ot.Option)
}

// ---- copyEventPage tests ----

func TestCopyEventPage_DeepCopy(t *testing.T) {
	original := &EventPage{
		Trigger: 0,
		Image:   EventImage{CharacterName: "Actor1", CharacterIndex: 2},
		List: []*EventCommand{
			{Code: 101, Parameters: []interface{}{"hello", float64(42)}},
			{Code: 0},
		},
		MoveRoute: &MoveRoute{
			List: []*MoveCommand{
				{Code: 1, Parameters: []interface{}{float64(2)}},
			},
			Repeat: true,
		},
	}

	copied := copyEventPage(original)

	// Values should match.
	assert.Equal(t, original.Trigger, copied.Trigger)
	assert.Equal(t, original.Image.CharacterName, copied.Image.CharacterName)
	assert.Equal(t, original.List[0].Code, copied.List[0].Code)

	// Modifying the copy should not affect the original.
	copied.List[0].Code = 999
	assert.Equal(t, 101, original.List[0].Code)

	copied.MoveRoute.List[0].Code = 888
	assert.Equal(t, 1, original.MoveRoute.List[0].Code)
}

// TestTemplateEventAdapter_OriginalPages_TECallOriginEvent verifies that after
// template resolution, OriginalPages is set and the TE_CALL_ORIGIN_EVENT command
// in the template page can be detected.
func TestTemplateEventAdapter_OriginalPages_TECallOriginEvent(t *testing.T) {
	// Template event "Move→" (like in real game: trigger=1, has TE_CALL_ORIGIN_EVENT).
	tmplEvent := &MapEvent{
		ID:   9,
		Name: "Move→",
		Pages: []*EventPage{
			{
				Trigger: 1, // player touch
				List: []*EventCommand{
					{Code: 117, Parameters: []interface{}{float64(12)}},        // Common Event #12
					{Code: 356, Parameters: []interface{}{"TE固有イベント呼び出し"}}, // TE_CALL_ORIGIN_EVENT
					{Code: 117, Parameters: []interface{}{float64(13)}},        // Common Event #13
					{Code: 0},
				},
			},
		},
	}
	tmplMap := &MapData{
		ID:     2,
		Events: []*MapEvent{nil, nil, nil, nil, nil, nil, nil, nil, nil, tmplEvent},
	}

	// Target event (door): has transfer command in its original page.
	targetEvent := &MapEvent{
		ID:   10,
		Name: "EV010",
		X:    19,
		Y:    13,
		Note: "<TE:Move→>",
		Pages: []*EventPage{
			{
				Trigger: 0,
				List: []*EventCommand{
					{Code: 201, Parameters: []interface{}{float64(0), float64(50), float64(12), float64(10), float64(6)}}, // Transfer to map 50
					{Code: 0},
				},
			},
		},
	}
	targetMap := &MapData{
		ID:     5,
		Events: []*MapEvent{nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, targetEvent},
	}

	rl := NewLoader("", "")
	rl.Maps[2] = tmplMap
	rl.Maps[5] = targetMap

	adapter := &templateEventAdapter{}
	params := map[string]string{"TemplateMapId": "2"}
	err := adapter.Apply(rl, params)
	require.NoError(t, err)

	// Verify template was applied.
	require.Len(t, targetEvent.Pages, 1)
	assert.Equal(t, 1, targetEvent.Pages[0].Trigger, "template page trigger should be 1 (player touch)")

	// Verify OriginalPages was saved.
	require.NotNil(t, targetEvent.OriginalPages, "OriginalPages should be set after template resolution")
	require.Len(t, targetEvent.OriginalPages, 1)

	// Verify original page has the transfer command.
	origPage := targetEvent.OriginalPages[0]
	assert.Equal(t, 201, origPage.List[0].Code, "original page should have Transfer Player command")

	// Verify the template page contains TE_CALL_ORIGIN_EVENT.
	tmplPage := targetEvent.Pages[0]
	foundTE := false
	for _, cmd := range tmplPage.List {
		if cmd != nil && cmd.Code == 356 {
			s, ok := cmd.Parameters[0].(string)
			if ok && s == "TE固有イベント呼び出し" {
				foundTE = true
			}
		}
	}
	assert.True(t, foundTE, "template page should contain TE固有イベント呼び出し plugin command")
}
