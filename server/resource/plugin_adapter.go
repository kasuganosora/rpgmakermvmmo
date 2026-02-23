package resource

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ---- Plugin Detection ----

// PluginEntry represents one entry from RMMV's plugins.js.
type PluginEntry struct {
	Name       string            `json:"name"`
	Status     bool              `json:"status"`
	Parameters map[string]string `json:"parameters"`
}

// loadPlugins reads the game's js/plugins.js and returns active plugin entries.
// The file format is: var $plugins = [ {...}, {...}, ... ];
func loadPlugins(dataPath string) ([]*PluginEntry, error) {
	// plugins.js is in the parent's js/ directory (dataPath is www/data/).
	jsPath := filepath.Join(filepath.Dir(dataPath), "js", "plugins.js")
	data, err := os.ReadFile(jsPath)
	if err != nil {
		return nil, nil // no plugins.js → no plugins
	}

	// Strip "var $plugins =" prefix and trailing semicolons to get pure JSON.
	content := strings.TrimSpace(string(data))
	idx := strings.Index(content, "[")
	if idx < 0 {
		return nil, nil
	}
	content = content[idx:]
	content = strings.TrimRight(content, "; \t\r\n")

	var entries []*PluginEntry
	if err := json.Unmarshal([]byte(content), &entries); err != nil {
		return nil, fmt.Errorf("resource: parse plugins.js: %w", err)
	}
	return entries, nil
}

// ---- Plugin Adapter Interface ----

// PluginAdapter transforms loaded resource data to apply plugin-specific behavior.
type PluginAdapter interface {
	// Name returns the RMMV plugin name this adapter handles.
	Name() string
	// Apply transforms the loaded data. Called after all maps are loaded.
	Apply(rl *ResourceLoader, params map[string]string) error
}

// knownAdapters lists all server-side plugin adapters.
// Add new adapters here as they are implemented.
var knownAdapters = []PluginAdapter{
	&templateEventAdapter{},
	&regionRestrictionsAdapter{},
	&cpStarPassFixAdapter{},
}

// applyPluginAdapters detects active plugins and runs matching adapters.
func (rl *ResourceLoader) applyPluginAdapters() error {
	plugins, err := loadPlugins(rl.DataPath)
	if err != nil {
		return err
	}

	// Build lookup of active plugins.
	active := make(map[string]map[string]string)
	for _, p := range plugins {
		if p != nil && p.Status {
			active[p.Name] = p.Parameters
		}
	}

	// Run each known adapter if its plugin is active.
	for _, adapter := range knownAdapters {
		params, ok := active[adapter.Name()]
		if !ok {
			continue
		}
		if err := adapter.Apply(rl, params); err != nil {
			return fmt.Errorf("plugin adapter %s: %w", adapter.Name(), err)
		}
	}
	return nil
}

// ---- TemplateEvent Adapter ----

// templateEventAdapter handles the TemplateEvent.js plugin by Triacontane.
// It resolves <TE:name> note tags: finds the named event in the template map
// and replaces the referencing event's pages with the template event's pages.
type templateEventAdapter struct{}

func (a *templateEventAdapter) Name() string { return "TemplateEvent" }

func (a *templateEventAdapter) Apply(rl *ResourceLoader, params map[string]string) error {
	// Parse template map ID from plugin parameters.
	tmplMapIDStr := params["TemplateMapId"]
	tmplMapID, err := strconv.Atoi(tmplMapIDStr)
	if err != nil || tmplMapID <= 0 {
		fmt.Printf("[TemplateEvent] invalid TemplateMapId: %q\n", tmplMapIDStr)
		return nil // no valid template map configured
	}

	tmplMap, ok := rl.Maps[tmplMapID]
	if !ok {
		fmt.Printf("[TemplateEvent] template map %d not found in loaded maps\n", tmplMapID)
		return nil // template map not found in data
	}

	// Parse OverrideTarget settings.
	override := parseOverrideTarget(params["OverrideTarget"])

	// Build name→event lookup for the template map.
	tmplByName := make(map[string]*MapEvent)
	for _, ev := range tmplMap.Events {
		if ev != nil && ev.Name != "" {
			tmplByName[ev.Name] = ev
		}
	}

	fmt.Printf("[TemplateEvent] template map %d: %d named events indexed\n", tmplMapID, len(tmplByName))

	// Regex to match <TE:name> note tags.
	teRegex := regexp.MustCompile(`<TE:([^>]+)>`)

	applied := 0
	notFound := 0
	// Scan all maps for events with <TE:name> note tags.
	for _, md := range rl.Maps {
		if md == nil || md.ID == tmplMapID {
			continue // skip the template map itself
		}
		for _, ev := range md.Events {
			if ev == nil || ev.Note == "" {
				continue
			}
			matches := teRegex.FindStringSubmatch(ev.Note)
			if matches == nil {
				continue
			}
			tmplRef := matches[1]

			// Match TemplateEvent.js behavior: try numeric ID first, then name.
			var tmplEv *MapEvent
			if id, err := strconv.Atoi(tmplRef); err == nil && id > 0 && id < len(tmplMap.Events) {
				tmplEv = tmplMap.Events[id]
			}
			if tmplEv == nil {
				tmplEv = tmplByName[tmplRef]
			}
			if tmplEv == nil {
				notFound++
				continue
			}

			// Replace the event's pages with the template event's pages.
			applyTemplate(ev, tmplEv, override)
			applied++
		}
	}

	fmt.Printf("[TemplateEvent] applied %d template(s) from map %d (%d not found)\n", applied, tmplMapID, notFound)
	return nil
}

// overrideTarget controls which properties get overridden from the template.
type overrideTarget struct {
	Image     bool
	Direction bool
	Move      bool
	Priority  bool
	Trigger   bool
	Option    bool
}

// parseOverrideTarget parses the OverrideTarget JSON string from plugin params.
func parseOverrideTarget(raw string) overrideTarget {
	ot := overrideTarget{
		Image:     true,
		Direction: true,
	}
	if raw == "" {
		return ot
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ot
	}
	ot.Image = m["Image"] == "true"
	ot.Direction = m["Direction"] == "true"
	ot.Move = m["Move"] == "true"
	ot.Priority = m["Priority"] == "true"
	ot.Trigger = m["Trigger"] == "true"
	ot.Option = m["Option"] == "true"
	return ot
}

// applyTemplate replaces the target event's pages with the template event's pages,
// respecting the OverrideTarget settings per the TemplateEvent.js behavior.
//
// OverrideTarget semantics: when a flag is TRUE, the original event's property
// overrides (replaces) the template's value. When FALSE, the template is used as-is.
func applyTemplate(target, tmpl *MapEvent, ot overrideTarget) {
	if len(tmpl.Pages) == 0 {
		return
	}

	// Save the original pages before replacing — needed for TE_CALL_ORIGIN_EVENT
	// which calls back to the source event's commands (e.g., door transfer targets).
	target.OriginalPages = target.Pages

	// Replace pages with deep copies from the template.
	target.Pages = make([]*EventPage, len(tmpl.Pages))
	for i, tmplPage := range tmpl.Pages {
		if tmplPage == nil {
			continue
		}
		page := copyEventPage(tmplPage)

		// Respect OverrideTarget: when a flag is ON, replace the template's
		// property with the original event's value.
		// Only apply if the original event has a page at this index.
		if i < len(target.OriginalPages) && target.OriginalPages[i] != nil {
			orig := target.OriginalPages[i]
			if ot.Trigger {
				page.Trigger = orig.Trigger
			}
			if ot.Priority {
				page.PriorityType = orig.PriorityType
			}
			if ot.Move {
				page.MoveType = orig.MoveType
				page.MoveSpeed = orig.MoveSpeed
				page.MoveFrequency = orig.MoveFrequency
				page.MoveRoute = orig.MoveRoute
			}
			if ot.Image {
				page.Image = orig.Image
			}
			if ot.Direction && !ot.Image {
				page.Image.Direction = orig.Image.Direction
			}
			if ot.Option {
				page.StepAnime = orig.StepAnime
				page.DirectionFix = orig.DirectionFix
				page.Through = orig.Through
				page.WalkAnime = orig.WalkAnime
			}
		}

		target.Pages[i] = page
	}
}

// copyEventPage creates a deep copy of an EventPage.
func copyEventPage(src *EventPage) *EventPage {
	if src == nil {
		return nil
	}
	dst := *src // shallow copy of all fields

	// Deep copy the command list.
	if src.List != nil {
		dst.List = make([]*EventCommand, len(src.List))
		for i, cmd := range src.List {
			if cmd != nil {
				cmdCopy := *cmd
				if cmd.Parameters != nil {
					cmdCopy.Parameters = make([]interface{}, len(cmd.Parameters))
					copy(cmdCopy.Parameters, cmd.Parameters)
				}
				dst.List[i] = &cmdCopy
			}
		}
	}

	// Deep copy move route.
	if src.MoveRoute != nil {
		mr := *src.MoveRoute
		if src.MoveRoute.List != nil {
			mr.List = make([]*MoveCommand, len(src.MoveRoute.List))
			for i, mc := range src.MoveRoute.List {
				if mc != nil {
					mcCopy := *mc
					if mc.Parameters != nil {
						mcCopy.Parameters = make([]interface{}, len(mc.Parameters))
						copy(mcCopy.Parameters, mc.Parameters)
					}
					mr.List[i] = &mcCopy
				}
			}
		}
		dst.MoveRoute = &mr
	}

	return &dst
}

// ---- YEP_RegionRestrictions Adapter ----

// regionRestrictionsAdapter parses region-based movement restriction config
// from the YEP_RegionRestrictions plugin and stores it on the ResourceLoader.
type regionRestrictionsAdapter struct{}

func (a *regionRestrictionsAdapter) Name() string { return "YEP_RegionRestrictions" }

func (a *regionRestrictionsAdapter) Apply(rl *ResourceLoader, params map[string]string) error {
	rr := &RegionRestrictions{
		EventRestrict: parseIntList(params["Event Restrict"]),
		AllRestrict:   parseIntList(params["All Restrict"]),
		EventAllow:    parseIntList(params["Event Allow"]),
		AllAllow:      parseIntList(params["All Allow"]),
	}
	rl.RegionRestr = rr
	fmt.Printf("[YEP_RegionRestrictions] event_restrict=%v all_restrict=%v event_allow=%v all_allow=%v\n",
		rr.EventRestrict, rr.AllRestrict, rr.EventAllow, rr.AllAllow)
	return nil
}

// parseIntList splits a space-separated string of ints into a slice.
func parseIntList(s string) []int {
	var result []int
	for _, part := range strings.Split(strings.TrimSpace(s), " ") {
		part = strings.TrimSpace(part)
		if part == "" || part == "0" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err == nil && n > 0 {
			result = append(result, n)
		}
	}
	return result
}

// ---- CP_Star_Passability_Fix Adapter ----

// cpStarPassFixAdapter detects the CP_Star_Passability_Fix plugin and sets a
// flag so buildPassability uses the modified star tile behavior.
type cpStarPassFixAdapter struct{}

func (a *cpStarPassFixAdapter) Name() string { return "CP_Star_Passability_Fix" }

func (a *cpStarPassFixAdapter) Apply(rl *ResourceLoader, _ map[string]string) error {
	rl.CPStarPassFix = true
	fmt.Println("[CP_Star_Passability_Fix] enabled — star tiles can block passage")
	return nil
}
